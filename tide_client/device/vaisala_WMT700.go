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
	RegisterDevice("WMT700", &wmt700{})
}

type wmt700 struct{}

var wmt700Items = map[string]int{"wind_speed": 0, "wind_direction": 1}

func (wmt700) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
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
		input   = []byte("$" + conf.Addr + "POLL,21\r\n")
		line    string
		tmpData = make(map[string]*float64)
	)

	var job = func() map[string]*float64 {
		line, err = conn.ReadLine(input)
		if err != nil {
			slog.Error("IO error while reading from WMT700 device", "error", err, "received", []byte(line))
			return nil
		}
		var ws, wd *float64

		val := strings.TrimSpace(getStringInBetween(line, "$", ","))
		if val == "" {
			slog.Error("Parse error for first value from WMT700 device", "error", err, "received", []byte(line))
			return nil
		}
		if f, err := strconv.ParseFloat(val, 64); err != nil {
			slog.Error("Parse error while converting first value from WMT700 device", "error", err, "received", []byte(line))
			return nil
		} else if f >= 999 {
			slog.Error("Item error for first value from WMT700 device (value >= 999)", "error", err, "received", []byte(line))
		} else {
			ws = &f
		}
		val = strings.TrimSpace(getStringInBetween(line, ",", "\r"))
		if val == "" {
			slog.Error("Parse error for second value from WMT700 device", "error", err, "received", []byte(line))
			return nil
		}
		if f, err := strconv.ParseFloat(val, 64); err != nil {
			slog.Error("Parse error while converting second value from WMT700 device", "error", err, "received", []byte(line))
			return nil
		} else if f >= 999 {
			slog.Error("Item error for second value from WMT700 device (value >= 999)", "error", err, "received", []byte(line))
		} else {
			wd = &f
		}
		tmpData["wind_speed"] = ws
		tmpData["wind_direction"] = wd
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, wmt700Items, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}

// WMT700 addr String with a maximum of 40 characters.
// When the WMT700 profile is used, WMT700 indicates missing readings by showing 999 in the data messages
//$aPOLL,y<CR><LF>("$%f,%f\r\n")
//$\ws,\wd\cr\lf
