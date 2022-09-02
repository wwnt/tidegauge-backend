package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
)

func init() {
	RegisterDevice("uart-sdi12", &uartSdi12{})
}

type uartSdi12 struct{}

func (d uartSdi12) NewDevice(c interface{}, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	conn.Typ = "uart-sdi12"
	var conf []struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(map[string]map[string]string)
	for _, subDevice := range conf {
		subInfo := GetDevice(subDevice.Model).(Device).NewDevice(conn, subDevice.Config)
		MergeInfo(info, subInfo)
	}
	return info
}
