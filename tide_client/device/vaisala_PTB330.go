package device

import (
	"encoding/json"
	"strconv"
	"strings"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
)

func init() {
	RegisterDevice("PTB330", &ptb330{})
}

type ptb330 struct{}

var ptb330Items = map[string]int{"air_pressure": 0}

func (ptb330) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	var conf struct {
		Addr       string            `json:"addr"`
		DeviceName string            `json:"device_name"`
		Cron       string            `json:"cron"`
		Items      map[string]string `json:"items"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	DevicesUartConn[conf.DeviceName] = conn

	var (
		err     error
		line    string
		input   = []byte("SEND " + conf.Addr + "\r")
		tmpData = make(map[string]*float64)
	)
	job := func() map[string]*float64 {
		line, err = conn.ReadLine(input)
		if err != nil {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrIO, Received: []byte(line), Err: err})
			return nil
		}
		if line[0] == '*' {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrDevice, Received: []byte(line), Err: err})
			return nil
		}
		val := strings.TrimSpace(line)
		if f, err := strconv.ParseFloat(val, 64); err != nil {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrParse, Received: []byte(line), Err: err})
			return nil
		} else {
			tmpData["air_pressure"] = &f
		}
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, ptb330Items, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}

// err: (***)
// output: 1010.31\r\n
