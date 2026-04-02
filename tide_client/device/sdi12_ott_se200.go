package device

import (
	"encoding/json"
	"log/slog"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/protocol/sdi12"
	"time"
)

func init() {
	RegisterDevice("SE200", &se200{
		minLevel: -30.0,
		maxLevel: 30.0,
	})
}

type se200 struct {
	minLevel float64
	maxLevel float64
}

var (
	_ BusDevice   = (*se200)(nil)
	_ SDI12Device = (*se200)(nil)
)

func (d *se200) NewBusDevice(bus *connWrap.Bus, rawConf json.RawMessage) common.StringMapMap {
	return d.NewSDI12Device(sdi12.NewSession(bus, sdi12.ModeNative), rawConf)
}

func (d *se200) NewSDI12Device(session *sdi12.Session, rawConf json.RawMessage) map[string]map[string]string {
	var conf struct {
		DeviceName    string  `json:"device_name"`
		Addr          string  `json:"addr"`
		ExtraWakeTime byte    `json:"extra_wake_time"`
		Cron          string  `json:"cron"`
		ItemName      string  `json:"item_name"`
		Correction    float64 `json:"correction"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	var (
		err    error
		output = conf.Addr + "00101\r\n" + conf.Addr + "\r\n"
	)
	var job = func() *float64 {
		err = session.ConcurrentMeasurement(conf.Addr, conf.ExtraWakeTime, output, time.Second)
		if err != nil {
			slog.Error("Failed to perform SDI-12 concurrent measurement", "device", "SE200", "addr", conf.Addr, "error", err)
			return nil
		}
		//2+01.001\r\n
		values, err := session.GetData(conf.Addr, conf.ExtraWakeTime, 1)
		if err != nil {
			slog.Error("Failed to get SDI-12 data", "device", "SE200", "addr", conf.Addr, "error", err)
			return nil
		}
		if values[0] != nil {
			val := *values[0] + conf.Correction
			if d.minLevel < val && val < d.maxLevel {
				return &val
			} else {
				return nil
			}
		}
		return nil
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return map[string]map[string]string{conf.DeviceName: {"water_distance": conf.ItemName}}
}
