package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
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

	controller.Init()

	project.Run(startRunningStatus, stopRunningStatus, shutdownRunningStatus, abortedRunningStatus)
	project.CallReleaseFunc()
}

func startRunningStatus() {
	slog.Info("update running status", "status", "started")
}
func stopRunningStatus() {
	slog.Info("update running status", "status", "stopped")
}
func shutdownRunningStatus() {
	slog.Info("update running status", "status", "shutdown")
}
func abortedRunningStatus(t time.Time) {
	slog.Info("update running status", "status", "aborted", "at", t)
}
