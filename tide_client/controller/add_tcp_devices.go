package controller

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/connWrap/tcp"
	"tide/tide_client/device"
)

func init() {
	RegisterConn("tcp", newTcpConn)
}

type tcpDeviceConfig struct {
	Addr        string          `json:"addr"`
	ReadTimeout uint32          `json:"read_timeout"`
	Model       string          `json:"model"`
	Config      json.RawMessage `json:"config"`
}

func newTcpConn(rawConf json.RawMessage) common.StringMapMap {
	var conf tcpDeviceConfig
	pkg.Must(json.Unmarshal(rawConf, &conf))

	connCommon, err := tcp.StartTcp(conf.Addr, conf.ReadTimeout)
	if err != nil {
		slog.Error("Connecting", "tcp", conf.Addr, "error", err)
		os.Exit(1)
	}
	bus := connWrap.NewBus(connCommon)
	slog.Info("Connection manager started", "tcp", conf.Addr)

	subInfo := device.MustBusDevice(conf.Model).NewBusDevice(bus, conf.Config)
	var info = make(common.StringMapMap)
	device.MergeInfo(info, subInfo)
	return info
}
