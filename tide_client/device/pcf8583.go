package device

import (
	"encoding/json"
	"log"
	"tide/common"
	"tide/pkg"

	"periph.io/x/conn/v3/i2c"
)

func init() {
	RegisterDevice("pcf8583", &pcf8583{})
}

/**
 * This code enables the access to PCF8583 Event Mode Counter.<br>
 * It handles PCF8583 low level reads.<br><br>
 *
 * This code is an adaptation of [PCF8583 Event Counter] C Library to Golang.
 * Original library developed by Xose Pérez, [PCF8583 Real Time Clock and Event Counter] C Library for Arduino.<br>
 * https://github.com/xoseperez/pcf8583
 */

type pcf8583 struct{}

func (pcf8583) NewDevice(conn interface{}, rawConf json.RawMessage) common.StringMapMap {
	bus := conn.(i2c.Bus)
	var conf struct {
		Addr       uint16  `json:"addr"` //0xA0 convert to decimal .. 160
		DeviceName string  `json:"device_name"`
		Model      string  `json:"model"`
		Cron       string  `json:"cron"`
		ItemName   string  `json:"item_name"`
		Resolution float64 `json:"resolution"`
		ResetC     bool    `json:"reset_c"`
	}

	pkg.Must(json.Unmarshal(rawConf, &conf))
	d := i2c.Dev{Bus: bus, Addr: conf.Addr >> 1} //Saves device, prevents having to specify address everytime. ">> 1" convert to 7 bit.
	setMode(d, mode_event_counter)
	log.Printf("PCF8583 Mode 0x%X", uint8(getMode(d)))
	var job = func() *float64 {
		var value float64 = float64(getCount(d)) * conf.Resolution
		if conf.ResetC {
			setCount(d, 0)
		}
		return &value
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return common.StringMapMap{conf.DeviceName: map[string]string{"pcf8583_counter": conf.ItemName}}
}

func setMode(d i2c.Dev, _mode byte) {
	var mode byte = _mode
	var control uint8 = getRegister(d, location_control)
	control = (control & ^mode_test) | (mode & mode_test)
	setRegister(d, location_control, control)
}

func getMode(d i2c.Dev) uint8 {
	var register_value uint8 = getRegister(d, location_control)
	register_value = register_value & mode_test
	return register_value
}

func getCount(d i2c.Dev) int32 {
	var readBuffer []byte = []byte{}
	d.Write([]byte{location_control})
	readBuffer = Read(d, 0, 4)
	//log.Println("Counter ")
	//for _, n := range readBuffer {
	//	log.Printf("% 08b", n)
	//}
	//log.Printf("\n")
	return int32(bcdToBYTE(readBuffer[1])) +
		int32(bcdToBYTE(readBuffer[2]))*100 +
		int32(bcdToBYTE(readBuffer[3]))*10000
}

func setCount(d i2c.Dev, count int32) {
	var writeBuffer []byte = []byte{
		location_control,
		stop(d),
		byteToBCD(uint8(count % 100)),
		byteToBCD(uint8((count / 100) % 100)),
		byteToBCD(uint8((count / 10000) % 100))}
	d.Write(writeBuffer)
	start(d)
}

func stop(d i2c.Dev) uint8 {
	var control uint8 = getRegister(d, location_control)
	control |= 0x80
	return control
	//setRegister(d, location_control, control)
}
func start(d i2c.Dev) {
	var control uint8 = getRegister(d, location_control)
	control &= 0x7F
	setRegister(d, location_control, control)
}

func setRegister(d i2c.Dev, offset byte, value byte) {
	d.Write([]byte{offset, value})
}

func getRegister(d i2c.Dev, offset byte) uint8 {
	read := make([]byte, 1)
	d.Tx([]byte{offset}, read)
	return read[0]
}

func Read(d i2c.Dev, offset byte, size int) []byte {
	read := make([]byte, size)
	d.Tx([]byte{offset}, read)
	return read
}

func reset(d i2c.Dev) {
	d.Write([]byte{location_control})

	d.Write([]byte{0x04}) // 00 control/status (alarm enabled by default)
	d.Write([]byte{0x00}) // 01 set hundreds-of-seconds
	d.Write([]byte{0x00}) // 02 set second
	d.Write([]byte{0x00}) // 03 set minute
	d.Write([]byte{0x00}) // 04 set hour (24h format)
	d.Write([]byte{0x01}) // 05 set day
	d.Write([]byte{0x01}) // 06 set month
	d.Write([]byte{0x00}) // 07 set timer
	d.Write([]byte{0x00}) // 08 set alarm control
	d.Write([]byte{0x00}) // 09 set alarm hundreds-of-seconds
	d.Write([]byte{0x00}) // 0A set alarm second
	d.Write([]byte{0x00}) // 0B set alarm minute
	d.Write([]byte{0x00}) // 0C set alarm hour
	d.Write([]byte{0x01}) // 0D set alarm day
	d.Write([]byte{0x01}) // 0E set alarm month
	d.Write([]byte{0x00}) // 0F set alarm timer
	d.Write([]byte{0x00}) // 10 set year offset to 0
	d.Write([]byte{0x00}) // 11 set last read value for year to 0
}

const (
	mode_event_counter byte = 0x20
	mode_test          byte = 0x30
	location_counter   byte = 0x01
	location_control   byte = 0x00
)

func bcdToBYTE(b byte) byte {
	return ((b >> 4) * 10) + (b & 0x0f)
}
func byteToBCD(b byte) byte {
	return ((b / 10) << 4) | (b % 10)
}
