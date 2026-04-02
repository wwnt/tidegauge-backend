package device

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"tide/common"
	"tide/pkg"
	"tide/pkg/custype"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
	"tide/tide_client/protocol/sdi12"
	"time"

	"periph.io/x/conn/v3/i2c"
)

var (
	DataReceive = make(chan []itemData, 2000)

	devicesMu sync.RWMutex
	devices   = make(map[string]any)
)

func RegisterDevice(name string, d any) {
	devicesMu.Lock()
	defer devicesMu.Unlock()
	if d == nil {
		panic("Register device is nil")
	}
	if _, dup := devices[name]; dup {
		panic("Register called twice for device " + name)
	}
	devices[name] = d
}

type BusDevice interface {
	NewBusDevice(bus *connWrap.Bus, rawConf json.RawMessage) common.StringMapMap
}

type I2CDevice interface {
	NewI2CDevice(bus i2c.Bus, rawConf json.RawMessage) common.StringMapMap
}

type SDI12Device interface {
	NewSDI12Device(session *sdi12.Session, rawConf json.RawMessage) common.StringMapMap
}

func MustBusDevice(name string) BusDevice {
	device, ok := getRegisteredDevice(name).(BusDevice)
	if !ok {
		slog.Error("Device model does not support requested capability", "model", name, "capability", "shared bus")
		os.Exit(1)
		return nil
	}
	return device
}

func MustI2CDevice(name string) I2CDevice {
	device, ok := getRegisteredDevice(name).(I2CDevice)
	if !ok {
		slog.Error("Device model does not support requested capability", "model", name, "capability", "i2c")
		os.Exit(1)
		return nil
	}
	return device
}

func MustSDI12Device(name string) SDI12Device {
	device, ok := getRegisteredDevice(name).(SDI12Device)
	if !ok {
		slog.Error("Device model does not support requested capability", "model", name, "capability", "sdi-12")
		os.Exit(1)
		return nil
	}
	return device
}

func getRegisteredDevice(name string) any {
	devicesMu.RLock()
	defer devicesMu.RUnlock()

	device, ok := devices[name]
	if !ok {
		slog.Error("Unknown device model", "model", name)
		os.Exit(1)
	}
	return device
}

type itemData struct {
	At custype.UnixMs

	Typ      common.MsgType
	ItemName string
	Value    *float64
}

func nowMs() custype.UnixMs {
	return custype.ToUnixMs(time.Now())
}

func AddCronJob(cron string, items map[string]string, provideItems map[string]int, job func() map[string]*float64) {
	verifyItems(items, provideItems)
	var (
		inQuery atomic.Bool
		tmpData map[string]*float64
	)

	jobWrap := func() {
		// Determine if this device is being queried
		if !inQuery.CompareAndSwap(false, true) {
			slog.Warn("Query interval too short", "items", items)
			return
		}
		defer inQuery.Store(false)

		tmpData = job()
		at := nowMs()

		var sendData []itemData
		for itemType, itemName := range items {
			// if tmpData == nil, tmpData[itemType] == nil
			sendData = append(sendData, itemData{At: at, Typ: common.MsgData, ItemName: itemName, Value: tmpData[itemType]})
		}
		DataReceive <- sendData
	}
	pkg.Must2(global.CronJob.AddFunc(cron, jobWrap))
}

func AddCronJobWithOneItem(cron string, itemName string, job func() *float64) {
	if itemName == "" {
		slog.Error("Item name cannot be empty")
		os.Exit(1)
	}
	var (
		inQuery atomic.Bool
	)
	jobWrap := func() {
		// Determine if this device is being queried
		if !inQuery.CompareAndSwap(false, true) {
			slog.Warn("Query interval too short", "item_name", itemName)
			return
		}
		defer inQuery.Store(false)

		val := job()

		DataReceive <- []itemData{{At: nowMs(), Typ: common.MsgData, ItemName: itemName, Value: val}}
	}
	pkg.Must2(global.CronJob.AddFunc(cron, jobWrap))
}

func verifyItems(items map[string]string, provideItems map[string]int) {
	if len(items) == 0 {
		slog.Error("Items cannot be empty")
		os.Exit(1)
	}
	for itemType := range items {
		if _, ok := provideItems[itemType]; !ok {
			slog.Error("Item type does not exist", "item_type", itemType)
			os.Exit(1)
		}
	}
}
