package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
)

func init() {
	RegisterDevice("uart-rs485", &uartRs485{})
}

type uartRs485 struct{}

func (uartRs485) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	conn.Typ = "uart-rs485"
	var conf []struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(common.StringMapMap)
	for _, subDevice := range conf {
		subInfo := GetDevice(subDevice.Model).(Device).NewDevice(conn, subDevice.Config)
		MergeInfo(info, subInfo)
	}
	return info
}
