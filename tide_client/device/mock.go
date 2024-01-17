package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
)

func init() {
	RegisterDevice("gpioTestDevice", &gpioTestDevice{})
}

type gpioTestDevice struct{}

func (gpioTestDevice) NewDevice(_ any, rawConf json.RawMessage) common.StringMapMap {
	var conf struct {
		DeviceName string `json:"device_name"`
		Pin        int    `json:"pin"`
		ItemName   string `json:"item_name"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	DataReceive <- []itemData{
		{Typ: common.MsgGpioData, ItemName: conf.ItemName, Value: Float64P(1)},
	}

	return common.StringMapMap{conf.DeviceName: {"precipitation_detection": conf.ItemName}}
}

func init() {
	RegisterDevice("uartTestDevice", &uartTestDevice{})
}

type uartTestDevice struct{}

var uartTestDeviceItems = map[string]int{"air_humidity": 0, "air_temperature": 1}

func (uartTestDevice) NewDevice(_ any, rawConf json.RawMessage) common.StringMapMap {
	var conf struct {
		Addr       string            `json:"addr"`
		DeviceName string            `json:"device_name"`
		Cron       string            `json:"cron"`
		Items      map[string]string `json:"items"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	var (
		randomData    = []*float64{Float64P(-1.1), Float64P(0), Float64P(1.1), nil}
		randomDataLen = len(randomData)
		i             = 0
		tmpData       = make(map[string]*float64)
	)

	job := func() map[string]*float64 {
		for itemType := range conf.Items {
			tmpData[itemType] = randomData[i]
		}
		if i == randomDataLen-1 {
			i = 0
		} else {
			i++
		}
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, uartTestDeviceItems, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}
