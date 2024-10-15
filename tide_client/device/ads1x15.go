package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/global"

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
	dev, _ := ads1x15.NewADS1115(bus, &ads1x15.Opts{I2cAddress: conf.Addr})
	var info = make(map[string]map[string]string)
	for _, item := range conf.Items {
		MergeInfo(info, common.StringMapMap{item.DeviceName: {item.ItemType: item.ItemName}})

		pin, err := dev.PinForChannel(item.Channel, 5*physic.Volt, 1*physic.Hertz, ads1x15.BestQuality)
		if err != nil {
			global.Log.Fatal(err)
		}
		var ch_m float64 = item.ChM
		var ch_b float64 = item.ChB
		var job = func() *float64 {
			sample, err := pin.Read()
			if err != nil {
				global.Log.Error(err)
				return nil
			} else {
				var f = (float64(sample.V)/float64(physic.Volt))*ch_m + ch_b
				return &f
			}
		}
		AddCronJobWithOneItem(item.Cron, item.ItemName, job)
	}
	return info
}
