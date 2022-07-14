package project

import (
	"os"
	"os/signal"
	"syscall"
)

func runService(start, stop, shutdown func()) {
	start()
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1)

	switch <-c {
	case syscall.SIGUSR1:
		shutdown()
	default:
		stop()
	}
}
