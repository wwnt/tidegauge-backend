package device

import (
	"encoding/json"
	"log/slog"
	"tide/pkg"
	"tide/tide_client/connWrap"
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

func (d *se200) NewDevice(c any, rawConf json.RawMessage) map[string]map[string]string {
	conn := c.(*connWrap.ConnUtil)
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
		err = conn.SDI12ConcurrentMeasurement(conf.Addr, conf.ExtraWakeTime, output, time.Second)
		if err != nil {
			slog.Error("", "error", err)
			return nil
		}
		//2+01.001\r\n
		values, err := conn.GetSDI12Data(conf.Addr, conf.ExtraWakeTime, 1)
		if err != nil {
			slog.Error("", "error", err)
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
