package device

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"syscall"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

func init() {
	RegisterDevice("DRD11A", &dRD11A{})
}

type dRD11A struct{}

func (dRD11A) NewDevice(conn any, rawConf json.RawMessage) common.StringMapMap {
	gpio := conn.(*gpiocdev.Chip)
	var conf struct {
		DeviceName string `json:"device_name"`
		Pin        int    `json:"pin"`
		ItemName   string `json:"item_name"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var rain, notRain float64 = 1, 0

	ll, err := gpio.RequestLines([]int{conf.Pin}, gpiocdev.WithPullUp, gpiocdev.WithBothEdges,
		gpiocdev.WithDebounce(time.Millisecond*10),
		gpiocdev.WithEventHandler(func(evt gpiocdev.LineEvent) {
			if evt.Type == gpiocdev.LineEventRisingEdge {
				DataReceive <- []itemData{{Typ: common.MsgGpioData, ItemName: conf.ItemName, Value: &notRain}}
			}
			DataReceive <- []itemData{{Typ: common.MsgGpioData, ItemName: conf.ItemName, Value: &rain}}
		}))
	if err != nil {
		if errors.Is(err, syscall.Errno(22)) {
			slog.Error("Note that the WithPullUp option requires kernel V5.5 or later - check your kernel version.")
		}
		slog.Error("RequestLine returned error", "error", err)
		os.Exit(1)
	}
	slog.Debug("watch on gpio pin", "pin", conf.Pin)
	project.RegisterReleaseFunc(func() { _ = ll.Close() })
	return common.StringMapMap{conf.DeviceName: map[string]string{"precipitation_detection": conf.ItemName}}
}
