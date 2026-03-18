package controller

import (
	"log"
	"os"
	"testing"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/global"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	global.Init("../config.test.json")
	db.Init()
	db.InitDB()

	dataBroker = pubsub.NewBroker()
	go receiveData(dataBroker)

	rc := m.Run()

	os.Exit(rc)
}

var dataBroker *pubsub.Broker
var station1Info = common.StationInfoStruct{
	Identifier: "station1",
	Devices: common.StringMapMap{
		"地点1温湿度": {
			"air_humidity": "item1",
		},
		"地点1能见度": {
			"air_visibility": "item2",
		},
	},
	Cameras: []string{"camera1"},
}
