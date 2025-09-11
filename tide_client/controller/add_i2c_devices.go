package controller

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/device"

	"periph.io/x/host/v3/sysfs"
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
		slog.Error("Connecting", "i2c", conf.BusNumber, "error", err)
		os.Exit(1)
	}
	slog.Info("Connected", "i2c", conf.BusNumber)

	var info = make(common.StringMapMap)
	for _, deviceConf := range conf.Config {
		subInfo := device.GetDevice(deviceConf.Model).(device.Device).NewDevice(bus, deviceConf.Config)
		device.MergeInfo(info, subInfo)
	}
	return info
}
