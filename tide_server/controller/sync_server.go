package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"
)

func Sync(c *gin.Context) {
	// make client's Response.Body implement io.ReadWriteCloser
	// net/http/response.go: func isProtocolSwitchResponse()
	c.Writer.Header().Set("Upgrade", "websocket")
	c.Writer.Header().Set("Connection", "Upgrade")
	c.Writer.WriteHeader(http.StatusSwitchingProtocols)
	c.Writer.WriteHeaderNow()
	conn, _, err := c.Writer.Hijack()
	if err != nil {
		logger.Error("Hijack", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	username := c.GetString(contextKeyUsername)

	var permissions common.UUIDStringsMap // admin is nil
	if c.GetInt(contextKeyRole) == auth.NormalUser {
		permissions, err = authorization.GetPermissions(username)
		if err != nil {
			logger.Error(err.Error())
			return
		}
	}

	handleSyncServerConn(conn, username, permissions)
}

func handleSyncServerConn(conn io.ReadWriteCloser, username string, permissions common.UUIDStringsMap) {
	var permTopic pubsub.TopicMap
	if permissions != nil {
		permTopic = uuidStringsMapToTopic(permissions)
	}

	muxConfig := yamux.DefaultConfig()
	muxConfig.LogOutput = io.Discard
	session, err := yamux.Server(conn, muxConfig)
	if err != nil {
		return
	}

	defer func() { _ = session.Close() }()
	// stream1: increment config sync
	// stream2: full config sync
	// stream3: increment data sync
	// stream4: missing data sync
	stream1, err := session.Accept()
	if err != nil {
		return
	}

	{
		localAvail, err := db.GetAvailableItems()
		if err != nil {
			logger.Error(err.Error())
			return
		}
		var downstreamAvail = make(common.UUIDStringsMap)
		for _, stationItem := range localAvail {
			if _, ok := permTopic[stationItem]; ok || permTopic == nil {
				downstreamAvail[stationItem.StationId] = append(downstreamAvail[stationItem.StationId], stationItem.ItemName)
			}
		}

		if len(downstreamAvail) > 0 {
			logger.Debug("update available")
			if err = json.NewEncoder(stream1).Encode(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail}); err != nil {
				return
			}
		}
	}
	subscriber := pubsub.NewSubscriber(session.CloseChan(), stream1)

	// increment and full are separated to avoid receiving increment first
	// make sure subscribe first
	{
		addUserConn(username, subscriber, connTypeSyncConfig)
		defer delUserConn(username, subscriber)

		configPubSub.SubscribeTopic(subscriber, nil)
		defer configPubSub.Evict(subscriber)
		statusPubSub.SubscribeTopic(subscriber, nil)
		defer statusPubSub.Evict(subscriber)
	}

	stream2, err := session.Accept()
	if err != nil {
		return
	}
	// Make sure stream1 have subscribed when querying the database
	fullSyncConfigServer(stream2)
	_ = stream2.Close()

	go func() {
		defer func() { _ = session.Close() }()
		for {
			if !syncDataServer(username, session, permTopic, permissions) {
				return
			}
		}
	}()

	_, _ = io.Copy(io.Discard, stream1)
}

func fullSyncConfigServer(conn net.Conn) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	stations, err := db.GetStationsFullInfo()
	if err != nil {
		logger.Error(err.Error())
		return
	}

	logger.Debug("send full station info")
	if err = encoder.Encode(stations); err != nil {
		return
	}

	deviceRecords, err := db.GetDeviceRecords()
	if err != nil {
		logger.Error(err.Error())
		return
	}

	logger.Debug("send device record")
	err = encoder.Encode(deviceRecords)
	if err != nil {
		return
	}

	//miss status
	var stationsLatestStatusLogRowId map[uuid.UUID]int64
	if err = decoder.Decode(&stationsLatestStatusLogRowId); err != nil {
		logger.Debug(err.Error())
		return
	}
	var missStatusLogs = make(map[uuid.UUID][]common.RowIdItemStatusStruct)
	for stationId, rowId := range stationsLatestStatusLogRowId {
		hs, err := db.GetItemStatusLogs(stationId, rowId)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		if hs != nil {
			missStatusLogs[stationId] = hs
		}
	}

	logger.Debug("send miss status log")
	if err = encoder.Encode(missStatusLogs); err != nil {
		logger.Error(err.Error())
		return
	}
}

func syncDataServer(username string, session *yamux.Session, permTopic pubsub.TopicMap, permissions common.UUIDStringsMap) (retOk bool) {
	stream3, err := session.Accept()
	if err != nil {
		return
	}
	defer func() { _ = stream3.Close() }()

	subscriber := pubsub.NewSubscriber(session.CloseChan(), stream3)
	{
		addUserConn(username, subscriber, connTypeSyncData)
		defer delUserConn(username, subscriber)

		dataPubSub.SubscribeTopic(subscriber, permTopic)
		defer dataPubSub.Evict(subscriber)
		missDataPubSub.SubscribeTopic(subscriber, permTopic)
		defer missDataPubSub.Evict(subscriber)
	}

	stream4, err := session.Accept()
	if err != nil {
		return
	}

	fillMissDataServer(stream4, permissions)
	_ = stream4.Close()

	_, _ = io.Copy(io.Discard, stream3)
	return true
}

func fillMissDataServer(conn net.Conn, permissions common.UUIDStringsMap) {
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	if permissions == nil {
		items, err := db.GetItems(uuid.Nil)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		permissions = make(common.UUIDStringsMap)
		for _, item := range items {
			permissions[item.StationId] = append(permissions[item.StationId], item.Name)
		}
	}

	err := encoder.Encode(permissions)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	// miss data
	var stationsItemsLatest map[uuid.UUID]common.StringMsecMap
	if err = decoder.Decode(&stationsItemsLatest); err != nil {
		logger.Error(err.Error())
		return
	}

	var stationsMissData = make(map[uuid.UUID]map[string][]common.DataTimeStruct)
	for stationId, items := range permissions {
		var missData = make(map[string][]common.DataTimeStruct)
		for _, itemName := range items {
			msec := stationsItemsLatest[stationId][itemName]
			if msec > 0 {
				ds, err := db.GetDataHistory(stationId, itemName, msec, 0)
				if err != nil {
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
						// relation Table does not exist
						continue
					}
					logger.Error(err.Error())
					return
				}
				if len(ds) > 0 {
					missData[itemName] = ds
				}
			}
		}
		stationsMissData[stationId] = missData
	}

	// send missData
	if err = encoder.Encode(stationsMissData); err != nil {
		logger.Error(err.Error())
		return
	}
}
