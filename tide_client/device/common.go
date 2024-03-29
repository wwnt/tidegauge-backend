package device

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
)

var (
	DataReceive     = make(chan []itemData, 100)
	DevicesUartConn = make(map[string]*connWrap.ConnUtil)

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

func GetDevice(m string) any {
	return devices[m]
}

type itemData struct {
	Typ      common.MsgType
	ItemName string
	Value    *float64
}

type Device interface {
	NewDevice(conn any, rawConf json.RawMessage) common.StringMapMap
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
			global.Log.Errorf("The query interval is too short. items: %+v", items)
			return
		}
		defer inQuery.Store(false)

		tmpData = job()

		var sendData []itemData
		for itemType, itemName := range items {
			// if tmpData == nil, tmpData[itemType] == nil
			sendData = append(sendData, itemData{Typ: common.MsgData, ItemName: itemName, Value: tmpData[itemType]})
		}
		DataReceive <- sendData
	}
	pkg.Must2(global.CronJob.AddFunc(cron, jobWrap))
}

func AddCronJobWithOneItem(cron string, itemName string, job func() *float64) {
	if itemName == "" {
		global.Log.Fatalf("item_name is empty")
	}
	var (
		inQuery atomic.Bool
	)
	jobWrap := func() {
		// Determine if this device is being queried
		if !inQuery.CompareAndSwap(false, true) {
			global.Log.Errorf("The query interval is too short. item_name: %v", itemName)
			return
		}
		defer inQuery.Store(false)

		val := job()

		DataReceive <- []itemData{{Typ: common.MsgData, ItemName: itemName, Value: val}}
	}
	pkg.Must2(global.CronJob.AddFunc(cron, jobWrap))
}

func verifyItems(items map[string]string, provideItems map[string]int) {
	if len(items) == 0 {
		global.Log.Fatal("items empty")
	}
	for itemType := range items {
		if _, ok := provideItems[itemType]; !ok {
			global.Log.Fatal(itemType + " does not exist")
		}
	}
}
