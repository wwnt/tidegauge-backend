package device

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"tide/common"
	"tide/pkg"
	"tide/tide_client/connWrap"
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
		err  error
		line string
	)
	var job = func() *float64 {
		line, err = conn.ReadLine([]byte("\r\x05PW " + conf.Addr + " 0\r"))
		if err != nil {
			log.Printf("err: %v\n received: %s\n received hex: (% X)\n", err, pkg.Printable([]byte(line)), line)
			return nil
		}
		if len(line) != 25 {
			log.Printf("received: %s\n received hex: (% X)\n", pkg.Printable([]byte(line)), line)
			return nil
		}
		status := line[8]
		if status != '0' {
			log.Printf("status: %c\n", status)
			recv, err := conn.CustomCommand([]byte("\r\x05PW " + conf.Addr + " 3\r"))
			if err != nil {
				log.Printf("err: %v\n received: %s\n", err, pkg.Printable(recv))
			} else {
				log.Printf("received: %s\n", pkg.Printable(recv))
			}
			return nil
		}
		val1 := strings.TrimSpace(line[9:16])
		if len(val1) == 0 {
			log.Printf("received: %s\n received hex: (% X)\n", pkg.Printable([]byte(line)), line)
			return nil
		}
		if val1[0] == '/' {
			log.Printf("received: %s\n received hex: (% X)\n", pkg.Printable([]byte(line)), line)
			return nil
		}
		f, err := strconv.ParseFloat(val1, 64)
		if err != nil {
			log.Printf("err: %v\n received: %s\n received hex: (% X)\n", err, pkg.Printable([]byte(line)), line)
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
// output: PW  104   3432  3386    ("\u0001PW "+addr+"\u0002%d %f %s\r\n")
// output: PW  100  22022 // /////
// output: PW  100 22903 27910 /// // // // ////// ////// ////
