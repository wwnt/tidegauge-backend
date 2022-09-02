package global

import (
	"encoding/json"
	"github.com/robfig/cron/v3"
	"log"
	"os"
	"tide/pkg/simplelog"
	"time"
)

var Config struct {
	LogLevel   string              `json:"log_level"`
	Listen     string              `json:"listen"`
	Server     string              `json:"server"`
	Identifier string              `json:"identifier"`
	Devices    map[string][]string `json:"devices"`
	Db         struct {
		Dsn string `json:"dsn"`
	} `json:"db"`
	Cameras struct {
		Ftp struct {
			Path     string        `json:"path"`
			HoldDays time.Duration `json:"hold_days"`
		} `json:"ftp"`
		List map[string]struct {
			Snapshot string `json:"snapshot"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"list"`
	} `json:"cameras"`
}

var CameraHoldTime time.Time
var Log *simplelog.Logger
var CronJob *cron.Cron

func Init(name string) {
	b, err := os.ReadFile(name)
	if err != nil {
		log.Fatal(err)
	}
	if err = json.Unmarshal(b, &Config); err != nil {
		log.Fatal(err)
	}
	if Config.Cameras.Ftp.HoldDays <= 0 {
		CameraHoldTime = time.UnixMilli(0)
	} else {
		CameraHoldTime = time.Now().Add(-24 * time.Hour * Config.Cameras.Ftp.HoldDays)
	}
	Log = simplelog.NewLogger(simplelog.GetLevel(Config.LogLevel), log.Default())
	CronJob = cron.New(cron.WithParser(cron.NewParser(cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor)), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	CronJob.Start()
}
