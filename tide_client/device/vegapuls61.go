package device

import (
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"math"
	"tide/pkg"
	"tide/tide_client/connWrap"

	"github.com/wwnt/modbus"
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
			slog.Error("Error reading input registers from VEGAPULS61 device", "error", err)
			return nil
		}
		if results[0] != 0 {
			slog.Error("Non-zero result from VEGAPULS61 device", "results", results)
			return nil
		}
		var val = float64(math.Float32frombits(binary.BigEndian.Uint32(results[4:])))
		return &val
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return map[string]map[string]string{conf.DeviceName: {"water_distance": conf.ItemName}}
}
