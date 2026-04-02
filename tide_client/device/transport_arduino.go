package device

import (
	"encoding/json"
	"log/slog"
	"math"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	protocolarduino "tide/tide_client/protocol/arduino"
	"tide/tide_client/protocol/sdi12"
)

func init() {
	RegisterDevice("arduino", &arduino{})
}

type arduino struct{}

func (arduino) NewBusDevice(bus *connWrap.Bus, rawConf json.RawMessage) common.StringMapMap {
	arduinoSession := protocolarduino.NewSession(bus)
	sdi12Session := sdi12.NewSession(bus, sdi12.ModeArduino)
	var conf struct {
		Sdi12 []struct {
			Model  string          `json:"model"`
			Config json.RawMessage `json:"config"`
		}
		Analog []struct {
			DeviceName string `json:"device_name"`
			Pin        byte   `json:"pin"`
			Cron       string `json:"cron"`
			ItemName   string `json:"item_name"`
			ItemType   string `json:"item_type"`
		}
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(map[string]map[string]string)
	for _, subDevice := range conf.Sdi12 {
		subInfo := MustSDI12Device(subDevice.Model).NewSDI12Device(sdi12Session, subDevice.Config)
		MergeInfo(info, subInfo)
	}
	for _, item := range conf.Analog {
		MergeInfo(info, common.StringMapMap{item.DeviceName: map[string]string{item.ItemType: item.ItemName}})

		var job = func() *float64 {
			val, err := arduinoSession.AnalogRead(item.Pin)
			if err != nil {
				slog.Error("Error reading analog value from Arduino device", "error", err)
				return nil
			}
			return new(math.Round(float64(val)/1023*5*1000) / 1000)
		}
		AddCronJobWithOneItem(item.Cron, item.ItemName, job)
	}
	return info
}
