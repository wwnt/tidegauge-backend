package device

import (
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"math"
	"strconv"
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
		return new(Float32To64(math.Float32frombits(binary.BigEndian.Uint32(results[4:]))))
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return map[string]map[string]string{conf.DeviceName: {"water_distance": conf.ItemName}}
}
func Float32To64(val float32) float64 {
	// 1. 将 float32 转为字符串
	// 'g' 标记表示使用最紧凑的格式（指数或常规）
	// -1 表示让 strconv 自动决定需要的最小小数位
	// 32 表示这个数来源于 float32（这是关键，它会过滤掉 float32 精度之外的噪声）
	str := strconv.FormatFloat(float64(val), 'g', -1, 32)

	// 2. 将字符串解析为 float64
	res, _ := strconv.ParseFloat(str, 64)
	return res
}
