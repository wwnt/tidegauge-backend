package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
)

func init() {
	RegisterDevice("uart-rs232", &uartRs232{})
}

type uartRs232 struct{}

func (uartRs232) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	conn.Typ = "uart-rs232"
	var conf struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(common.StringMapMap)

	subInfo := GetDevice(conf.Model).(Device).NewDevice(conn, conf.Config)
	MergeInfo(info, subInfo)

	return info
}
