package controller

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"tide/common"
	"tide/pkg/custype"
	"tide/pkg/pubsub"
	"tide/tide_client/global"
	"time"
)

func addRpiStatus(dataPub *pubsub.PubSub) {
	if _, err := rpiStat(); err == nil {
		_, err = global.CronJob.AddFunc("@every 20s",
			func() {
				data, err := rpiStat()
				if err != nil {
					global.Log.Error(err)
					return
				}
				err = dataPub.Publish(common.SendMsgStruct{
					Type: common.MsgRpiStatus,
					Body: common.RpiStatusTimeStruct{
						RpiStatusStruct: data,
						Millisecond:     custype.ToTimeMillisecond(time.Now()),
					}}, nil)
				if err != nil {
					global.Log.Error(err)
					return
				}
			},
		)
		if err != nil {
			global.Log.Fatal(err)
		}
	}
}

func rpiCPUTemp() (float64, error) {
	b, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, err
	}
	t, err := strconv.ParseInt(string(bytes.TrimSpace(b)), 10, 0)
	if err != nil {
		return 0, err
	}
	return float64(t) / 1000, err
}

func rpiGPUTemp() (float64, error) {
	b, err := exec.Command("vcgencmd", "measure_temp").Output()
	if err != nil {
		return 0, err
	}
	var temp float64
	//temp=45.7'C\n
	if _, err = fmt.Sscanf(string(b), "temp=%f'C\n", &temp); err != nil {
		return 0, err
	}
	return temp, err
}

func rpiStat() (s common.RpiStatusStruct, err error) {
	s.CpuTemp, err = rpiCPUTemp()
	if err != nil {
		return
	}
	s.GpuTemp, err = rpiGPUTemp()
	return
}
