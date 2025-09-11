package controller

import (
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"tide/common"
	"tide/tide_server/db"
	"tide/tide_server/global"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

func tideDataReceiver() {
	if global.Config.Tide.Listen == "" {
		return
	}
	ln, err := net.Listen("tcp", global.Config.Tide.Listen)
	if err != nil {
		slog.Error("Failed to start tide data receiver", "listen", global.Config.Tide.Listen, "error", err)
		os.Exit(1)
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
			slog.Error("Station connection panic", "remote", conn.RemoteAddr().String(), "error", err)
		}
		_ = conn.Close()
		slog.Info("Station connection closed", "remote", conn.RemoteAddr().String())
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
		slog.Debug("Failed to decode station info", "identifier", info.Identifier, "error", err)
		return
	}
	stationId, err := db.GetLocalStationIdByIdentifier(info.Identifier)
	if err != nil {
		slog.Info("Station not created", "identifier", info.Identifier, "remote", conn.RemoteAddr().String())
		return
	}
	slog.Debug("Station connected", "identifier", info.Identifier, "remote", conn.RemoteAddr().String())

	if _, loaded := recvConnections.LoadOrStore(stationId, session); loaded {
		slog.Warn("Repeated station identifier detected", "identifier", info.Identifier, "remote", conn.RemoteAddr().String())
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
		slog.Error("Failed to edit station IP address", "station_id", stationId, "error", err)
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
				slog.Debug("Station connection decode error", "error", err)
			}
			break
		}
		switch msg.Type {
		case common.MsgData:
			var body common.ItemNameDataTimeStruct
			if err = json.Unmarshal(msg.Body, &body); err != nil {
				slog.Error("Failed to unmarshal data message", "error", err)
				return
			}
			// save and publish
			if _, err = db.SaveDataHistory(stationId, body.ItemName, body.Value, body.Millisecond.ToTime()); err != nil {
				slog.Error("Failed to save data history", "item_name", body.ItemName, "error", err)
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
				slog.Error("Failed to unmarshal GPIO data message", "error", err)
				return
			}
			if _, err = db.UpdateItemStatus(stationId, body.ItemName, common.NoStatus, body.Millisecond.ToTime()); err != nil {
				slog.Error("Failed to update item status", "item_name", body.ItemName, "error", err)
				return
			}
			if _, err = db.SaveDataHistory(stationId, body.ItemName, body.Value, body.Millisecond.ToTime()); err != nil {
				slog.Error("Failed to save GPIO data history", "item_name", body.ItemName, "error", err)
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
				slog.Error("Failed to unmarshal RPI status message", "error", err)
				return
			}
			if err = db.SaveRpiStatus(stationId, body.CpuTemp, body.Millisecond.ToTime()); err != nil {
				slog.Error("Failed to save RPI status", "station_id", stationId, "error", err)
			}
		case common.MsgItemStatus:
			var body common.RowIdItemStatusStruct
			if err = json.Unmarshal(msg.Body, &body); err != nil {
				slog.Error("Failed to unmarshal item status message", "error", err)
				return
			}
			// save and publish
			saveAndUpdateItemStatus(stationId, info.Identifier, body)
		default:
		}
	}
}

func saveAndUpdateItemStatus(stationId uuid.UUID, identifier string, statusLog common.RowIdItemStatusStruct) {
	slog.Debug("Updating item status",
		"identifier", identifier,
		"row_id", statusLog.RowId,
		"item_name", statusLog.ItemName,
		"status", statusLog.Status,
		"changed_at", statusLog.ChangedAt.ToTime(),
	)
	if n, err := db.UpdateAndSaveStatusLog(stationId, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime()); err != nil {
		slog.Error("Failed to update and save status log", "item_name", statusLog.ItemName, "error", err)
	} else if n > 0 {
		err = statusPubSub.Publish(SendMsgStruct{Type: kMsgUpdateItemStatus,
			Body: common.FullItemStatusStruct{
				StationId:             stationId,
				Identifier:            identifier,
				RowIdItemStatusStruct: statusLog,
			}}, nil)
		if err != nil {
			slog.Error("Failed to publish status update", "item_name", statusLog.ItemName, "error", err)
		}
	}
}
