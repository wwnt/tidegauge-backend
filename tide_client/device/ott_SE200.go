package device

import (
	"encoding/json"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
	"time"
)

func init() {
	RegisterDevice("SE200", &se200{})
}

type se200 struct{}

func (se200) NewDevice(c any, rawConf json.RawMessage) map[string]map[string]string {
	conn := c.(*connWrap.ConnUtil)
	var conf struct {
		DeviceName    string `json:"device_name"`
		Addr          string `json:"addr"`
		ExtraWakeTime byte   `json:"extra_wake_time"`
		Cron          string `json:"cron"`
		ItemName      string `json:"item_name"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	var (
		err    error
		output = conf.Addr + "00101\r\n" + conf.Addr + "\r\n"
	)
	var job = func() *float64 {
		err = conn.SDI12ConcurrentMeasurement(conf.Addr, conf.ExtraWakeTime, output, time.Second)
		if err != nil {
			global.Log.Error(err)
			return nil
		}
		//2+01.001\r\n
		values, err := conn.GetSDI12Data(conf.Addr, conf.ExtraWakeTime, 1)
		if err != nil {
			global.Log.Error(err)
			return nil
		}
		return values[0]
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return map[string]map[string]string{conf.DeviceName: {"water_distance": conf.ItemName}}
}
