package controller

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
					slog.Error("Failed to get raspberry pi status", "error", err)
					return
				}
				err = dataPub.Publish(common.SendMsgStruct{
					Type: common.MsgRpiStatus,
					Body: common.RpiStatusTimeStruct{
						RpiStatusStruct: data,
						Millisecond:     custype.ToTimeMillisecond(time.Now()),
					}}, nil)
				if err != nil {
					slog.Error("Failed to publish raspberry pi status message", "error", err)
					return
				}
			},
		)
		if err != nil {
			slog.Error("Failed to add raspberry pi status cron job", "error", err)
			os.Exit(1)
		}
	}
}

func rpiCPUTemp() (float64, error) {
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
	return
}
