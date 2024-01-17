package device

import (
	"encoding/binary"
	"encoding/json"
	"github.com/wwnt/modbus"
	"math"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
)

func init() {
	RegisterDevice("VEGAPULS61", &vegaPULS61{})
}

type vegaPULS61 struct{}

func (vegaPULS61) NewDevice(c any, rawConf json.RawMessage) map[string]map[string]string {
	conn := c.(*connWrap.ConnUtil)
	var conf struct {
		DeviceName string `json:"device_name"`
		Addr       byte   `json:"addr"`
		Cron       string `json:"cron"`
		ItemName   string `json:"item_name"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	h := modbus.NewRTUClientHandler(conn)
	h.SlaveId = conf.Addr
	var (
		err     error
		client  = modbus.NewClient(h)
		results []byte
	)
	var job = func() *float64 {
		conn.Lock()
		defer conn.Unlock() // must be locked to prevent simultaneous operations
		results, err = client.ReadInputRegisters(2000, 4)
		if err != nil {
			global.Log.Error(err)
			return nil
		}
		if results[0] != 0 {
			global.Log.Errorf("% x\n", results)
			return nil
		}
		var val = float64(math.Float32frombits(binary.BigEndian.Uint32(results[4:])))
		return &val
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return map[string]map[string]string{conf.DeviceName: {"water_distance": conf.ItemName}}
}
