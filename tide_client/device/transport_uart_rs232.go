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

func (uartRs232) NewBusDevice(bus *connWrap.Bus, rawConf json.RawMessage) common.StringMapMap {
	var conf struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(common.StringMapMap)

	subInfo := MustBusDevice(conf.Model).NewBusDevice(bus, conf.Config)
	MergeInfo(info, subInfo)

	return info
}
