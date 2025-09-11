package controller

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"tide/tide_client/device"

	"github.com/warthog618/go-gpiocdev"
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

	rpiGPIO, err := gpiocdev.NewChip(conf.Name)
	if err != nil {
		slog.Error("Connecting", "gpio", conf.Name, "error", err)
		os.Exit(1)
	}
	slog.Info("Connected", "gpio", conf.Name)

	project.RegisterReleaseFunc(func() { _ = rpiGPIO.Close() })

	var info = make(common.StringMapMap)
	for _, deviceConf := range conf.Config {
		subInfo := device.GetDevice(deviceConf.Model).(device.Device).NewDevice(rpiGPIO, deviceConf.Config)
		device.MergeInfo(info, subInfo)
	}
	return info
}
