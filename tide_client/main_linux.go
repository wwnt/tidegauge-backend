package main

import (
	"flag"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"tide/pkg/project"
	"tide/tide_client/controller"
	"tide/tide_client/global"
)

func init() {
	flag.StringVar(&global.Config.LogLevel, "log", "debug", "log level")
	cfgName := flag.String("config", "config.json", "Config file")
	flag.Parse()

	global.Init(*cfgName)
}

func main() {
	controller.Init()
	go func() {
		err := http.ListenAndServe(global.Config.Listen, nil)
		slog.Error("HTTP server exited", "error", err)
		os.Exit(1)
	}()
	waitAndCleanUp()
}

func waitAndCleanUp() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	switch <-ch {
	case syscall.SIGTERM:
	case syscall.SIGINT:
	}
	project.CallReleaseFunc()
}
