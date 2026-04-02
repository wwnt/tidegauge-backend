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

func (uartRs485) NewBusDevice(bus *connWrap.Bus, rawConf json.RawMessage) common.StringMapMap {
	var conf []struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(common.StringMapMap)
	for _, subDevice := range conf {
		subInfo := MustBusDevice(subDevice.Model).NewBusDevice(bus, subDevice.Config)
		MergeInfo(info, subInfo)
	}
	return info
}
