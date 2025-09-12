package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"

	"tide/common"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

func syncDataClient(conn net.Conn) {
	decoder := json.NewDecoder(conn)

	var err error
	var receivedItems = make(map[common.StationItemStruct]struct{})
	for {
		var msg forwardDataStruct
		if err = decoder.Decode(&msg); err != nil {
			if err != io.EOF && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "use of closed network connection") {
				slog.Error("Failed to decode sync data message", "error", err)
			}
			break
		}

		// First time received
		if _, ok := receivedItems[msg.StationItemStruct]; !ok {
			receivedItems[msg.StationItemStruct] = struct{}{}
			slog.Debug("First time receiving data from item", "station_id", msg.StationId, "item_name", msg.ItemName)
			// First make sure the table exists
			if err = db.MakeSureTableExist(msg.ItemName); err != nil {
				slog.Error("Failed to create table for item", "item_name", msg.ItemName, "error", err)
				return
			}
		}
		// save and publish
		if n, err := db.SaveDataHistory(msg.StationId, msg.ItemName, msg.Value, msg.Millisecond.ToTime()); err != nil {
			slog.Error("Failed to save data history", "station_id", msg.StationId, "item_name", msg.ItemName, "error", err)
			return
		} else if n > 0 {
			if msg.Type == kMsgDataGpio {
				_, _ = db.UpdateItemStatus(msg.StationId, msg.ItemName, common.NoStatus, msg.Millisecond.ToTime())
			}
			if msg.Type == kMsgMissData {
				Publish(missDataPubSub, msg, msg.StationItemStruct)
			} else {
				// msg.Type == kMsgData
				Publish(dataPubSub, msg, msg.StationItemStruct)
			}
		}
	}
}

func fillMissDataClient(conn net.Conn) (retOk bool) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	var permissions common.UUIDStringsMap
	err := decoder.Decode(&permissions)
	if err != nil {
		slog.Error("Failed to decode permissions", "error", err)
		return
	}
	var stationsItemsLatest = make(map[uuid.UUID]common.StringMsecMap)
	for stationId, items := range permissions {
		var itemsLatest = make(common.StringMsecMap)
		for _, itemName := range items {
			itemsLatest[itemName] = 0
		}
		err = db.GetItemsLatest(stationId, itemsLatest)
		if err != nil {
			slog.Error("Failed to get items latest timestamps", "station_id", stationId, "error", err)
			return
		}
		stationsItemsLatest[stationId] = itemsLatest
	}

	if err = encoder.Encode(stationsItemsLatest); err != nil {
		slog.Error("Failed to encode stations items latest", "error", err)
		return
	}

	var stationsMissData map[uuid.UUID]map[string][]common.DataTimeStruct
	err = decoder.Decode(&stationsMissData)
	if err != nil {
		slog.Error("Failed to decode stations miss data", "error", err)
		return
	}
	for stationId, missData := range stationsMissData {
		for itemName, ds := range missData {
			slog.Debug("Processing miss data", "item_name", itemName, "data_count", len(ds))
			for _, data := range ds {
				if err = db.MakeSureTableExist(itemName); err != nil {
					slog.Error("Failed to create table for item", "item_name", itemName, "error", err)
					return
				}
				if n, err := db.SaveDataHistory(stationId, itemName, data.Value, data.Millisecond.ToTime()); err != nil {
					slog.Error("Failed to save miss data history", "station_id", stationId, "item_name", itemName, "error", err)
					return
				} else if n > 0 {
					stationItem := common.StationItemStruct{StationId: stationId, ItemName: itemName}
					err = missDataPubSub.Publish(forwardDataStruct{
						Type:              kMsgMissData,
						StationItemStruct: stationItem,
						DataTimeStruct:    data,
					}, stationItem)
					if err != nil {
						slog.Error("Failed to publish miss data", "station_id", stationId, "item_name", itemName, "error", err)
					}
				}
			}
		}
	}
	return true
}
