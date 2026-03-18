package global

import (
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/robfig/cron/v3"
)

type Ftp struct {
	Path     string        `json:"path"`
	HoldDays time.Duration `json:"hold_days"`
}

var Config struct {
	LogLevel string `json:"log_level"`
	Listen   string `json:"listen"`
	Server   string `json:"server"`
	SyncV2   struct {
		Enabled bool   `json:"enabled"`
		Addr    string `json:"addr"`
	} `json:"sync_v2"`
	Identifier string              `json:"identifier"`
	Devices    map[string][]string `json:"devices"`
	Db         struct {
		Dsn      string        `json:"dsn"`
		HoldDays time.Duration `json:"hold_days"`
	} `json:"db"`
	Gnss struct {
		Ftp Ftp `json:"ftp"`
	} `json:"gnss"`
	Cameras struct {
		Ftp  Ftp `json:"ftp"`
		List map[string]struct {
			Snapshot string `json:"snapshot"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"list"`
	} `json:"cameras"`
}

var CronJob *cron.Cron

func Init(name string) {
	b, err := os.ReadFile(name)
	if err != nil {
		log.Fatal(err)
	}
	if err = json.Unmarshal(b, &Config); err != nil {
		log.Fatal(err)
	}

	// Initialize logger
	initLogger()

	CronJob = cron.New(
		cron.WithParser(cron.NewParser(cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor)),
		cron.WithChain(cron.Recover(cron.DefaultLogger)))
	CronJob.Start()
}

func initLogger() {
	// Set log handler based on configuration level
	var level slog.Level
	switch Config.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Use tinted text logs for readability.
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      level,
		TimeFormat: time.DateTime,
		AddSource:  true,
	})
	slog.SetDefault(slog.New(handler))
}
