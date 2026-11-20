package controller

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"sort"
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

	itemNames := addDevices()

	// 消费者必须先于启动期清理运行：addDevices 之后各设备的 cron 任务已经开始往
	// device.DataReceive（容量 2000）里投递，而首次清理（旧积压的状态日志、FTP 目录遍历）
	// 可能持续数分钟；没人消费时通道写满，采集任务会阻塞并跳过后续周期，造成数据缺口。
	go receiveData(dataBroker)

	scheduleRemoveOutdatedData(itemNames)

	addRpiStatus(dataBroker)

	for name := range global.Config.Cameras.List {
		stationInfo.Cameras = append(stationInfo.Cameras, name)
	}
	// map 迭代顺序随机；排序保证每次重启发给服务端的 station info 一致。
	sort.Strings(stationInfo.Cameras)

	go func() {
		// 重连退避：连接尝试失败时按 10s -> 20s -> 40s ... 上限 10 分钟递增；
		// 连接维持了较长时间（正常会话）则重置回 10s。
		const baseDelay, maxDelay = 10 * time.Second, 10 * time.Minute
		const stableConn = time.Minute // 连接维持超过该时长视为成功过一次
		delay := baseDelay
		for {
			started := time.Now()
			if !runSyncV2ClientOnce(dataBroker) {
				client(dataBroker)
			}
			if time.Since(started) > stableConn {
				delay = baseDelay
			}
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}()
}

var itemsStatus = make(map[string]common.StatusChangeStruct)

// addDevices 按配置装载全部设备，并为每个 item 建表。返回本站全部 item 名——
// 它们同时是 SQLite 里的数据表名，磁盘清理按这份清单执行。
func addDevices() []string {
	var info = stationInfo.Devices
	for connType, files := range global.Config.Devices {
		connFunc, ok := GetRegConn(connType).(func(json.RawMessage) common.StringMapMap)
		if !ok {
			slog.Error("Unknown conn type in config", "conn_type", connType,
				"known_types", []string{"uart", "tcp", "i2c", "gpio"})
			os.Exit(1)
		}
		for _, filename := range files {
			rawConf, err := os.ReadFile(filename)
			if err != nil {
				slog.Error("Failed to read config file", "filename", filename, "error", err)
				os.Exit(1)
			}
			subInfo := connFunc(rawConf)
			device.MergeInfo(info, subInfo)
		}
	}

	// check duplicate
	var itemNames []string
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
			itemNames = append(itemNames, name)
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
	return itemNames
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

				// NaN/±Inf 不是有效读数：按缺失处理（状态记为异常、不落库）。±Inf 一旦写进
				// SQLite，之后每次 v1 重放都会在 JSON 编码时失败，站点将一直无法同步。
				if data.Value != nil && (math.IsNaN(*data.Value) || math.IsInf(*data.Value, 0)) {
					slog.Warn("Discarding non-finite reading", "item_name", data.ItemName, "value", *data.Value)
					data.Value = nil
				}

				if data.Typ == common.MsgData {
					// 缺失读数（nil）视为异常，有读数则视为正常；
					// 状态翻转时落库并广播，未翻转则跳过。
					status := common.Normal
					if data.Value == nil {
						status = common.Abnormal
					}
					maybeFlipItemStatus(dataBroker, data.ItemName, at, status)
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

// maybeFlipItemStatus 在 item 的状态与缓存不同（即发生翻转）时，把新状态写入
// item_status_log 并发布到 broker；未翻转则什么都不做。必须在持有 ingestMu 时调用。
//
// 缓存只在落库成功后更新：落库失败（例如清理任务正占着写锁）时缓存保持旧值，
// 下一条读数会再次判定为翻转并重试，这次翻转不会被永久吞掉。
func maybeFlipItemStatus(dataBroker *pubsub.Broker, itemName string, at custype.UnixMs, status common.Status) {
	if itemsStatus[itemName].Status == status {
		return
	}
	rowId, err := db.SaveItemStatusLog(itemName, status, at.ToInt64())
	if err != nil {
		slog.Error("Failed to save item status log",
			"item_name", itemName,
			"status", status,
			"error", err)
		return
	}
	itemsStatus[itemName] = common.StatusChangeStruct{Status: status, ChangedAt: at}
	dataBroker.Publish(common.SendMsgStruct{Type: common.MsgItemStatus,
		Body: common.RowIdItemStatusStruct{
			RowId:            rowId,
			ItemStatusStruct: common.ItemStatusStruct{ItemName: itemName, StatusChangeStruct: common.StatusChangeStruct{Status: status, ChangedAt: at}},
		}}, nil)
}
