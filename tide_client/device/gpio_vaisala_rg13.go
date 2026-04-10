//go:build linux

package device

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"syscall"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

func init() {
	RegisterDevice("RG13", &rg13{})
}

type rg13 struct{}

func (rg13) NewGPIODevice(gpio *gpiocdev.Chip, rawConf json.RawMessage) common.StringMapMap {
	var conf struct {
		DeviceName string  `json:"device_name"`
		Pin        int     `json:"pin"`
		ItemName   string  `json:"item_name"`
		Cron       string  `json:"cron"`
		Resolution float64 `json:"resolution"`
		ActiveLow  bool    `json:"active_low"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	if conf.Cron == "" {
		conf.Cron = "@every 1m"
	}
	if conf.Resolution == 0 {
		// GPIO RG13 counts one selected edge per bucket tip, so the default stays at 0.2 mm/tip.
		conf.Resolution = 0.2
	}
	var counter atomic.Int64

	opts := []gpiocdev.LineReqOption{gpiocdev.WithPullUp, gpiocdev.WithRisingEdge,
		gpiocdev.WithDebounce(time.Millisecond * 50),
		gpiocdev.WithEventHandler(func(evt gpiocdev.LineEvent) {
			counter.Add(1)
		})}
	if conf.ActiveLow {
		opts = append(opts, gpiocdev.AsActiveLow)
	}
	ll, err := gpio.RequestLines([]int{conf.Pin}, opts...)
	if err != nil {
		if errors.Is(err, syscall.Errno(22)) {
			slog.Error("Note that the WithPullUp option requires kernel V5.5 or later - check your kernel version.")
		}
		slog.Error("RequestLine returned error", "error", err)
		os.Exit(1)
	}
	slog.Info("watch on gpio pin", "pin", conf.Pin)
	project.RegisterReleaseFunc(func() { _ = ll.Close() })

	job := func() *float64 {
		tips := counter.Swap(0)
		// Each counted edge represents one tip, and the counter is reset after every scheduled upload.
		return new(float64(tips) * conf.Resolution)
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)

	return common.StringMapMap{conf.DeviceName: map[string]string{"rain_volume": conf.ItemName}}
}
