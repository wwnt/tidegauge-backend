package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
	"time"
)

func init() {
	RegisterDevice("PLS-C", &pls_c{})
}

type pls_c struct{}

var PLSCItems = map[string]int{"water_level": 0, "water_temperature": 1, "water_conductivity": 2, "water_salinity": 3, "water_total_dissolved_solids": 4}

func (pls_c) NewDevice(c interface{}, rawConf json.RawMessage) map[string]map[string]string {
	conn := c.(*connWrap.ConnUtil)
	var conf struct {
		Addr          string            `json:"addr"`
		ExtraWakeTime byte              `json:"extra_wake_time"`
		DeviceName    string            `json:"device_name"`
		Cron          string            `json:"cron"`
		Items         map[string]string `json:"items"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	var (
		err     error
		output  = conf.Addr + "00505\r\n"
		tmpData = make(map[string]*float64)
	)
	var job = func() map[string]*float64 {
		err = conn.SDI12ConcurrentMeasurement(conf.Addr, conf.ExtraWakeTime, output, 5*time.Second)
		if err != nil {
			global.Log.Error(err)
			return nil
		}
		values, err := conn.GetSDI12Data(conf.Addr, conf.ExtraWakeTime, 5)
		if err != nil {
			global.Log.Error(err)
			return nil
		}
		tmpData["water_level"] = values[0]
		tmpData["water_temperature"] = values[1]
		tmpData["water_conductivity"] = values[2]
		tmpData["water_salinity"] = values[3]
		tmpData["water_total_dissolved_solids"] = values[4]
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, PLSCItems, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}

//1-0.001+19.51+0.00\r\n
//1+0.01+0.000\r\n
