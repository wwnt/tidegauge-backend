package device

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"tide/common"
	"tide/pkg"

	"periph.io/x/conn/v3/i2c"
)

func init() {
	RegisterDevice("pcf8583", &pcf8583{})
}

type pcf8583 struct{}

func (pcf8583) NewDevice(conn interface{}, rawConf json.RawMessage) common.StringMapMap {
	bus := conn.(i2c.Bus)
	var conf struct {
		Addr       uint16 `json:"addr"` //0xA0 convert to decimal .. 160
		DeviceName string `json:"device_name"`
		Model      string `json:"model"`
		Cron       string `json:"cron"`
		ItemName   string `json:"item_name"`
	}

	pkg.Must(json.Unmarshal(rawConf, &conf))
	d := i2c.Dev{Bus: bus, Addr: conf.Addr >> 1} //Saves device, prevents having to specify address everytime.
	setMode(d, mode_event_counter)
	var job = func() *float64 {
		var value float64 = 1
		return &value
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return common.StringMapMap{conf.DeviceName: map[string]string{"pcf8583_counter": conf.ItemName}}
}

func setMode(d i2c.Dev, _mode byte) {
	var mode byte = _mode
	control := (location_control & ^mode_test) | (mode & mode_test)
	setRegister(d, location_control, control)
}

func getMode(d i2c.Dev) []byte {
	fmt.Printf("Running getMode \n")
	var register_value []byte = getRegister(d, location_control)
	for _, n := range register_value {
		fmt.Printf("% 08b", n) // prints 00000000 11111101
	}
	fmt.Printf("\n")
	register_value[0] = register_value[0] & mode_test

	for _, n := range register_value {
		fmt.Printf("% 08b", n) // prints 00000000 11111101
	}
	fmt.Printf("\n")

	return register_value
}

func getCount(d i2c.Dev) uint32 {
	var readBuffer []byte = []byte{}
	d.Write([]byte{location_control})
	readBuffer = Read(d, 0, 3)
	var count uint32
	count = bcdToBYTE(binary.BigEndian.Uint32([]byte{readBuffer[0]}))
	count += bcdToBYTE(binary.BigEndian.Uint32([]byte{readBuffer[1]})) * 100
	count += bcdToBYTE(binary.BigEndian.Uint32([]byte{readBuffer[2]})) * 10000

	return count
}

func setCount(d i2c.Dev, count uint32) {
	stop(d)
	var writeBuffer []byte = []byte{ //Please revisit
		byteToBCD(count % 100)[0],
		byteToBCD(((count / 100) % 100))[0],
		byteToBCD(((count / 10000) % 100))[0]}
	d.Write([]byte{location_control})
	d.Write(writeBuffer)
	start(d)
}

func stop(d i2c.Dev) {
	var control uint32 = bytesToUint32(getRegister(d, location_control))
	control |= 0x80
	setRegisters(d, location_control, uint32ToBytes(control))
}
func start(d i2c.Dev) {
	var control uint32 = bytesToUint32(getRegister(d, location_control))
	control &= 0x7F
	setRegisters(d, location_control, uint32ToBytes(control))
}

func setRegister(d i2c.Dev, offset byte, value byte) {
	d.Write([]byte{offset})
	d.Write([]byte{value})
}

func setRegisters(d i2c.Dev, offset byte, value []byte) {
	d.Write([]byte{offset})
	d.Write(value)
}

func getRegister(d i2c.Dev, offset byte) []byte {
	read := make([]byte, 5)
	d.Tx([]byte{offset}, read)
	return read
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

func bcdToBYTE(b uint32) uint32 {
	return ((b >> 4) * 10) + (b & 0x0f)
}
func byteToBCD(b uint32) []byte {
	return uint32ToBytes(((b / 10) << 4) + (b % 10))
}

func uint32ToBytes(vs uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, vs)
	return buf
}

func bytesToUint32(buf []byte) uint32 {
	return uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
}

//return uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2]) <<8 |
//        uint32(buf[3])
//for BE or
//
// return uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2]) <<16 |
//        uint32(buf[3]) <<24
//for LE
