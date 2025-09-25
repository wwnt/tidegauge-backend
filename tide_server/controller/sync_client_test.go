package controller

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"testing"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_server/db"
)

func TestSyncClient(t *testing.T) {
	truncateDB(t)

	upstreamConfig := db.Upstream{
		Username: "user01",
		Password: "123",
	}
	err := db.EditUpstream(&upstreamConfig)
	require.NoError(t, err)

	conn1, conn2 := net.Pipe() // conn1: sync server , conn2: sync client
	conn3, conn4 := net.Pipe() // conn3: sync server , conn4: sync client
	defer func() { _ = conn3.Close() }()

	subscriber := pubsub.NewSubscriber(nil, conn3)
	configPubSub.SubscribeTopic(subscriber, nil)
	defer configPubSub.Evict(subscriber)
	statusPubSub.SubscribeTopic(subscriber, nil)
	defer statusPubSub.Evict(subscriber)
	dataPubSub.SubscribeTopic(subscriber, nil)
	defer dataPubSub.Evict(subscriber)
	missDataPubSub.SubscribeTopic(subscriber, nil)
	defer missDataPubSub.Evict(subscriber)

	go func() {
		defer func() { _ = conn1.Close() }()
		mockSyncServer(t, conn1)
	}()
	go func() {
		defer func() { _ = conn2.Close() }()
		handleSyncClientConn(conn2, &upstreamStorage{
			config: upstreamConfig,
		})
	}()

	//{"type":"SyncStation","body":{"id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","identifier":"station1","name":"站1","upstream":fa lse}}
	//{"type":"update_station_status","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","identifier":"station1","status":"","changed_at":0}}
	//{"type":"SyncDevice","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","name":"地点1温湿度","specs":null,"last_mainte     nance":2000}}
	//{"type":"SyncDevice","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","name":"地点1能见度","specs":null,"last_mainte     nance":2000}}
	//{"type":"SyncItem","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","name":"location1_air_humidity","type":"air_humidity","device_name":"地点1温湿度","status":"Abnormal","status_changed_     at":1000,"available":true}}
	//{"type":"SyncItem","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","name":"location1_air_visibility","type":"air_visibility","device_name":"地点1能见度","status":"Normal","status_change     d_at":2000,"available":true}}
	//{"type":"EditDeviceRecord","body":{"id":"2e22d467-0375-4555-8a9e-ce89ad5a8f98","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","device_name":"地点1温湿度","record":"1","created_at":1000,"updated_     at":1200,"version":2}}
	//{"type":"EditDeviceRecord","body":{"id":"46944184-5dd6-4457-be7d-420277c677ae","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","device_name":"地点1能见度","record":"2","created_at":1000,"updated_     at":1200,"version":2}}

	//{"type":"MissItemStatus","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","identifier":"","row_id":1,"item_name":"location1_air_humidity","status":"Normal","changed_at":1300}}
	//{"type":"MissItemStatus","body":{"station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","identifier":"","row_id":2,"item_name":"location1_air_visibility","status":"Normal","changed_at":1300}}
	//2021-12-20T11:38:09.317+0800	DEBUG	controller/sync_client_data.go:97	got miss data	{"item_name": "location1_air_visibility", "len": 2}
	//{"Type":"MissData","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","item_name":"location1_air_visibility","val":1,"msec":100}
	//{"Type":"MissData","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","item_name":"location1_air_visibility","val":1,"msec":200}
	//2021-12-20T11:38:09.336+0800	DEBUG	controller/sync_client_data.go:97	got miss data	{"item_name": "location1_air_humidity", "len": 2}
	//{"Type":"MissData","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","item_name":"location1_air_humidity","val":1,"msec":100}
	//{"Type":"MissData","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","item_name":"location1_air_humidity","val":1,"msec":200}
	//2021-12-20T11:38:09.349+0800	DEBUG	controller/sync_client_data.go:33	first receive	{"station_id": "4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a", "item_name": "location1_air_humidity"}
	//{"Type":"data","station_id":"4ac29fb6-3fc6-4283-bf5e-9deb230b7d7a","item_name":"location1_air_humidity","val":1,"msec":8000}

	var msg RcvMsgStruct
	conn4Decoder := json.NewDecoder(conn4)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncStation, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgUpdateStationStatus, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncDevice, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncDevice, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncItem, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncItem, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgEditDeviceRecord, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgEditDeviceRecord, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissItemStatus, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissItemStatus, msg.Type)

	var dataMsg forwardDataStruct
	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissData, dataMsg.Type)

	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissData, dataMsg.Type)

	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissData, dataMsg.Type)

	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissData, dataMsg.Type)

	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	require.Equal(t, kMsgData, dataMsg.Type)
}

func mockSyncServer(t *testing.T, conn net.Conn) {
	muxConfig := yamux.DefaultConfig()
	muxConfig.LogOutput = io.Discard
	session, err := yamux.Server(conn, muxConfig)
	require.NoError(t, err)

	defer func() { _ = session.Close() }()
	// stream1: full sync
	// stream2: increment sync
	// stream3: data sync
	stream1, err := session.Accept()
	require.NoError(t, err)

	{
		logger.Debug("update available")
		err = json.NewEncoder(stream1).Encode(SendMsgStruct{Type: kMsgUpdateAvailable, Body: permissions})
		require.NoError(t, err)
	}
	// increment and full are separated to avoid receiving increment first
	// make sure subscribe first
	{
		subscriber := pubsub.NewSubscriber(nil, stream1)
		configPubSub.SubscribeTopic(subscriber, nil)
		defer configPubSub.Evict(subscriber)
		statusPubSub.SubscribeTopic(subscriber, nil)
		defer statusPubSub.Evict(subscriber)
	}

	stream2, err := session.Accept()
	require.NoError(t, err)

	// Make sure stream1 have subscribed when querying the database
	mockFullSyncServer(stream2)
	_ = stream2.Close()

	go func() {
		defer func() { _ = session.Close() }()

		stream3, err := session.Accept()
		if err != nil {
			return
		}
		defer func() { _ = stream3.Close() }()

		encoder := json.NewEncoder(stream3)
		{
			err = encoder.Encode(forwardDataStruct{
				Type:              kMsgData,
				StationItemStruct: common.StationItemStruct{StationId: stationsFullInfo[0].Id, ItemName: stationsFullInfo[0].Items[0].Name},
				DataTimeStruct:    common.DataTimeStruct{Value: 1, Millisecond: 8000},
			})
			require.NoError(t, err)
		}

		stream4, err := session.Accept()
		if err != nil {
			return
		}

		mockFillMissDataServer(stream4, permissions)
		_ = stream4.Close()

		_, _ = io.Copy(io.Discard, stream3)
	}()

	_, _ = io.Copy(io.Discard, stream1)
}

func mockFullSyncServer(conn net.Conn) {
	var err error
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	logger.Debug("send full station info")
	if err = encoder.Encode(stationsFullInfo); err != nil {
		return
	}

	logger.Debug("send device record")
	err = encoder.Encode(deviceRecords)
	if err != nil {
		return
	}

	//miss status
	var stationsLatestStatusLogRowId map[uuid.UUID]int64
	if err = decoder.Decode(&stationsLatestStatusLogRowId); err != nil {
		logger.Debug(err.Error())
		return
	}
	var missStatusLogs = make(map[uuid.UUID][]common.RowIdItemStatusStruct)
	for stationId, rowId := range stationsLatestStatusLogRowId {
		missStatusLogs[stationId] = []common.RowIdItemStatusStruct{
			{RowId: rowId + 1, ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "location1_air_humidity",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: 1300},
			}},
			{RowId: rowId + 2, ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "location1_air_visibility",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: 1300},
			}},
		}
	}

	logger.Debug("send miss status log")
	if err = encoder.Encode(missStatusLogs); err != nil {
		logger.Error(err.Error())
		return
	}
}

func mockFillMissDataServer(conn net.Conn, permissions common.UUIDStringsMap) {
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	err := encoder.Encode(permissions)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	// miss data
	var stationsItemsLatest map[uuid.UUID]common.StringMsecMap
	if err = decoder.Decode(&stationsItemsLatest); err != nil {
		logger.Error(err.Error())
		return
	}

	var stationsMissData = make(map[uuid.UUID]map[string][]common.DataTimeStruct)
	for stationId, items := range permissions {
		var missData = make(map[string][]common.DataTimeStruct)
		for _, itemName := range items {
			msec := stationsItemsLatest[stationId][itemName]

			missData[itemName] = []common.DataTimeStruct{
				{Value: 1, Millisecond: msec + 100},
				{Value: 1, Millisecond: msec + 200},
			}
		}
		stationsMissData[stationId] = missData
	}

	// send missData
	if err = encoder.Encode(stationsMissData); err != nil {
		logger.Error(err.Error())
		return
	}
}
