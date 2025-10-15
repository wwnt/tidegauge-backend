package device

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
	"tide/pkg"

	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/devices/v3/ads1x15"
)

func init() {
	RegisterDevice("ads1115", &ads1115{})
}

type ads1115 struct{}

func (ads1115) NewDevice(conn any, rawConf json.RawMessage) common.StringMapMap {
	bus := conn.(i2c.Bus)
	var conf struct {
		Addr  uint16 `json:"addr"`
		Items []struct {
			Channel    ads1x15.Channel `json:"channel"`
			DeviceName string          `json:"device_name"`
			Cron       string          `json:"cron"`
			ItemType   string          `json:"item_type"`
			ItemName   string          `json:"item_name"`
			ChM        float64         `json:"ch_m"`
			ChB        float64         `json:"ch_b"`
		}
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	dev, err := ads1x15.NewADS1115(bus, &ads1x15.Opts{I2cAddress: conf.Addr})
	if err != nil {
		slog.Error("Failed to create ADS1115 device", "i2c_address", conf.Addr, "error", err)
		os.Exit(1)
	}
	var info = make(map[string]map[string]string)
	for _, item := range conf.Items {
		MergeInfo(info, common.StringMapMap{item.DeviceName: {item.ItemType: item.ItemName}})

		pin, err := dev.PinForChannel(item.Channel, 5*physic.Volt, 1*physic.Hertz, ads1x15.BestQuality)
		if err != nil {
			slog.Error("Failed to create pin for ADS1115 channel",
				"device_name", item.DeviceName,
				"channel", item.Channel,
				"item_name", item.ItemName,
				"error", err)
			os.Exit(1)
		}
		var chM = item.ChM
		var chB = item.ChB
		var job = func() *float64 {
			sample, err := pin.Read()
			if err != nil {
				slog.Error("Failed to read from ADS1115 pin",
					"item_name", item.ItemName,
					"channel", item.Channel,
					"error", err)
				return nil
			}
			var f = (float64(sample.V)/float64(physic.Volt))*chM + chB
			return &f
		}
		AddCronJobWithOneItem(item.Cron, item.ItemName, job)
	}
	return info
}
