package global

import (
	"encoding/json"
	"log"
	"log/slog"
	"net/smtp"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var Config struct {
	Debug  bool
	Listen string `json:"listen"`
	Tide   struct {
		Listen string `json:"listen"`
		Camera struct {
			Storage             string `json:"storage"`
			LatestSnapshotCount int    `json:"latest_snapshot_count"`
		} `json:"camera"`
		DataDelaySec time.Duration `json:"data_delay_sec"`
	} `json:"tide"`
	Smtp     smtpConfigStruct `json:"smtp"`
	Keycloak struct {
		BasePath       string `json:"base_path"`
		MasterUsername string `json:"master_username"`
		MasterPassword string `json:"master_password"`
		Realm          string `json:"realm"`
		ClientId       string `json:"client_id"`
		ClientSecret   string `json:"client_secret"`
	} `json:"keycloak"`
	Db struct {
		Tide pgConfigStruct `json:"tide"`
		Sea  pgConfigStruct `json:"sea"`
	} `json:"db"`
}

type pgConfigStruct struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

type smtpConfigStruct struct {
	Host     string `json:"host"`
	Addr     string `json:"addr"`
	Username string `json:"username"`
	Password string `json:"password"`
}

var Smtp struct {
	Auth smtp.Auth
}

func ReadConfig(name string) {
	b, err := os.ReadFile(name)
	if err != nil {
		log.Fatal(err)
	}
	if err = json.Unmarshal(b, &Config); err != nil {
		log.Fatal(err)
	}
	if Config.Smtp.Username != "" && Config.Smtp.Password != "" && Config.Smtp.Host != "" && Config.Smtp.Addr != "" {
		Smtp.Auth = smtp.PlainAuth("", Config.Smtp.Username, Config.Smtp.Password, Config.Smtp.Host)
	}

	var level slog.Level
	if Config.Debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)
	projectDir := filepath.Dir(currentDir)
	replace := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05"))
		}
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			rel, err := filepath.Rel(projectDir, source.File)
			if err == nil {
				source.File = rel
			} else {
				source.File = filepath.Base(source.File)
			}
		}
		return a
	}
	// Create JSON format log handler, suitable for systemd integration
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level, AddSource: true, ReplaceAttr: replace}))
	slog.SetDefault(logger)
}
