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
	// 这个监听只提供 pprof 调试端点，与采集无关：没配置就不启动（空地址会去绑 :80，
	// 普通用户必然失败），绑定失败也只记日志，不能因此让采集进程退出重启。
	if global.Config.Listen == "" {
		slog.Info("Debug HTTP listener disabled: listen is not configured")
	} else {
		go func() {
			err := http.ListenAndServe(global.Config.Listen, nil)
			slog.Error("Debug HTTP listener exited", "listen", global.Config.Listen, "error", err)
		}()
	}
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
