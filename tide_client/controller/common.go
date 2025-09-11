package controller

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"tide/common"
	"tide/pkg/custype"
	"tide/pkg/project"
	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/device"
	"tide/tide_client/global"
	"time"
)

var (
	dataReceiveMu sync.Mutex

	connRegMu sync.RWMutex
	connReg   = make(map[string]any)
)

func RegisterConn(name string, d any) {
	connRegMu.Lock()
	defer connRegMu.Unlock()
	if d == nil {
		panic("Register device is nil")
	}
	if _, dup := connReg[name]; dup {
		panic("Register called twice for device " + name)
	}
	connReg[name] = d
}

func GetRegConn(name string) any {
	return connReg[name]
}

func Init() {
	dataPubSub := pubsub.NewPubSub()

	db.Init()
	project.RegisterReleaseFunc(db.Close)
	stationInfo.Identifier = global.Config.Identifier

	scheduleRemoveOutdatedData()

	addDevices()
	go receiveData(dataPubSub)

	addRpiStatus(dataPubSub)

	for name := range global.Config.Cameras.List {
		stationInfo.Cameras = append(stationInfo.Cameras, name)
	}

	go func() {
		for {
			client(dataPubSub)
			time.Sleep(3 * time.Second)
		}
	}()
}

var itemsStatus = make(map[string]common.StatusChangeStruct)

func addDevices() {
	var info = stationInfo.Devices
	for connType, files := range global.Config.Devices {
		newConnFunc := GetRegConn(connType).(func(json.RawMessage) common.StringMapMap)
		for _, filename := range files {
			rawConf, err := os.ReadFile(filename)
			if err != nil {
				slog.Error("Failed to read config file", "filename", filename, "error", err)
				os.Exit(1)
			}
			subInfo := newConnFunc(rawConf)
			device.MergeInfo(info, subInfo)
		}
	}
	// check duplicate
	var tmp = make(map[string]struct{})
	for deviceName, items := range info {
		for typ, name := range items {
			if _, ok := tmp[name]; ok {
				slog.Error("Duplicate item name", "item_name", name, "device_name", deviceName, "type", typ)
				os.Exit(1)
			} else {
				if common.ContainsIllegalCharacter(name) {
					slog.Error("Illegal item name", "item_name", name, "allowed_chars", "[0-9A-Za-z_]")
					os.Exit(1)
				}
				tmp[name] = struct{}{}
				if err := db.MakeSureTableExist(name); err != nil {
					slog.Error("Failed to create table", "item_name", name, "error", err)
					os.Exit(1)
				}
			}
		}
	}
	ds, err := db.GetItemsLatestStatus()
	if err != nil {
		slog.Error("Failed to get latest item status", "error", err)
		os.Exit(1)
	}
	for _, itemStatus := range ds {
		itemsStatus[itemStatus.ItemName] = itemStatus.StatusChangeStruct
	}
}

func receiveData(dataPub *pubsub.PubSub) {
	var (
		err error
		now custype.TimeMillisecond
	)
	for itemsData := range device.DataReceive {
		func() {
			// Lock when saving and sending data, avoid saving new data when querying data, and ensure that the connection is subscribed before running
			dataReceiveMu.Lock()
			defer dataReceiveMu.Unlock()
			now = custype.ToTimeMillisecond(time.Now())
			for _, data := range itemsData {
				if data.Typ == common.MsgData {
					if data.Value == nil {
						if itemsStatus[data.ItemName].Status != common.Abnormal {
							itemsStatus[data.ItemName] = common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: now}
							if rowId, err := db.SaveItemStatusLog(data.ItemName, common.Abnormal, int64(now)); err != nil {
								slog.Error("Failed to save item status log",
									"item_name", data.ItemName,
									"status", common.Abnormal,
									"error", err)
							} else {
								err = dataPub.Publish(common.SendMsgStruct{Type: common.MsgItemStatus,
									Body: common.RowIdItemStatusStruct{
										RowId: rowId,
										ItemStatusStruct: common.ItemStatusStruct{
											ItemName:           data.ItemName,
											StatusChangeStruct: common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: now},
										},
									}}, nil)
								if err != nil {
									slog.Error("Failed to publish item status message",
										"item_name", data.ItemName,
										"status", "abnormal",
										"error", err)
								}
							}
						}
					} else {
						if itemsStatus[data.ItemName].Status != common.Normal {
							itemsStatus[data.ItemName] = common.StatusChangeStruct{Status: common.Normal, ChangedAt: now}
							if rowId, err := db.SaveItemStatusLog(data.ItemName, common.Normal, int64(now)); err != nil {
								slog.Error("Failed to save item status log",
									"item_name", data.ItemName,
									"status", common.Normal,
									"error", err)
							} else {
								err = dataPub.Publish(common.SendMsgStruct{Type: common.MsgItemStatus,
									Body: common.RowIdItemStatusStruct{
										RowId: rowId,
										ItemStatusStruct: common.ItemStatusStruct{
											ItemName:           data.ItemName,
											StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: now},
										},
									},
								}, nil)
								if err != nil {
									slog.Error("Failed to publish item status message",
										"item_name", data.ItemName,
										"status", "normal",
										"error", err)
								}
							}
						}
					}
				}
				if data.Value != nil {
					if err = db.SaveData(data.ItemName, *data.Value, now.ToInt64()); err != nil {
						slog.Error("Failed to save data",
							"item_name", data.ItemName,
							"value", *data.Value,
							"error", err)
					}
					if err = dataPub.Publish(common.SendMsgStruct{
						Type: data.Typ,
						Body: common.ItemNameDataTimeStruct{
							ItemName:       data.ItemName,
							DataTimeStruct: common.DataTimeStruct{Value: *data.Value, Millisecond: now},
						},
					}, nil); err != nil {
						slog.Error("Failed to publish data message",
							"item_name", data.ItemName,
							"error", err)
					}
				}
			}
		}()
	}
}
