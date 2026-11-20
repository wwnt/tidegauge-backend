package global

import (
	"encoding/json"
	"log"
	"log/slog"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

var Config struct {
	Debug  bool
	Listen string `json:"listen"`
	SyncV2 struct {
		Enabled bool `json:"enabled"`
	} `json:"sync_v2"`
	Tide struct {
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
	Jwt struct {
		Expire time.Duration `json:"expire"`
	} `json:"jwt"`
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

// DSN 生成 libpq 关键字/值形式的连接串，库名用 dbName（传空串则用配置里的 DBName）。
//
// 每个值都加引号并转义：直接把值拼进去的话，空密码会让解析器把后面的 "dbname=x" 当成
// 密码的值，含空格的密码会被截断并吞掉后面的参数，报出来的错误还指不到配置上。
func (c pgConfigStruct) DSN(dbName string) string {
	if dbName == "" {
		dbName = c.DBName
	}
	quote := func(v string) string {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
	}
	return "host=" + quote(c.Host) +
		" port=" + quote(c.Port) +
		" user=" + quote(c.User) +
		" password=" + quote(c.Password) +
		" dbname=" + quote(dbName) +
		" sslmode=disable"
}

type smtpConfigStruct struct {
	Host string `json:"host"`
	// Addr 是 "host:port" 形式的服务器地址。留空时由 Host 和 Port 组合而成
	// （config.template.json 写的是 host + port）。
	Addr     string `json:"addr"`
	Port     int    `json:"port"`
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
	if Config.Smtp.Addr == "" && Config.Smtp.Host != "" && Config.Smtp.Port > 0 {
		Config.Smtp.Addr = net.JoinHostPort(Config.Smtp.Host, strconv.Itoa(Config.Smtp.Port))
	}
	mailEnabled := Config.Smtp.Username != "" && Config.Smtp.Password != "" && Config.Smtp.Host != "" && Config.Smtp.Addr != ""
	if mailEnabled {
		Smtp.Auth = smtp.PlainAuth("", Config.Smtp.Username, Config.Smtp.Password, Config.Smtp.Host)
	}

	var level slog.Level
	if Config.Debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}

	// Use tinted text logs for readability.
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      level,
		TimeFormat: time.DateTime,
		AddSource:  true,
	})
	slog.SetDefault(slog.New(handler))

	if !mailEnabled {
		// 账号申请/审批通知靠邮件；没配全时静默不发，至少在启动日志里说明。
		slog.Warn("SMTP is not fully configured (need host, port or addr, username, password); notification mails are disabled")
	}
}
