package db

import (
	"log"
	"os"
	"testing"
	"tide/tide_client/global"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	global.Init("../config.test.json")
	Init()
	InitDB()

	exitCode := m.Run()
	os.Exit(exitCode)
}
