package device

import (
	"encoding/json"
	"tide/common"
	"tide/pkg"
)

func init() {
	RegisterDevice("test_i2c", &testI2c{})
}

type testI2c struct{}

func (testI2c) NewDevice(_ any, rawConf json.RawMessage) common.StringMapMap {
	var conf struct {
		Items []struct {
			DeviceName string `json:"device_name"`
			Cron       string `json:"cron"`
			ItemType   string `json:"item_type"`
			ItemName   string `json:"item_name"`
		}
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	var info = make(map[string]map[string]string)
	for _, item := range conf.Items {
		MergeInfo(info, common.StringMapMap{item.DeviceName: {item.ItemType: item.ItemName}})

		var i = 0
		var job = func() *float64 {
			if i == 5 {
				i = 0
				return nil
			}
			i++
			return new(float64(i))
		}
		AddCronJobWithOneItem(item.Cron, item.ItemName, job)
	}
	return info
}
