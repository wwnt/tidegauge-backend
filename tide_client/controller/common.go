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
	ingestMu sync.Mutex

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
	dataBroker := pubsub.NewBroker()

	db.Init()
	project.RegisterReleaseFunc(db.Close)
	stationInfo.Identifier = global.Config.Identifier

	scheduleRemoveOutdatedData()

	addDevices()
	go receiveData(dataBroker)

	addRpiStatus(dataBroker)

	for name := range global.Config.Cameras.List {
		stationInfo.Cameras = append(stationInfo.Cameras, name)
	}

	go func() {
		for {
			if !runSyncV2ClientOnce(dataBroker) {
				client(dataBroker)
			}
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
			}

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
	ds, err := db.GetItemsLatestStatus()
	if err != nil {
		slog.Error("Failed to get latest item status", "error", err)
		os.Exit(1)
	}
	for _, itemStatus := range ds {
		itemsStatus[itemStatus.ItemName] = itemStatus.StatusChangeStruct
	}
}

func receiveData(dataBroker *pubsub.Broker) {
	for itemsData := range device.DataReceive {
		func() {
			// Lock when saving and sending data, avoid saving new data when querying data, and ensure that the connection is subscribed before running
			ingestMu.Lock()
			defer ingestMu.Unlock()
			for _, data := range itemsData {
				at := data.At
				if at == 0 {
					at = custype.ToUnixMs(time.Now())
				}

				if data.Typ == common.MsgData {
					if data.Value == nil {
						if itemsStatus[data.ItemName].Status != common.Abnormal {
							itemsStatus[data.ItemName] = common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: at}
							if rowId, err := db.SaveItemStatusLog(data.ItemName, common.Abnormal, at.ToInt64()); err != nil {
								slog.Error("Failed to save item status log",
									"item_name", data.ItemName,
									"status", common.Abnormal,
									"error", err)
							} else {
								dataBroker.Publish(common.SendMsgStruct{Type: common.MsgItemStatus,
									Body: common.RowIdItemStatusStruct{
										RowId: rowId,
										ItemStatusStruct: common.ItemStatusStruct{
											ItemName:           data.ItemName,
											StatusChangeStruct: common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: at},
										},
									}}, nil)
							}
						}
					} else {
						if itemsStatus[data.ItemName].Status != common.Normal {
							itemsStatus[data.ItemName] = common.StatusChangeStruct{Status: common.Normal, ChangedAt: at}
							if rowId, err := db.SaveItemStatusLog(data.ItemName, common.Normal, at.ToInt64()); err != nil {
								slog.Error("Failed to save item status log",
									"item_name", data.ItemName,
									"status", common.Normal,
									"error", err)
							} else {
								dataBroker.Publish(common.SendMsgStruct{Type: common.MsgItemStatus,
									Body: common.RowIdItemStatusStruct{
										RowId: rowId,
										ItemStatusStruct: common.ItemStatusStruct{
											ItemName:           data.ItemName,
											StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: at},
										},
									},
								}, nil)
							}
						}
					}
				}
				if data.Value != nil {
					if err := db.SaveData(data.ItemName, *data.Value, at.ToInt64()); err != nil {
						slog.Error("Failed to save data",
							"item_name", data.ItemName,
							"value", *data.Value,
							"error", err)
					}
					dataBroker.Publish(common.SendMsgStruct{
						Type: data.Typ,
						Body: common.ItemNameDataTimeStruct{
							ItemName:       data.ItemName,
							DataTimeStruct: common.DataTimeStruct{Value: *data.Value, Millisecond: at},
						},
					}, nil)
				}
			}
		}()
	}
}
