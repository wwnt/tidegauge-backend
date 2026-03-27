package device

import (
	"encoding/json"
	"log/slog"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"time"
)

func init() {
	RegisterDevice("PLS-C", &plsC{
		minLevel: -1.0,
		maxLevel: 4.0,
		minTemp:  -8.0,
		maxTemp:  40.0,
		minCond:  0.1,
		maxCond:  100.0,
		minSal:   10.0,
		maxSal:   50.0,
		minTDS:   10.0,
		maxTDS:   50.0,
	})
}

type plsC struct {
	minLevel float64
	maxLevel float64
	minTemp  float64
	maxTemp  float64
	minCond  float64
	maxCond  float64
	minSal   float64
	maxSal   float64
	minTDS   float64
	maxTDS   float64
}

var PLSCItems = map[string]int{"water_level": 0, "water_temperature": 1, "water_conductivity": 2, "water_salinity": 3, "water_total_dissolved_solids": 4}

func (d *plsC) NewDevice(c any, rawConf json.RawMessage) map[string]map[string]string {
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
			slog.Error("Failed to perform SDI-12 concurrent measurement", "device", "PLS-C", "addr", conf.Addr, "error", err)
			return nil
		}
		values, err := conn.GetSDI12Data(conf.Addr, conf.ExtraWakeTime, 5)
		if err != nil {
			slog.Error("Failed to get SDI-12 data", "device", "PLS-C", "addr", conf.Addr, "error", err)
			return nil
		}

		if values[0] != nil {
			if d.minLevel < *values[0] && *values[0] < d.maxLevel {
				tmpData["water_level"] = values[0]
			} else {
				tmpData["water_level"] = nil
			}
		} else {
			tmpData["water_level"] = nil
		}
		if values[1] != nil {
			if d.minTemp < *values[1] && *values[1] < d.maxTemp {
				tmpData["water_temperature"] = values[1]
			} else {
				tmpData["water_temperature"] = nil
			}
		} else {
			tmpData["water_temperature"] = nil
		}
		if values[2] != nil {
			if d.minCond < *values[2] && *values[2] < d.maxCond {
				tmpData["water_conductivity"] = values[2]
			} else {
				tmpData["water_conductivity"] = nil
			}
		} else {
			tmpData["water_conductivity"] = nil
		}
		if values[3] != nil {
			if d.minSal < *values[3] && *values[3] < d.maxSal {
				tmpData["water_salinity"] = values[3]
			} else {
				tmpData["water_salinity"] = nil
			}
		} else {
			tmpData["water_salinity"] = nil
		}
		if values[4] != nil {
			if d.minTDS < *values[4] && *values[4] < d.maxTDS {
				tmpData["water_total_dissolved_solids"] = values[4]
			} else {
				tmpData["water_total_dissolved_solids"] = nil
			}
		} else {
			tmpData["water_total_dissolved_solids"] = nil
		}
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, PLSCItems, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}

//1-0.001+19.51+0.00\r\n
//1+0.01+0.000\r\n
