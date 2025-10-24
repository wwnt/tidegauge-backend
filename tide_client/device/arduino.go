package device

import (
	"encoding/json"
	"log/slog"
	"math"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
)

func init() {
	RegisterDevice("arduino", &arduino{})
}

type arduino struct{}

func (arduino) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	conn.Typ = "arduino"
	var conf struct {
		Sdi12 []struct {
			Model  string          `json:"model"`
			Config json.RawMessage `json:"config"`
		}
		Analog []struct {
			DeviceName string `json:"device_name"`
			Model      string `json:"model"`
			Pin        byte   `json:"pin"`
			Cron       string `json:"cron"`
			ItemName   string `json:"item_name"`
			ItemType   string `json:"item_type"`
		}
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(map[string]map[string]string)
	for _, subDevice := range conf.Sdi12 {
		subInfo := GetDevice(subDevice.Model).(Device).NewDevice(conn, subDevice.Config)
		MergeInfo(info, subInfo)
	}
	for _, item := range conf.Analog {
		MergeInfo(info, common.StringMapMap{item.DeviceName: map[string]string{item.ItemType: item.ItemName}})

		var job = func() *float64 {
			val, err := conn.AnalogRead(item.Pin)
			if err != nil {
				slog.Error("Error reading analog value from Arduino device", "error", err)
				return nil
			}
			var f = math.Round(float64(val)/1023*5*1000) / 1000
			return &f
		}
		AddCronJobWithOneItem(item.Cron, item.ItemName, job)
	}
	return info
}
