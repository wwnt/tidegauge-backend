package controller

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/device"
)

func init() {
	RegisterConn("mock_conn", newMockConn)
}

type mockDeviceConfig struct {
	Config []struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	} `json:"config"`
}

func newMockConn(rawConf json.RawMessage) common.StringMapMap {
	var conf mockDeviceConfig
	pkg.Must(json.Unmarshal(rawConf, &conf))

	var info = make(common.StringMapMap)
	for _, deviceConf := range conf.Config {
		subInfo := device.GetDevice(deviceConf.Model).(device.Device).NewDevice(nil, deviceConf.Config)
		device.MergeInfo(info, subInfo)
	}
	return info
}
