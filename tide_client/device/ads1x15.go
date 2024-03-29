package device

import (
	"encoding/json"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/devices/v3/ads1x15"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/global"
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
		}
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	dev, _ := ads1x15.NewADS1115(bus, &ads1x15.Opts{I2cAddress: conf.Addr})
	var info = make(map[string]map[string]string)
	for _, item := range conf.Items {
		MergeInfo(info, common.StringMapMap{item.DeviceName: {item.ItemType: item.ItemName}})

		pin, err := dev.PinForChannel(item.Channel, 5*physic.Volt, 1*physic.Hertz, ads1x15.SaveEnergy)
		if err != nil {
			global.Log.Fatal(err)
		}
		var job = func() *float64 {
			sample, err := pin.Read()
			if err != nil {
				global.Log.Error(err)
				return nil
			} else {
				var f = float64(sample.V / physic.Volt)
				return &f
			}
		}
		AddCronJobWithOneItem(item.Cron, item.ItemName, job)
	}
	return info
}
