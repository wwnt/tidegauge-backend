package controller

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"log"
	"net/http/httptest"
	"os"
	"testing"
	"tide/common"
	"tide/pkg/project"
	"tide/tide_server/auth"
	"tide/tide_server/db"
	"tide/tide_server/global"
	"tide/tide_server/test"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	global.ReadConfig("../config.test.json")
	global.Config.Debug = true

	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	zap.ReplaceGlobals(logger)

	test.InitDB()
	test.InitKeycloak(test.AdminUsername, test.AdminPassword, true)
	Init()
	router = setupRouter()
	testServer = httptest.NewServer(router)

	exitCode := m.Run()

	testServer.Close()
	_ = logger.Sync()
	project.CallReleaseFunc()
	db.CloseDB()
	os.Exit(exitCode)
}

var (
	router     *gin.Engine
	testServer *httptest.Server

	station1 = db.Station{
		Id:              uuid.New(),
		Identifier:      "station1",
		Name:            "站1",
		IpAddr:          "",
		Location:        nil,
		Partner:         nil,
		Cameras:         nil,
		Status:          "",
		StatusChangedAt: 0,
		Upstream:        false,
	}
	station1Info = common.StationInfoStruct{
		Identifier: "station1",
		Devices: common.StringMapMap{
			"地点1温湿度": {
				"air_humidity": "location1_air_humidity",
			},
			"地点1能见度": {
				"air_visibility": "location1_air_visibility",
			},
		},
		Cameras: []string{"camera1"},
	}
	stationsFullInfo = []db.StationFullInfo{
		{
			Station: station1,
			Items: []db.Item{

				{StationId: station1.Id, Name: "location1_air_humidity", Type: "air_humidity", DeviceName: "地点1温湿度", Status: common.Abnormal, StatusChangedAt: 1000, Available: true},
				{StationId: station1.Id, Name: "location1_air_visibility", Type: "air_visibility", DeviceName: "地点1能见度", Status: common.Normal, StatusChangedAt: 2000, Available: true},
			},
			Devices: []db.Device{
				{StationId: station1.Id, Name: "地点1温湿度", Specs: test.JsonNull, LastMaintenance: 2000},
				{StationId: station1.Id, Name: "地点1能见度", Specs: test.JsonNull, LastMaintenance: 2000},
			},
		},
	}

	deviceRecords = []db.DeviceRecord{
		{Id: uuid.New(), StationId: station1.Id, DeviceName: "地点1温湿度", Record: "1", CreatedAt: 1000, UpdatedAt: 1200, Version: 2},
		{Id: uuid.New(), StationId: station1.Id, DeviceName: "地点1能见度", Record: "2", CreatedAt: 1000, UpdatedAt: 1200, Version: 2},
	}

	user01 = auth.User{
		UserBaseInfo: auth.UserBaseInfo{Username: "user01", Password: test.AdminPassword, Email: ""},
	}
	permissions = common.UUIDStringsMap{
		station1.Id: {"location1_air_humidity", "location1_air_visibility"},
	}
)

func truncateDB(t *testing.T) {
	var err error

	_, err = db.TideDB.Exec(`
truncate table devices restart identity cascade;
truncate table device_record restart identity cascade;
truncate table upstreams restart identity cascade;
truncate table upstream_stations restart identity cascade;
truncate table permissions_item_data restart identity cascade;
truncate table items restart identity cascade;
truncate table stations restart identity cascade;
truncate table permissions_camera_status restart identity cascade;
truncate table item_status_log restart identity cascade;
truncate table rpi_status_log restart identity cascade;
drop table if exists item1 cascade;
`)
	require.NoError(t, err)
}

func _(ctx context.Context) {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal(err)
	}

	resp, err := dockerCli.ContainerCreate(ctx,
		&container.Config{
			Image: "quay.io/keycloak/keycloak",
		},
		&container.HostConfig{},
		&network.NetworkingConfig{},
		nil,
		"",
	)
	if err != nil {
		log.Fatal(err)
	}
	if err = dockerCli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Fatal(err)
	}
}
