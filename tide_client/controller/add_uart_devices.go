package controller

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/connWrap/uart"
	"tide/tide_client/device"
)

func init() {
	RegisterConn("uart", newUartConn)
}

type uartDeviceConfig struct {
	Port        string          `json:"port"`
	ReadTimeout uint32          `json:"read_timeout"`
	Mode        uart.Mode       `json:"mode"`
	Model       string          `json:"model"`
	Config      json.RawMessage `json:"config"`
}

func newUartConn(rawConf json.RawMessage) common.StringMapMap {
	var conf uartDeviceConfig
	pkg.Must(json.Unmarshal(rawConf, &conf))

	connCommon, err := uart.NewUart(conf.Port, conf.ReadTimeout, conf.Mode)
	if err != nil {
		slog.Error("Connecting", "port", conf.Port, "error", err)
		os.Exit(1)
	}
	slog.Info("Connected", "port", conf.Port)

	connU := connWrap.NewConnUtil(connCommon)
	subInfo := device.GetDevice(conf.Model).(device.Device).NewDevice(connU, conf.Config)
	var info = make(common.StringMapMap)
	device.MergeInfo(info, subInfo)
	return info
}
