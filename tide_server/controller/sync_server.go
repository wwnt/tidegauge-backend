package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/jackc/pgx/v5/pgconn"
)

func Sync(w http.ResponseWriter, r *http.Request) {
	conn, err := hijackUpgradeConn(w)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	username := requestUsername(r)

	var permissions common.UUIDStringsMap
	if requestRole(r) == auth.NormalUser {
		permissions, err = authorization.GetPermissions(username)
		if err != nil {
			slog.Error("Failed to get user permissions", "username", username, "error", err)
			return
		}
	}

	handleSyncServerConn(r.Context(), conn, username, permissions)
}

func handleSyncServerConn(parentCtx context.Context, conn io.ReadWriteCloser, username string, permissions common.UUIDStringsMap) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	// v1 同步流程（server 端）：
	// 1) stream1: 配置/状态增量
	// 2) stream2: 配置全量 + miss-status
	// 3) stream3: 数据增量
	// 4) stream4: miss-data
	// 其中 stream3/4 是一组循环：当权限变更触发 data 流断开后，会重新建立一组 stream3/4。
	var permissionTopics pubsub.TopicSet
	if permissions != nil {
		permissionTopics = uuidStringsMapToTopics(permissions)
	}

	cnf := yamux.DefaultConfig()
	cnf.EnableKeepAlive = false
	cnf.ConnectionWriteTimeout = 120 * time.Second
	cnf.LogOutput = io.Discard
	session, err := yamux.Server(conn, cnf)
	if err != nil {
		return
	}
	defer func() { _ = session.Close() }()

	// stream1: 配置/状态增量通道。先建立订阅，避免全量完成前漏掉增量消息。
	stream1, err := session.Accept()
	if err != nil {
		return
	}

	{
		localAvail, err := db.GetAvailableItems()
		if err != nil {
			slog.Error("Failed to get available items", "error", err)
			return
		}
		downstreamAvail := make(common.UUIDStringsMap)
		for _, stationItem := range localAvail {
			if _, ok := permissionTopics[stationItem]; ok || permissionTopics == nil {
				downstreamAvail[stationItem.StationId] = append(downstreamAvail[stationItem.StationId], stationItem.ItemName)
			}
		}

		if len(downstreamAvail) > 0 {
			slog.Debug("Sending available items update")
			if err = json.NewEncoder(stream1).Encode(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail}); err != nil {
				return
			}
		}
	}

	ctx, cancel := context.WithCancel(parentCtx)
	go func() {
		<-session.CloseChan()
		cancel()
	}()

	subscriber := hub.NewSubscriber(ctx, cancel, jsonWriter(stream1))
	go func() {
		<-ctx.Done()
		_ = stream1.Close()
	}()

	{
		// 配置增量与全量分离：先订阅 stream1，再执行 stream2 全量，保证顺序一致性。
		hub.TrackSubscriber(username, subscriber, connTypeSyncConfig)
		defer hub.UntrackSubscriber(username, subscriber)

		hub.Subscribe(BrokerConfig, subscriber, nil)
		defer hub.Unsubscribe(BrokerConfig, subscriber)
		hub.Subscribe(BrokerStatus, subscriber, nil)
		defer hub.Unsubscribe(BrokerStatus, subscriber)
	}

	// stream2: 配置全量与 miss-status，要求在 stream1 订阅完成后再发送。
	stream2, err := session.Accept()
	if err != nil {
		return
	}
	fullSyncConfigServer(stream2)
	_ = stream2.Close()

	go func() {
		defer func() { _ = session.Close() }()
		for {
			// 每轮建立一组 stream3/4，用于 data 增量 + miss-data。
			// 若权限变更导致 syncData 被取消，会退出当前轮次并在此处重建。
			if !syncDataServer(username, session, ctx) {
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
		slog.Error("Failed to get stations full info", "error", err)
		return
	}

	slog.Debug("Sending full station info")
	if err = encoder.Encode(stations); err != nil {
		return
	}

	deviceRecords, err := db.GetDeviceRecords()
	if err != nil {
		slog.Error("Failed to get device records", "error", err)
		return
	}

	slog.Debug("Sending device records")
	err = encoder.Encode(deviceRecords)
	if err != nil {
		return
	}

	var stationsLatestStatusLogRowID map[uuid.UUID]int64
	if err = decoder.Decode(&stationsLatestStatusLogRowID); err != nil {
		slog.Debug("Failed to decode stations latest status log row ID", "error", err)
		return
	}
	//miss status
	missStatusLogs := make(map[uuid.UUID][]common.RowIdItemStatusStruct)
	for stationID, rowID := range stationsLatestStatusLogRowID {
		hs, err := db.GetItemStatusLogs(stationID, rowID)
		if err != nil {
			slog.Error("Failed to get item status logs", "station_id", stationID, "row_id", rowID, "error", err)
			return
		}
		if hs != nil {
			missStatusLogs[stationID] = hs
		}
	}

	slog.Debug("Sending miss status logs")
	if err = encoder.Encode(missStatusLogs); err != nil {
		slog.Error("Failed to encode miss status logs", "error", err)
		return
	}
}

// currentSyncDataScope returns the latest permission scope for sync data streams.
// 权限刷新失败时按最小权限处理，避免因异常扩大数据可见范围。
func currentSyncDataScope(username string) (common.UUIDStringsMap, pubsub.TopicSet) {
	user, err := userManager.GetUser(username)
	if err != nil {
		slog.Error("Failed to get user while refreshing sync data scope", "username", username, "error", err)
		return common.UUIDStringsMap{}, pubsub.TopicSet{}
	}

	if user.Role != auth.NormalUser {
		// nil means full access for admin/super-admin (current behavior).
		return nil, nil
	}

	permissions, err := authorization.GetPermissions(username)
	if err != nil {
		slog.Error("Failed to get permissions while refreshing sync data scope", "username", username, "error", err)
		return common.UUIDStringsMap{}, pubsub.TopicSet{}
	}
	if permissions == nil {
		permissions = common.UUIDStringsMap{}
	}
	return permissions, uuidStringsMapToTopics(permissions)
}

func syncDataServer(username string, session *yamux.Session, parentCtx context.Context) (retOK bool) {
	// stream3: 数据增量（实时 + miss-data 通知）订阅通道。
	stream3, err := session.Accept()
	if err != nil {
		slog.Error("Failed to accept session", "error", err)
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	permissions, permissionTopics := currentSyncDataScope(username)

	subscriber := hub.NewSubscriber(ctx, cancel, jsonWriter(stream3))
	go func() { <-ctx.Done(); _ = stream3.Close() }()
	{
		hub.TrackSubscriber(username, subscriber, connTypeSyncData)
		defer hub.UntrackSubscriber(username, subscriber)

		hub.Subscribe(BrokerData, subscriber, permissionTopics)
		defer hub.Unsubscribe(BrokerData, subscriber)
		hub.Subscribe(BrokerMissingData, subscriber, permissionTopics)
		defer hub.Unsubscribe(BrokerMissingData, subscriber)
	}

	// stream4: miss-data 拉取通道。先下发权限范围，再按对端 latest 时间戳补数。
	stream4, err := session.Accept()
	if err != nil {
		slog.Error("Failed to accept session", "error", err)
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

	// miss data
	if permissions == nil {
		items, err := db.GetItems(uuid.Nil)
		if err != nil {
			slog.Error("Failed to get all items for permissions", "error", err)
			return
		}
		permissions = make(common.UUIDStringsMap)
		for _, item := range items {
			permissions[item.StationId] = append(permissions[item.StationId], item.Name)
		}
	}

	if err := encoder.Encode(permissions); err != nil {
		slog.Error("Failed to encode permissions", "error", err)
		return
	}

	var stationsItemsLatest map[uuid.UUID]common.StringMsecMap
	if err := decoder.Decode(&stationsItemsLatest); err != nil {
		slog.Error("Failed to decode stations items latest", "error", err)
		return
	}

	stationsMissData := make(map[uuid.UUID]map[string][]common.DataTimeStruct)
	for stationID, items := range permissions {
		missData := make(map[string][]common.DataTimeStruct)
		for _, itemName := range items {
			msec := stationsItemsLatest[stationID][itemName]
			if msec > 0 {
				ds, err := db.GetDataHistory(stationID, itemName, msec, 0)
				if err != nil {
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
						// relation Table does not exist
						continue
					}
					slog.Error("Failed to get data history for miss data", "station_id", stationID, "item_name", itemName, "error", err)
					return
				}
				if len(ds) > 0 {
					missData[itemName] = ds
				}
			}
		}
		stationsMissData[stationID] = missData
	}

	// send missData
	if err := encoder.Encode(stationsMissData); err != nil {
		slog.Error("Failed to encode stations miss data", "error", err)
		return
	}
}
