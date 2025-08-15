package controller

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"go.uber.org/zap"
	"net"
	"strings"
	"tide/common"
	"tide/tide_server/db"
	"tide/tide_server/global"
	"time"
)

func tideDataReceiver() {
	if global.Config.Tide.Listen == "" {
		return
	}
	ln, err := net.Listen("tcp", global.Config.Tide.Listen)
	if err != nil {
		logger.Fatal("", zap.Error(err))
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleStationConn(conn)
	}
}

func handleStationConn(conn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("station conn panic", zap.Any("recover", err))
		}
		_ = conn.Close()
		logger.Info("station conn closed", zap.String("remote", conn.RemoteAddr().String()))
	}()
	cnf := yamux.DefaultConfig()
	cnf.KeepAliveInterval = 100 * time.Second
	cnf.ConnectionWriteTimeout = 30 * time.Second
	session, err := yamux.Server(conn, cnf)
	if err != nil {
		return
	}
	defer func() { _ = session.Close() }()

	stream, err := session.Open()
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()
	handleStationConnStream1(stream, session)
}

func handleStationConnStream1(conn net.Conn, session *yamux.Session) {
	var err error
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var info common.StationInfoStruct
	if err = decoder.Decode(&info); err != nil || info.Identifier == "" || len(info.Devices) == 0 {
		logger.Debug("decode stationInfo", zap.Error(err), zap.String("identifier", info.Identifier))
		return
	}
	stationId, err := db.GetLocalStationIdByIdentifier(info.Identifier)
	if err != nil {
		logger.Info("not created", zap.String("identifier", info.Identifier), zap.String("remote", conn.RemoteAddr().String()))
		return
	}
	logger.Debug("station conn", zap.String("identifier", info.Identifier), zap.String("remote", conn.RemoteAddr().String()))

	if _, loaded := recvConnections.LoadOrStore(stationId, session); loaded {
		logger.Warn("repeated identifier detected", zap.String("identifier", info.Identifier), zap.String("remote", conn.RemoteAddr().String()))
		return
	}

	defer func() {
		UpdateStationStatus(statusPubSub, stationId, info.Identifier, common.Disconnected)
		// Finish all the work, then delete from recvConnections
		recvConnections.Delete(stationId)
	}()
	if !UpdateStationStatus(statusPubSub, stationId, info.Identifier, common.Normal) {
		return
	}

	if err = db.EditStationNotSync(stationId, conn.RemoteAddr().String()); err != nil {
		logger.Error("db", zap.Error(err))
		return
	}

	if !SyncStationInfo(stationId, info) {
		return
	}

	if !WriteItemsLatest(encoder, stationId, info.Devices) {
		return
	}
	if !WriteLatestStatusLogRowId(encoder, stationId) {
		return
	}
	if !ReadMissData(decoder, stationId) {
		return
	}
	if !ReadMissStatusLogs(decoder, stationId) {
		return
	}

	for {
		var msg common.ReceiveMsgStruct
		if err = decoder.Decode(&msg); err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				logger.Debug("station conn", zap.Error(err))
			}
			break
		}
		switch msg.Type {
		case common.MsgData:
			var body common.ItemNameDataTimeStruct
			if err = json.Unmarshal(msg.Body, &body); err != nil {
				logger.Error(err.Error())
				return
			}
			// save and publish
			if _, err = db.SaveDataHistory(stationId, body.ItemName, body.Value, body.Millisecond.ToTime()); err != nil {
				logger.Error(err.Error())
				return
			}
			stationItem := common.StationItemStruct{StationId: stationId, ItemName: body.ItemName}
			dataPubSubDelay.DelayPublish(forwardDataStruct{
				Type:              kMsgData,
				StationItemStruct: stationItem,
				DataTimeStruct:    body.DataTimeStruct,
			}, stationItem)
		case common.MsgGpioData:
			var body common.ItemNameDataTimeStruct
			if err = json.Unmarshal(msg.Body, &body); err != nil {
				logger.Error(err.Error())
				return
			}
			if _, err = db.UpdateItemStatus(stationId, body.ItemName, common.NoStatus, body.Millisecond.ToTime()); err != nil {
				logger.Error(err.Error())
				return
			}
			if _, err = db.SaveDataHistory(stationId, body.ItemName, body.Value, body.Millisecond.ToTime()); err != nil {
				logger.Error(err.Error())
				return
			}
			stationItem := common.StationItemStruct{StationId: stationId, ItemName: body.ItemName}
			dataPubSubDelay.DelayPublish(forwardDataStruct{
				Type:              kMsgDataGpio,
				StationItemStruct: stationItem,
				DataTimeStruct:    body.DataTimeStruct,
			}, stationItem)
		case common.MsgRpiStatus:
			var body common.RpiStatusTimeStruct
			if err = json.Unmarshal(msg.Body, &body); err != nil {
				logger.Error(err.Error())
				return
			}
			if err = db.SaveRpiStatus(stationId, body.CpuTemp, body.Millisecond.ToTime()); err != nil {
				logger.Error(err.Error())
			}
		case common.MsgItemStatus:
			var body common.RowIdItemStatusStruct
			if err = json.Unmarshal(msg.Body, &body); err != nil {
				logger.Error(err.Error())
				return
			}
			// save and publish
			saveAndUpdateItemStatus(stationId, info.Identifier, body)
		default:
		}
	}
}

func saveAndUpdateItemStatus(stationId uuid.UUID, identifier string, statusLog common.RowIdItemStatusStruct) {
	logger.Debug("update item status",
		zap.String("identifier", identifier),
		zap.Int64("rowId", statusLog.RowId),
		zap.String("item", statusLog.ItemName),
		zap.String("status", statusLog.Status),
		zap.Time("changed_at", statusLog.ChangedAt.ToTime()),
	)
	if n, err := db.UpdateAndSaveStatusLog(stationId, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime()); err != nil {
		logger.Error(err.Error())
	} else if n > 0 {
		err = statusPubSub.Publish(SendMsgStruct{Type: kMsgUpdateItemStatus,
			Body: common.FullItemStatusStruct{
				StationId:             stationId,
				Identifier:            identifier,
				RowIdItemStatusStruct: statusLog,
			}}, nil)
		if err != nil {
			logger.DPanic("publish", zap.Error(err))
		}
	}
}
