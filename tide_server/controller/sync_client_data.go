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

func syncDataClient(conn net.Conn) {
	decoder := json.NewDecoder(conn)

	var err error
	var receivedItems = make(map[common.StationItemStruct]struct{})
	for {
		var msg forwardDataStruct
		if err = decoder.Decode(&msg); err != nil {
			if err != io.EOF && err != context.Canceled && !strings.Contains(err.Error(), "use of closed network connection") {
				logger.Error(err.Error())
			}
			break
		}

		// First time received
		if _, ok := receivedItems[msg.StationItemStruct]; !ok {
			receivedItems[msg.StationItemStruct] = struct{}{}
			logger.Debug("first receive", zap.String("station_id", msg.StationId.String()), zap.String("item_name", msg.ItemName))
			// First make sure the table exists
			if err = db.MakeSureTableExist(msg.ItemName); err != nil {
				logger.Error(err.Error())
				return
			}
		}
		// save and publish
		if n, err := db.SaveDataHistory(msg.StationId, msg.ItemName, msg.Value, msg.Millisecond.ToTime()); err != nil {
			logger.Error(err.Error())
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
		logger.Error(err.Error())
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
			logger.Error(err.Error())
			return
		}
		stationsItemsLatest[stationId] = itemsLatest
	}

	if err = encoder.Encode(stationsItemsLatest); err != nil {
		logger.Error(err.Error())
		return
	}

	var stationsMissData map[uuid.UUID]map[string][]common.DataTimeStruct
	err = decoder.Decode(&stationsMissData)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	for stationId, missData := range stationsMissData {
		for itemName, ds := range missData {
			logger.Debug("got miss data", zap.String("item_name", itemName), zap.Int("len", len(ds)))
			for _, data := range ds {
				if err = db.MakeSureTableExist(itemName); err != nil {
					logger.Error(err.Error())
					return
				}
				if n, err := db.SaveDataHistory(stationId, itemName, data.Value, data.Millisecond.ToTime()); err != nil {
					logger.Error(err.Error())
					return
				} else if n > 0 {
					stationItem := common.StationItemStruct{StationId: stationId, ItemName: itemName}
					err = missDataPubSub.Publish(forwardDataStruct{
						Type:              kMsgMissData,
						StationItemStruct: stationItem,
						DataTimeStruct:    data,
					}, stationItem)
					if err != nil {
						logger.DPanic("publish", zap.Error(err))
					}
				}
			}
		}
	}
	return true
}
