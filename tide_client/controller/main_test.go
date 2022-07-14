package controller

import (
	"github.com/stretchr/testify/assert"
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

	dataPubSub = pubsub.NewPubSub()

	addDevices()
	go receiveData(dataPubSub)

	rc := m.Run()

	os.Exit(rc)
}

func TestStationInfoDevice(t *testing.T) {
	var devices = common.StringMapMap{
		"rs485_1": {
			"air_humidity":    "item1",
			"air_temperature": "item2",
		},
		"rs485_2": {
			"air_humidity":    "item3",
			"air_temperature": "item4",
		},
		"gpio1": {
			"precipitation_detection": "item5",
		},
	}
	assert.Equal(t, devices, stationInfo.Devices)
}

var dataPubSub *pubsub.PubSub
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
