package device

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
)

func init() {
	RegisterDevice("HMP155", &hMP155{})
}

type hMP155 struct{}

var hmp155Items = map[string]int{"air_humidity": 0, "air_temperature": 1}

func (hMP155) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	var conf struct {
		DeviceName string            `json:"device_name"`
		Addr       string            `json:"addr"`
		Cron       string            `json:"cron"`
		Items      map[string]string `json:"items"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	DevicesUartConn[conf.DeviceName] = conn
	var (
		err     error
		line    string
		input   = []byte("SEND " + conf.Addr + "\r\n")
		tmpData = make(map[string]*float64)
	)
	var job = func() map[string]*float64 {
		line, err = conn.ReadLine(input)
		if err != nil {
			slog.Error("IO error while reading from HMP155 device", "error", err)
			return nil
		}
		if line[0] == '*' {
			slog.Error("Device error from HMP155 device", "error", err)
			return nil
		}

		var rh, t *float64

		val := strings.TrimSpace(getStringInBetween(line, "RH=", "%RH"))
		if val == "" {
			slog.Error("Parse error for RH value from HMP155 device", "error", err)
			return nil
		} else if val[0] == '*' {
			slog.Error("Item error for RH value from HMP155 device", "error", err)
		} else {
			if f, err := strconv.ParseFloat(val, 64); err != nil {
				slog.Error("Parse error while converting RH value from HMP155 device", "error", err)
				return nil
			} else {
				rh = &f
			}
		}

		val = strings.TrimSpace(getStringInBetween(line, "T=", "'C"))
		if val == "" {
			slog.Error("Parse error for T value from HMP155 device", "error", err)
			return nil
		} else if val[0] == '*' {
			slog.Error("Item error for T value from HMP155 device", "error", err)
		} else {
			if f, err := strconv.ParseFloat(val, 64); err != nil {
				slog.Error("Parse error while converting T value from HMP155 device", "error", err)
				return nil
			} else {
				t = &f
			}
		}
		tmpData["air_humidity"] = rh
		tmpData["air_temperature"] = t
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, hmp155Items, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}

// HMP155 addr 0 to 99, Format: RH= 40.5 %RH T= 22.9 'C \r\n ("RH= %f %%RH T= %f 'C \r\n")
// err:(***)
