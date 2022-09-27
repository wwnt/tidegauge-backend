package main

import (
	"bufio"
	"flag"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"os"
	"path"
	"tide/pkg/project"
	"tide/tide_server/controller"
	"tide/tide_server/global"
	"tide/tide_server/test"
	"time"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Llongfile)
}

func main() {
	initKeycloak := flag.Bool("initKeycloak", false, "initialize keycloak")
	wkDir := flag.String("dir", ".", "working dir")
	flag.BoolVar(&global.Config.Debug, "debug", true, "debug mode")
	cfgName := flag.String("config", "config.json", "Config file")
	flag.Parse()

	if err := os.Chdir(*wkDir); err != nil {
		log.Fatal(err)
	}

	global.ReadConfig(*cfgName)

	if *initKeycloak {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("Enter password for Superadmin (The first user on install will be the superadmin. A superadmin can adjust other users' privileges, including making them admin.): ")
		var adminPassword string
		if scanner.Scan() {
			adminPassword = scanner.Text()
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
		test.InitKeycloak(test.AdminUsername, adminPassword, false)
		return
	}

	initZapLogger()
	project.RegisterReleaseFunc(func() { _ = zap.L().Sync() })

	controller.Init()

	project.Run(!global.Config.Debug, startRunningStatus, stopRunningStatus, shutdownRunningStatus, abortedRunningStatus)
	project.CallReleaseFunc()
}

func initZapLogger() {
	var (
		err    error
		logger *zap.Logger
	)
	if global.Config.Debug {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("01/02 15:04:05")
		logger, err = config.Build()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		if err = os.MkdirAll("log", os.ModePerm); err != nil {
			log.Fatal(err)
		}
		var ts = time.Now().Format("2006-01-02__15-04-05")
		lowWriteSyncer, err := os.Create(path.Join("log", ts+".info.log"))
		if err != nil {
			log.Fatal(err)
		}
		highWriteSyncer, err := os.Create(path.Join("log", ts+".error.log"))
		if err != nil {
			log.Fatal(err)
		}
		encoderCfg := zap.NewProductionEncoderConfig()
		encoder := zapcore.NewJSONEncoder(encoderCfg)
		lowCore := zapcore.NewCore(encoder, lowWriteSyncer, zap.LevelEnablerFunc(func(lev zapcore.Level) bool { return lev < zap.ErrorLevel }))
		highCore := zapcore.NewCore(encoder, highWriteSyncer, zap.LevelEnablerFunc(func(lev zapcore.Level) bool { return lev >= zap.ErrorLevel }))
		logger = zap.New(zapcore.NewTee(highCore, lowCore), zap.AddCaller())
	}

	zap.ReplaceGlobals(logger)
	if _, err = zap.RedirectStdLogAt(logger, zap.DebugLevel); err != nil {
		logger.Fatal(err.Error())
	}
}

func startRunningStatus() {
	zap.L().Info("update running status", zap.String("status", "started"))
}
func stopRunningStatus() {
	zap.L().Info("update running status", zap.String("status", "stopped"))
}
func shutdownRunningStatus() {
	zap.L().Info("update running status", zap.String("status", "shutdown"))
}
func abortedRunningStatus(t time.Time) {
	zap.L().Info("update running status", zap.String("status", "aborted"), zap.Time("at", t))
}
