package device

import (
	"encoding/json"
	"strconv"
	"strings"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
)

func init() {
	RegisterDevice("PWD50", &pwd50{})
}

type pwd50 struct{}

func (pwd50) NewDevice(c any, rawConf json.RawMessage) common.StringMapMap {
	conn := c.(*connWrap.ConnUtil)
	var conf struct {
		ItemName   string `json:"item_name"`
		DeviceName string `json:"device_name"`
		Addr       string `json:"addr"`
		Cron       string `json:"cron"`
	}
	pkg.Must(json.Unmarshal(rawConf, &conf))
	DevicesUartConn[conf.DeviceName] = conn

	var (
		err   error
		input = []byte("\r\x05PW " + conf.Addr + " 0\r")
		line  string
	)
	var job = func() *float64 {
		line, err = conn.ReadLine(input)
		if err != nil {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrIO, Received: []byte(line), Err: err})
			return nil
		}
		if len(line) != 25 {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrParse, Received: []byte(line), Err: err})
			return nil
		}
		val1 := strings.TrimSpace(line[9:16])
		if val1[0] == '/' {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrItem, Received: []byte(line), Err: err})
			return nil
		}
		f, err := strconv.ParseFloat(val1, 64)
		if err != nil {
			global.Log.Error(&connWrap.Error{Type: connWrap.ErrParse, Received: []byte(line), Err: err})
			return nil
		}
		return &f
	}
	AddCronJobWithOneItem(conf.Cron, conf.ItemName, job)
	return common.StringMapMap{conf.DeviceName: {"air_visibility": conf.ItemName}}
}

// PWD50 PWD50 id 2 CHAR
// page:48 input:  CRPW id message_numberCR
// page 44
// output: PW  104   3432  3386    ("\u0001PW " + addr + "\u0002%d %f %s\r\n")
// output: PW  100  22022 // /////
// output: PW  100 22903 27910 /// // // // ////// ////// ////
