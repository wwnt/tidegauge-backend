package device

import (
	"encoding/json"
	"github.com/warthog618/gpiod"
	"syscall"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"tide/tide_client/global"
	"time"
)

func init() {
	RegisterDevice("DRD11A", &dRD11A{})
}

type dRD11A struct{}

func (dRD11A) Analog() float64 {
	return 0
}

func (dRD11A) NewDevice(conn interface{}, rawConf json.RawMessage) common.StringMapMap {
	gpio := conn.(*gpiod.Chip)
	var conf struct {
		DeviceName string `json:"device_name"`
		Pin        int    `json:"pin"`
		ItemName   string `json:"item_name"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var rain, notRain float64 = 1, 0

	ll, err := gpio.RequestLines([]int{conf.Pin}, gpiod.WithPullUp, gpiod.WithBothEdges,
		gpiod.WithDebounce(time.Millisecond*10),
		gpiod.WithEventHandler(func(evt gpiod.LineEvent) {
			if evt.Type == gpiod.LineEventRisingEdge {
				DataReceive <- []itemData{{Typ: common.MsgGpioData, ItemName: conf.ItemName, Value: &notRain}}
			}
			DataReceive <- []itemData{{Typ: common.MsgGpioData, ItemName: conf.ItemName, Value: &rain}}
		}))
	if err != nil {
		if err == syscall.Errno(22) {
			global.Log.Error("Note that the WithPullDown option requires kernel V5.5 or later - check your kernel version.")
		}
		global.Log.Fatalf("RequestLine returned error: %s\n", err)
	}
	global.Log.Debugf("watch on gpio pin: %v", conf.Pin)
	project.RegisterReleaseFunc(func() { _ = ll.Close() })
	return common.StringMapMap{conf.DeviceName: map[string]string{"precipitation_detection": conf.ItemName}}
}
