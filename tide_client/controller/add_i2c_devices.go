package controller

import (
	"encoding/json"
	"periph.io/x/host/v3/sysfs"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/device"
	"tide/tide_client/global"
)

func init() {
	RegisterConn("i2c", newI2cConn)
}

type i2cDeviceConfig struct {
	BusNumber int `json:"bus_number"`
	Config    []struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	} `json:"config"`
}

func newI2cConn(rawConf json.RawMessage) common.StringMapMap {
	var conf i2cDeviceConfig
	pkg.Must(json.Unmarshal(rawConf, &conf))
	bus, err := sysfs.NewI2C(conf.BusNumber)
	if err != nil {
		panic(err)
	}
	global.Log.Info("open i2c", conf.BusNumber)
	var info = make(common.StringMapMap)
	for _, deviceConf := range conf.Config {
		subInfo := device.GetDevice(deviceConf.Model).(device.Device).NewDevice(bus, deviceConf.Config)
		device.MergeInfo(info, subInfo)
	}
	return info
}
