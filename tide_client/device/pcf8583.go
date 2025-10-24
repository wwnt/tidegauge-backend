package device

import (
	"encoding/json"
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
		Addr       uint16 `json:"addr"` //0xA0 convert to decimal .. 160
		DeviceName string `json:"device_name"`
		Cron       string `json:"cron"`
		ItemName   string `json:"item_name"`
		ResetC     bool   `json:"reset_c"`
	}

	pkg.Must(json.Unmarshal(rawConf, &conf))
	d := i2c.Dev{Bus: bus, Addr: conf.Addr >> 1} //Saves device, prevents having to specify address everytime. ">> 1" convert to 7 bit.
	_ = setRegister(d, locationControl, modeEventCounter)
	_ = setCount(d, 0) // Reset counter at startup
	var job = func() *float64 {
		cnt, err := getCount(d)
		if err != nil {
			return nil
		}
		var value = float64(cnt*2) / 10
		if conf.ResetC {
			_ = setCount(d, 0)
		}
		return &value
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return common.StringMapMap{conf.DeviceName: map[string]string{"pcf8583_counter": conf.ItemName}}
}

const (
	modeEventCounter byte = 0x20
	modeTest         byte = 0x30
	CtrlStopCounting byte = 0x80
	locationControl  byte = 0x00
	locationCounter  byte = 0x01
)

func getCount(d i2c.Dev) (int32, error) {
	data := make([]byte, 3)
	err := d.Tx([]byte{locationCounter}, data)
	if err != nil {
		return 0, err
	}
	return int32(bcdToBYTE(data[0])) +
		int32(bcdToBYTE(data[1]))*100 +
		int32(bcdToBYTE(data[2]))*10000, nil
}

func setCount(d i2c.Dev, count int32) error {
	ctrlRegVal, err := getRegister(d, locationControl)
	if err != nil {
		return err
	}
	if err = setRegister(d, locationControl, ctrlRegVal|CtrlStopCounting); err != nil {
		return err
	}
	_, err = d.Write([]byte{
		locationCounter,
		byteToBCD(uint8(count % 100)),
		byteToBCD(uint8((count / 100) % 100)),
		byteToBCD(uint8((count / 10000) % 100))})
	if err != nil {
		return err
	}
	return setRegister(d, locationControl, ctrlRegVal&0x7F)
}

func setRegister(d i2c.Dev, offset byte, value byte) error {
	_, err := d.Write([]byte{offset, value})
	return err
}

func getRegister(d i2c.Dev, offset byte) (uint8, error) {
	read := make([]byte, 1)
	err := d.Tx([]byte{offset}, read)
	return read[0], err
}

func bcdToBYTE(b byte) byte {
	return ((b >> 4) * 10) + (b & 0x0f)
}
func byteToBCD(b byte) byte {
	return ((b / 10) << 4) | (b % 10)
}
