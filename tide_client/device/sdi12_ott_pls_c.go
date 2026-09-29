package device

import (
	"encoding/json"
	"log/slog"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/protocol/sdi12"
	"time"
)

// 合理性区间：读数落在此闭区间之外时该项上报为空值，而非存入可疑读数。
//
// 取值刻意放宽到覆盖 OTT 全部 PLS-C 硬件变体，使任何变体都不会被默认配置否掉：
//   - 水位覆盖 OTT 最宽的压力单元（0...100 m 水柱），并给"相对参考面"留一点负值空间；
//   - 电导率覆盖探头完整量程（5 uS/cm ... 100 mS/cm，以 mS/cm 表示），
//     而不只是其中 0.1...100 mS/cm 那一段；
//   - 盐度/TDS 接受淡水场景的 0，上限 60 是 ppt 档满量程。
//   - 温度沿用原有运行窗口 -8...40 ℃（探头本身额定 -25...70 ℃）。
//
// 这些只是拦截明显坏值的上下界，不能替代逐站标定。
func init() {
	RegisterDevice("PLS-C", &plsC{
		minLevel: -5.0,
		maxLevel: 100.0,
		minTemp:  -8.0,
		maxTemp:  40.0,
		minCond:  0.005,
		maxCond:  100.0,
		minSal:   0.0,
		maxSal:   60.0,
		minTDS:   0.0,
		maxTDS:   60.0,
	})
}

type plsC struct {
	minLevel float64
	maxLevel float64
	minTemp  float64
	maxTemp  float64
	minCond  float64
	maxCond  float64
	minSal   float64
	maxSal   float64
	minTDS   float64
	maxTDS   float64
}

var (
	_ BusDevice   = (*plsC)(nil)
	_ SDI12Device = (*plsC)(nil)
)

var PLSCItems = map[string]int{"water_level": 0, "water_temperature": 1, "water_conductivity": 2, "water_salinity": 3, "water_total_dissolved_solids": 4}

func (d *plsC) NewBusDevice(bus *connWrap.Bus, rawConf json.RawMessage) common.StringMapMap {
	return d.NewSDI12Device(sdi12.NewSession(bus, sdi12.ModeNative), rawConf)
}

// filterPLSValue 返回落在 [min, max] 闭区间内的读数，缺失或越界时返回 nil，
// 让调用方把该项上报为空值。区间取闭区间是有意的：恰好等于满量程的读数
// （例如 100 m 变体的 100.000 m）以及淡水恰好为 0 的盐度/TDS 都必须保留。
// 比较写成"取反"形式，这样 NaN 也会被判为越界丢弃。
func filterPLSValue(itemName string, val *float64, min float64, max float64) *float64 {
	if val == nil {
		return nil
	}
	if !(min <= *val && *val <= max) {
		// 用 Debug 级别：区间配置慢性不匹配的站点否则会每个 cron 周期都刷日志。
		// 排查时把 log_level 设为 debug。
		slog.Debug("Discarding implausible PLS-C reading",
			"item_name", itemName, "value", *val, "min", min, "max", max)
		return nil
	}
	return val
}

func (d *plsC) NewSDI12Device(session *sdi12.Session, rawConf json.RawMessage) map[string]map[string]string {
	var conf struct {
		Addr          string            `json:"addr"`
		ExtraWakeTime byte              `json:"extra_wake_time"`
		DeviceName    string            `json:"device_name"`
		Cron          string            `json:"cron"`
		Items         map[string]string `json:"items"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))

	var (
		tmpData = make(map[string]*float64)
		output  = conf.Addr + "00505\r\n"
	)
	var job = func() map[string]*float64 {
		if err := session.ConcurrentMeasurement(conf.Addr, conf.ExtraWakeTime, output, 5*time.Second); err != nil {
			slog.Error("Failed to perform SDI-12 concurrent measurement", "device", "PLS-C", "addr", conf.Addr, "error", err)
			return nil
		}
		values, err := session.GetData(conf.Addr, conf.ExtraWakeTime, 5)
		if err != nil {
			slog.Error("Failed to get SDI-12 data", "device", "PLS-C", "addr", conf.Addr, "error", err)
			return nil
		}

		tmpData["water_level"] = filterPLSValue("water_level", values[0], d.minLevel, d.maxLevel)
		tmpData["water_temperature"] = filterPLSValue("water_temperature", values[1], d.minTemp, d.maxTemp)
		tmpData["water_conductivity"] = filterPLSValue("water_conductivity", values[2], d.minCond, d.maxCond)
		tmpData["water_salinity"] = filterPLSValue("water_salinity", values[3], d.minSal, d.maxSal)
		tmpData["water_total_dissolved_solids"] = filterPLSValue("water_total_dissolved_solids", values[4], d.minTDS, d.maxTDS)
		return tmpData
	}
	AddCronJob(conf.Cron, conf.Items, PLSCItems, job)
	return common.StringMapMap{conf.DeviceName: conf.Items}
}

//1-0.001+19.51+0.00\r\n
//1+0.01+0.000\r\n
