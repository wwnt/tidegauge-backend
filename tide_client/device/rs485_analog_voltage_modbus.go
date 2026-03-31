package device

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"

	"github.com/wwnt/modbus"
)

func init() {
	RegisterDevice("ANALOG-VOLTAGE-MODBUS", &analogVoltageModbus{})
}

type analogVoltageModbus struct{}

func (analogVoltageModbus) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
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
	client := modbus.NewClient(h)
	job := func() *float64 {
		conn.Lock()
		defer conn.Unlock() // prevent overlapping Modbus requests on the shared bus

		results, readErr := client.ReadInputRegisters(0, 1)
		if readErr != nil {
			slog.Error("Error reading input registers from analog voltage Modbus device", "error", readErr, "addr", conf.Addr)
			return nil
		}
		value, decodeErr := decodeAnalogVoltageModbusValue(results)
		if decodeErr != nil {
			slog.Error("Invalid register payload from analog voltage Modbus device", "error", decodeErr, "addr", conf.Addr, "results", results)
			return nil
		}
		return &value
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return common.StringMapMap{conf.DeviceName: {"rain_intensity": conf.ItemName}}
}

func decodeAnalogVoltageModbusValue(results []byte) (float64, error) {
	if len(results) != 2 {
		return 0, fmt.Errorf("unexpected analog voltage Modbus payload length: %d", len(results))
	}

	raw := binary.BigEndian.Uint16(results)
	decimalPlaces := int(raw / 10000)
	mantissa := int(raw % 10000)

	return float64(mantissa) / math.Pow10(decimalPlaces), nil
}
