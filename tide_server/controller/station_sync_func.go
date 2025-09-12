package controller

import (
	"encoding/json"
	"log/slog"

	"tide/common"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

func SyncStationInfo(stationId uuid.UUID, info common.StationInfoStruct) (retOk bool) {
	//stream, _ := net.Pipe()
	var err error

	// update items and devices
	var (
		oldItems   = make(map[string]db.Item)
		newItems   = make(map[string]db.Item)
		oldDevices = make(map[string]bool)
	)

	if ds, err := db.GetDevices(stationId); err != nil {
		slog.Error("Failed to get devices for station", "station_id", stationId, "error", err)
		return
	} else {
		for _, device := range ds {
			oldDevices[device.Name] = false
		}
	}

	if items, err := db.GetItems(stationId); err != nil {
		slog.Error("Failed to get items for station", "station_id", stationId, "error", err)
		return
	} else {
		for _, item := range items {
			oldItems[item.Name] = item
		}
	}
	// create device first because item need device
	for deviceName, items := range info.Devices {
		if _, ok := oldDevices[deviceName]; !ok {
			// only create device
			var device = db.Device{StationId: stationId, Name: deviceName}
			if err = db.EditDevice(device); err != nil { // add device
				slog.Error("Failed to add device", "station_id", stationId, "device_name", deviceName, "error", err)
				return
			}
			Publish(configPubSub, SendMsgStruct{Type: kMsgSyncDevice, Body: device}, nil)
		} else {
			oldDevices[deviceName] = true
		}
		for itemType, itemName := range items {
			newItems[itemName] = db.Item{Type: itemType, DeviceName: deviceName}
		}
	}

	for deviceName, keep := range oldDevices {
		if !keep {
			if _, err = db.DelDevice(stationId, deviceName); err != nil {
				slog.Error("Failed to delete device", "station_id", stationId, "device_name", deviceName, "error", err)
				return
			}
			Publish(configPubSub, SendMsgStruct{Type: kMsgDelDevice, Body: db.Device{StationId: stationId, Name: deviceName}}, nil)
		}
	}

	for itemName := range oldItems {
		if _, ok := newItems[itemName]; !ok {
			// delete item
			if _, err = db.DelItem(stationId, itemName); err != nil {
				slog.Error("Failed to delete item", "station_id", stationId, "item_name", itemName, "error", err)
				return
			}
			Publish(configPubSub, SendMsgStruct{Type: kMsgDelItem, Body: db.Item{StationId: stationId, Name: itemName}}, nil)
		}
	}
	var numNewItems int
	for itemName, newItemInfo := range newItems {
		// item info changed
		oldItemInfo, ok := oldItems[itemName]
		if !ok {
			numNewItems++
		}
		if !ok || oldItemInfo.Name != newItemInfo.DeviceName || oldItemInfo.Type != newItemInfo.Type {
			item := db.Item{StationId: stationId, Name: itemName, Type: newItemInfo.Type, DeviceName: newItemInfo.DeviceName}
			if err = db.EditItem(item); err != nil {
				slog.Error("Failed to edit item", "station_id", stationId, "item_name", itemName, "error", err)
				return
			}
			Publish(configPubSub, SendMsgStruct{Type: kMsgSyncItem, Body: item}, nil)
		}
	}
	if numNewItems > 0 {
		handleAddItems()
	}

	if camerasJson, err := json.Marshal(info.Cameras); err != nil {
		slog.Error("Failed to marshal cameras JSON", "station_id", stationId, "error", err)
		return
	} else {
		if n, err := db.SyncStationCannotEdit(stationId, camerasJson); err != nil {
			slog.Error("Failed to sync station cameras", "station_id", stationId, "error", err)
			return
		} else if n > 0 {
			Publish(configPubSub, SendMsgStruct{Type: kMsgSyncStationCannotEdit, Body: db.Station{Id: stationId, Cameras: camerasJson}}, nil)
		}
	}
	return true
}

// WriteItemsLatest get the last record time of each item
func WriteItemsLatest(encoder *json.Encoder, stationId uuid.UUID, devices map[string]map[string]string) (retOk bool) {
	var itemsLatest = make(common.StringMsecMap)
	for _, items := range devices {
		for _, itemName := range items {
			itemsLatest[itemName] = 0
		}
	}
	err := db.GetItemsLatest(stationId, itemsLatest)
	if err != nil {
		slog.Error("Failed to get items latest timestamps", "station_id", stationId, "error", err)
		return
	}
	slog.Debug("Retrieved latest timestamps for all items", "station_id", stationId, "item_count", len(itemsLatest))
	if err = encoder.Encode(itemsLatest); err != nil {
		slog.Error("Failed to encode items latest timestamps", "station_id", stationId, "error", err)
		return
	}
	return true
}

// WriteLatestStatusLogRowId get the latest status log's rowId
func WriteLatestStatusLogRowId(encoder *json.Encoder, stationId uuid.UUID) (retOk bool) {
	latestStatusLogRowId, err := db.GetLatestStatusLogRowId(stationId)
	if err != nil {
		slog.Error("Failed to get latest status log row ID", "station_id", stationId, "error", err)
		return
	}
	slog.Debug("Retrieved latest status log row ID", "station_id", stationId, "row_id", latestStatusLogRowId)
	if err = encoder.Encode(latestStatusLogRowId); err != nil {
		slog.Error("Failed to encode latest status log row ID", "station_id", stationId, "error", err)
		return
	}
	return true
}

func ReadMissData(decoder *json.Decoder, stationId uuid.UUID) (retOk bool) {
	var missData map[string][]common.DataTimeStruct
	err := decoder.Decode(&missData)
	if err != nil {
		slog.Error("Failed to decode miss data", "station_id", stationId, "error", err)
		return
	}
	for itemName, ds := range missData {
		slog.Debug("Processing miss data", "station_id", stationId, "item_name", itemName, "data_count", len(ds))
		for _, dataTime := range ds {
			if n, err := db.SaveDataHistory(stationId, itemName, dataTime.Value, dataTime.Millisecond.ToTime()); err != nil {
				slog.Error("Failed to save miss data history", "station_id", stationId, "item_name", itemName, "error", err)
				return
			} else if n > 0 {
				stationItem := common.StationItemStruct{StationId: stationId, ItemName: itemName}
				err = missDataPubSub.Publish(forwardDataStruct{
					Type:              kMsgMissData,
					StationItemStruct: stationItem,
					DataTimeStruct:    dataTime,
				}, stationItem)
				if err != nil {
					slog.Error("Failed to publish miss data", "station_id", stationId, "item_name", itemName, "error", err)
				}
			}
		}
	}
	return true
}

func ReadMissStatusLogs(decoder *json.Decoder, stationId uuid.UUID) (retOk bool) {
	var missStatusLogs []common.RowIdItemStatusStruct
	err := decoder.Decode(&missStatusLogs)
	if err != nil {
		slog.Error("Failed to decode miss status logs", "station_id", stationId, "error", err)
		return
	}
	slog.Debug("Processing miss status logs", "station_id", stationId, "log_count", len(missStatusLogs))
	// The last index of each item
	var latestLogIndexByItem = make(map[string]int)
	for i, statusLog := range missStatusLogs {
		latestLogIndexByItem[statusLog.ItemName] = i
	}
	for _, i := range latestLogIndexByItem {
		statusLog := missStatusLogs[i]
		_, err = db.UpdateItemStatus(stationId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
		if err != nil {
			slog.Error("Failed to update item status from miss logs", "station_id", stationId, "item_name", statusLog.ItemName, "error", err)
			return
		}
	}
	// update items status
	for _, statusLog := range missStatusLogs {
		n, err := db.SaveItemStatusLog(stationId, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
		if err != nil {
			slog.Error("Failed to save item status log", "station_id", stationId, "item_name", statusLog.ItemName, "row_id", statusLog.RowId, "error", err)
			return
		} else if n > 0 {
			err = configPubSub.Publish(SendMsgStruct{Type: kMsgMissItemStatus,
				Body: common.FullItemStatusStruct{
					StationId:             stationId,
					RowIdItemStatusStruct: statusLog,
				}}, nil)
			if err != nil {
				slog.Error("Failed to publish miss item status", "station_id", stationId, "item_name", statusLog.ItemName, "error", err)
			}
		}
	}
	return true
}
