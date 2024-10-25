package device

import (
	"encoding/json"
	"errors"
	"github.com/warthog618/go-gpiocdev"
	"syscall"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"tide/tide_client/global"
	"time"
)

func init() {
	RegisterDevice("RG13", &rg13{})
}

type rg13 struct{}

func (rg13) NewDevice(conn any, rawConf json.RawMessage) common.StringMapMap {
	gpio := conn.(*gpiocdev.Chip)
	var conf struct {
		DeviceName string `json:"device_name"`
		Pin        int    `json:"pin"`
		ItemName   string `json:"item_name"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var val = 0.2

	ll, err := gpio.RequestLines([]int{conf.Pin}, gpiocdev.WithPullUp, gpiocdev.WithRisingEdge,
		gpiocdev.WithDebounce(time.Millisecond*10),
		gpiocdev.WithEventHandler(func(evt gpiocdev.LineEvent) {
			DataReceive <- []itemData{{Typ: common.MsgGpioData, ItemName: conf.ItemName, Value: &val}}
		}))
	if err != nil {
		if errors.Is(err, syscall.Errno(22)) {
			global.Log.Error("Note that the WithPullUp option requires kernel V5.5 or later - check your kernel version.")
		}
		global.Log.Fatalf("RequestLine returned error: %s\n", err)
	}
	global.Log.Debugf("watch on gpio pin: %v", conf.Pin)
	project.RegisterReleaseFunc(func() { _ = ll.Close() })
	return common.StringMapMap{conf.DeviceName: map[string]string{"rain_gauge": conf.ItemName}}
}
