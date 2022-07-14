package controller

import (
	"encoding/json"
	"github.com/warthog618/gpiod"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"tide/tide_client/device"
	"tide/tide_client/global"
)

func init() {
	RegisterConn("gpio", newGpioConn)
}

type gpioDevicesConfig struct {
	Name   string `json:"name"`
	Config []struct {
		Model  string          `json:"model"`
		Config json.RawMessage `json:"config"`
	} `json:"config"`
}

func newGpioConn(rawConf json.RawMessage) common.StringMapMap {
	var conf gpioDevicesConfig
	pkg.Must(json.Unmarshal(rawConf, &conf))

	rpiGPIO, err := gpiod.NewChip(conf.Name)
	if err != nil {
		global.Log.Fatal(err)
	}
	project.RegisterReleaseFunc(func() { _ = rpiGPIO.Close() })
	var (
		info = make(common.StringMapMap)
	)
	for _, deviceConf := range conf.Config {
		subInfo := device.GetDevice(deviceConf.Model).(device.Device).NewDevice(rpiGPIO, deviceConf.Config)
		device.MergeInfo(info, subInfo)
	}
	return info
}
