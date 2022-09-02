package controller

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"net"
	"strings"
	"tide/common"
	"tide/tide_server/db"
)

func fullSyncConfigClient(conn net.Conn, upstream *upstreamStorage) (retOk bool) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// update stations
	oldSs, err := db.GetStationsByUpstreamId(upstream.config.Id) // old stations
	if err != nil {
		logger.Error(err.Error())
		return
	}
	var newSs []db.StationFullInfo
	if err = decoder.Decode(&newSs); err != nil {
		logger.Error(err.Error())
		return
	}
	newStations := make(map[string]db.StationFullInfo)
	for _, stationFull := range newSs {
		newStations[stationFull.Identifier] = stationFull
		oldDs, err := db.GetDevices(stationFull.Id) // old devices
		if err != nil {
			logger.Error(err.Error())
			return
		}
		oldIs, err := db.GetItems(stationFull.Id) // old items
		if err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err := db.SyncStation(upstream.config.Id, stationFull.Station); err != nil {
			logger.Error(err.Error())
			return
		} else if n > 0 {
			Publish(configPubSub, SendMsgStruct{Type: kMsgSyncStation, Body: stationFull.Station}, nil)
		}
		// update station's cameras
		if n, err := db.SyncStationCannotEdit(stationFull.Station.Id, stationFull.Station.Cameras); err != nil {
			logger.Error(err.Error())
			return
		} else if n > 0 {
			Publish(configPubSub, SendMsgStruct{Type: kMsgSyncStationCannotEdit, Body: stationFull.Station}, nil)
		}
		// update station's status
		if n, err := db.UpdateStationStatus(stationFull.Id, stationFull.Status, stationFull.StatusChangedAt.ToTime()); err != nil {
			logger.Error(err.Error())
			return
		} else if n > 0 {
			Publish(configPubSub, SendMsgStruct{Type: kMsgUpdateStationStatus, Body: common.StationStatusStruct{
				StationId:          stationFull.Id,
				Identifier:         stationFull.Identifier,
				StatusChangeStruct: common.StatusChangeStruct{Status: stationFull.Status, ChangedAt: stationFull.StatusChangedAt},
			}}, nil)
		} //If there is no update, there is no need to publish
		// update devices
		newDevices := make(map[string]struct{})
		for _, device := range stationFull.Devices {
			newDevices[device.StationId.String()+device.Name] = struct{}{}
			if n, err := db.SyncDevice(device); err != nil {
				logger.Error(err.Error())
				return
			} else if n > 0 {
				Publish(configPubSub, SendMsgStruct{Type: kMsgSyncDevice, Body: device}, nil)
			} //If there is no update, there is no need to publish
		}
		for _, device := range oldDs {
			if _, ok := newDevices[device.StationId.String()+device.Name]; !ok {
				if n, err := db.DelDevice(device.StationId, device.Name); err != nil {
					logger.Error(err.Error())
					return
				} else if n > 0 {
					Publish(configPubSub, SendMsgStruct{Type: kMsgDelDevice, Body: device}, nil)
				} //If there is no update, there is no need to publish
			}
		}
		// update items
		newItems := make(map[common.StationItemStruct]struct{})
		for _, item := range stationFull.Items {
			newItems[common.StationItemStruct{StationId: item.StationId, ItemName: item.Name}] = struct{}{}
			if n, err := db.SyncItem(item); err != nil {
				logger.Error(err.Error())
				return
			} else if n > 0 {
				Publish(configPubSub, SendMsgStruct{Type: kMsgSyncItem, Body: item}, nil)
			} //If there is no update, there is no need to publish
		}
		for _, item := range oldIs {
			if _, ok := newItems[common.StationItemStruct{StationId: item.StationId, ItemName: item.Name}]; !ok {
				if n, err := db.DelItem(item.StationId, item.Name); err != nil {
					logger.Error(err.Error())
					return
				} else if n > 0 {
					Publish(configPubSub, SendMsgStruct{Type: kMsgDelItem, Body: item}, nil)
				} //If there is no update, there is no need to publish
			}
		}
	}
	for _, station := range oldSs {
		if _, ok := newStations[station.Identifier]; !ok {
			if n, err := db.DelUpstreamStation(upstream.config.Id, station.Id); err != nil {
				logger.Error(err.Error())
				return
			} else if n > 0 {
				Publish(configPubSub, SendMsgStruct{Type: kMsgDelUpstreamStation, Body: station.Id}, nil)
			} //If there is no update, there is no need to publish
		}
	}

	var newDrs []db.DeviceRecord
	if err = decoder.Decode(&newDrs); err != nil {
		logger.Error(err.Error())
		return
	}
	for _, dr := range newDrs {
		if n, err := db.SyncDeviceRecord(dr); err != nil {
			logger.Error(err.Error())
			return
		} else if n > 0 {
			Publish(configPubSub, SendMsgStruct{Type: kMsgEditDeviceRecord, Body: dr}, nil)
		} //If there is no update, there is no need to publish
	}

	var stationsLatestStatusLogRowId = make(map[uuid.UUID]int64)
	for _, station := range newSs {
		// get the latest status log rowId
		latestStatusLogRowId, err := db.GetLatestStatusLogRowId(station.Id)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		stationsLatestStatusLogRowId[station.Id] = latestStatusLogRowId
		logger.Debug("latest log", zap.String("identifier", station.Identifier), zap.Int64("rowId", latestStatusLogRowId))
	}

	if err = encoder.Encode(stationsLatestStatusLogRowId); err != nil {
		logger.Error(err.Error())
		return
	}

	var stationsMissStatusLogs = make(map[uuid.UUID][]common.RowIdItemStatusStruct)

	if err = decoder.Decode(&stationsMissStatusLogs); err != nil {
		logger.Error(err.Error())
		return
	}

	for stationId, missStatusLogs := range stationsMissStatusLogs {
		var latestLogByItem = make(map[string]int)
		for i, statusLog := range missStatusLogs {
			latestLogByItem[statusLog.ItemName] = i
		}
		for _, i := range latestLogByItem {
			statusLog := missStatusLogs[i]
			_, err = db.UpdateItemStatus(stationId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
			if err != nil {
				logger.Error(err.Error())
				return
			}
		}
		for _, statusLog := range missStatusLogs {
			if n, err := db.SaveItemStatusLog(stationId, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime()); err != nil {
				logger.Error(err.Error())
				return
			} else if n > 0 {
				Publish(configPubSub, SendMsgStruct{Type: kMsgMissItemStatus, Body: common.FullItemStatusStruct{
					StationId:             stationId,
					RowIdItemStatusStruct: statusLog,
				}}, nil)
			}
		}
	}
	return true
}

func incrementSyncConfigClient(conn net.Conn, upstream *upstreamStorage) {
	var (
		decoder = json.NewDecoder(conn)
		msg     RcvMsgStruct
	)
	for {
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF && err != context.Canceled && !strings.Contains(err.Error(), "use of closed network connection") {
				logger.Error(err.Error())
			}
			break
		}
		if !handleSyncConfigMsg(msg, upstream) {
			return
		}
	}
}

func handleSyncConfigMsg(msg RcvMsgStruct, upstream *upstreamStorage) (retOk bool) {
	var (
		err error
		n   int64
	)
	editMu.Lock()
	defer editMu.Unlock()

	switch msg.Type {
	case kMsgSyncStation:
		var station db.Station
		if err = json.Unmarshal(msg.Body, &station); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.SyncStation(upstream.config.Id, station); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgSyncStationCannotEdit:
		var station db.Station
		if err = json.Unmarshal(msg.Body, &station); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.SyncStationCannotEdit(station.Id, station.Cameras); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgDelUpstreamStation:
		var stationId uuid.UUID
		if err = json.Unmarshal(msg.Body, &stationId); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.DelUpstreamStation(upstream.config.Id, stationId); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgSyncDevice:
		var device db.Device
		if err = json.Unmarshal(msg.Body, &device); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.SyncDevice(device); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgDelDevice:
		var device db.Device
		if err = json.Unmarshal(msg.Body, &device); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.DelDevice(device.StationId, device.Name); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgSyncItem:
		var item db.Item
		if err = json.Unmarshal(msg.Body, &item); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.SyncItem(item); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgDelItem:
		var item db.Item
		if err = json.Unmarshal(msg.Body, &item); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.DelItem(item.StationId, item.Name); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgEditDeviceRecord:
		var dr db.DeviceRecord
		if err = json.Unmarshal(msg.Body, &dr); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.SyncDeviceRecord(dr); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgUpdateStationStatus:
		var body common.StationStatusStruct
		if err = json.Unmarshal(msg.Body, &body); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.UpdateStationStatus(body.StationId, body.Status, body.ChangedAt.ToTime()); err != nil {
			logger.Error(err.Error())
			return
		}
	case kMsgMissItemStatus, kMsgUpdateItemStatus:
		var body common.FullItemStatusStruct
		if err = json.Unmarshal(msg.Body, &body); err != nil {
			logger.Error(err.Error())
			return
		}
		if n, err = db.UpdateAndSaveStatusLog(body.StationId, body.RowId, body.ItemName, body.Status, body.ChangedAt.ToTime()); err != nil {
			logger.Error(err.Error())
			return
		}
	// establish connection
	// change sync user permission
	// upstream relay
	case kMsgUpdateAvailable:
		var body common.UUIDStringsMap
		if err = json.Unmarshal(msg.Body, &body); err != nil {
			logger.Error(err.Error())
			return
		}
		n, err = db.UpdateAvailableItems(upstream.config.Id, body)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		if n > 0 {
			changeUserAvailableScope(body)
			return true
		}
	}
	if n > 0 {
		switch msg.Type {
		case kMsgUpdateStationStatus, kMsgUpdateItemStatus:
			Publish(statusPubSub, msg, nil)
		default:
			Publish(configPubSub, msg, nil)
		}
	}
	return true
}
