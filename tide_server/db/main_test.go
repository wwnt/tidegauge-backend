package db

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"log"
	"os"
	"testing"
	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/global"
	"tide/tide_server/test"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	global.ReadConfig("../config.test.json")
	test.InitDB()
	Init()

	exitCode := m.Run()

	CloseDB()
	os.Exit(exitCode)
}

var (
	station1               Station
	device1                Device
	item1                  Item
	deviceRecord1          DeviceRecord
	upstream1              Upstream
	upstream1Station1      Station
	upstream1Device1       Device
	upstream1Item1         Item
	upstream1DeviceRecord1 DeviceRecord
	upstream2              Upstream
	data                   []common.DataTimeStruct
	station1StatusLogs     []common.RowIdItemStatusStruct
)

func initData(t *testing.T) {
	_, err := TideDB.Exec(`
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

	station1 = Station{
		Id:              uuid.Nil,
		Identifier:      "station1",
		Name:            "站1",
		IpAddr:          "1.1.1.1:1234",
		Location:        json.RawMessage(`{}`),
		Partner:         json.RawMessage(`{}`),
		Cameras:         json.RawMessage(`["camera1"]`),
		Status:          common.Disconnected,
		StatusChangedAt: 0,
		Upstream:        false,
	}

	err = EditStation(&station1)
	require.NoError(t, err)

	device1 = Device{
		StationId:       station1.Id,
		Name:            "device1",
		Specs:           nil,
		LastMaintenance: 0,
	}
	err = EditDevice(device1)
	require.NoError(t, err)
	device1.Specs = test.JsonNull

	item1 = Item{
		StationId:       station1.Id,
		Name:            "item1",
		Type:            "type1",
		DeviceName:      device1.Name,
		Status:          "",
		StatusChangedAt: 0,
		Available:       true,
	}
	err = EditItem(item1)
	require.NoError(t, err)

	n, err := SyncStationCannotEdit(station1.Id, station1.Cameras)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	err = EditStationNotSync(station1.Id, station1.IpAddr)
	require.NoError(t, err)

	deviceRecord1 = DeviceRecord{
		Id:         uuid.Nil,
		StationId:  station1.Id,
		DeviceName: device1.Name,
		Record:     "123",
		CreatedAt:  0,
		UpdatedAt:  0,
		Version:    0,
	}
	err = EditDeviceRecord(&deviceRecord1)
	require.NoError(t, err)

	upstream1 = Upstream{
		Id:       0,
		Username: "tgm-admin",
		Password: "123456",
		Url:      "http://localhost:7100",
	}
	err = EditUpstream(&upstream1)
	require.NoError(t, err)

	upstream1Station1 = Station{
		Id:              uuid.New(),
		Identifier:      "station1",
		Name:            "站1",
		IpAddr:          "",
		Location:        json.RawMessage(`{}`),
		Partner:         test.JsonNull,
		Cameras:         json.RawMessage(`["camera1"]`),
		Status:          common.Disconnected,
		StatusChangedAt: 0,
		Upstream:        true,
	}
	n, err = SyncStation(upstream1.Id, upstream1Station1)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	n, err = SyncStationCannotEdit(upstream1Station1.Id, upstream1Station1.Cameras)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	upstream1Device1 = device1
	upstream1Device1.StationId = upstream1Station1.Id
	n, err = SyncDevice(upstream1Device1)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	upstream1Item1 = item1
	upstream1Item1.StationId = upstream1Station1.Id
	upstream1Item1.Available = false
	n, err = SyncItem(upstream1Item1)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	upstream1DeviceRecord1 = DeviceRecord{
		Id:         uuid.New(),
		StationId:  upstream1Station1.Id,
		DeviceName: upstream1Device1.Name,
		Record:     "upstream device record",
		CreatedAt:  0,
		UpdatedAt:  0,
		Version:    1,
	}
	n, err = SyncDeviceRecord(upstream1DeviceRecord1)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	upstream2 = upstream1
	upstream2.Id = 0

	err = EditUpstream(&upstream2)
	require.NoError(t, err)

	n, err = SyncStation(upstream2.Id, upstream1Station1)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)

	data = []common.DataTimeStruct{
		{Millisecond: 1, Value: 0.1},
		{Millisecond: 2, Value: 0.1},
	}

	for _, d := range data {
		n, err = SaveDataHistory(station1.Id, item1.Name, d.Value, d.Millisecond.ToTime())
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
	}
	for _, d := range data {
		n, err = SaveDataHistory(upstream1Station1.Id, upstream1Item1.Name, d.Value, d.Millisecond.ToTime())
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
	}

	station1StatusLogs = []common.RowIdItemStatusStruct{
		{RowId: 1, ItemStatusStruct: common.ItemStatusStruct{
			ItemName:           item1.Name,
			StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: custype.TimeMillisecond(1000)},
		}},
		{RowId: 2, ItemStatusStruct: common.ItemStatusStruct{
			ItemName:           item1.Name,
			StatusChangeStruct: common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: custype.TimeMillisecond(1100)},
		}},
	}

	for _, statusLog := range station1StatusLogs {
		n, err = SaveItemStatusLog(station1.Id, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
	}
}
