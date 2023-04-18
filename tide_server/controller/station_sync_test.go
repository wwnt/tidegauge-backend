package controller

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"tide/common"
	"tide/tide_server/db"
)

func Test_handleStationConnStream1(t *testing.T) {
	truncateDB(t)

	err := db.EditStation(&db.Station{Identifier: station1.Identifier, Name: station1.Name})
	require.NoError(t, err)

	conn1, conn2 := net.Pipe() // conn1: station client, conn2: station server
	conn3, conn4 := net.Pipe() // conn3: sync server   , conn4: sync client
	defer func() { _ = conn3.Close() }()

	configPubSub.SubscribeTopic(conn3, nil)
	defer configPubSub.Evict(conn3)

	statusPubSub.SubscribeTopic(conn3, nil)
	defer statusPubSub.Evict(conn3)

	dataPubSub.SubscribeTopic(conn3, nil)
	defer dataPubSub.Evict(conn3)

	missDataPubSub.SubscribeTopic(conn3, nil)
	defer missDataPubSub.Evict(conn3)

	go func() {
		mockStationClient(t, conn1, station1Info)
		_ = conn1.Close()
	}()
	go func() {
		defer func() { _ = conn2.Close() }()
		handleStationConnStream1(conn2, nil)
	}()

	var msg RcvMsgStruct
	var dataMsg forwardDataStruct
	conn4Decoder := json.NewDecoder(conn4)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgUpdateStationStatus, msg.Type)

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncDevice, msg.Type)
	//require.JSONEq(t, `{"station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","name":"地点1温湿度","specs":null,"last_maintenance":0}`, string(msg.Body))

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncDevice, msg.Type)
	//require.JSONEq(t, `{"station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","name":"地点1能见度","specs":null,"last_maintenance":0}`, string(msg.Body))

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncItem, msg.Type)
	//require.JSONEq(t, `{"station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","name":"location1_air_visibility","type":"air_visibility","device_name":"地点1能见度","status":"","status_changed_at":0,"available":false}`, string(msg.Body))

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncItem, msg.Type)
	//require.JSONEq(t, `{"station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","name":"location1_air_humidity","type":"air_humidity","device_name":"地点1温湿度","status":"","status_changed_at":0,"available":false}`, string(msg.Body))

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgSyncStationCannotEdit, msg.Type)
	//require.JSONEq(t, `{"id":"ab49b2f1-5ef1-11ec-9a91-3497f6278c0d","cameras":["camera1"],"upstream":false}`, string(msg.Body))

	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	//require.JSONEq(t, `{"Type":"MissData","station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","item_name":"location1_air_humidity","d":1,"ts":1}`, string(msg.Body))

	err = conn4Decoder.Decode(&dataMsg)
	require.NoError(t, err)
	//require.JSONEq(t, `{"Type":"MissData","station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","item_name":"location1_air_visibility","d":1,"ts":1}`, string(msg.Body))

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgMissItemStatus, msg.Type)
	//require.JSONEq(t, `{"station_id":"5f32eecb-5ee6-11ec-82b1-3497f6278c0d","identifier":"","row_id":1,"item_name":"item1","status":"Abnormal","changed_at":0}`, string(msg.Body))

	err = conn4Decoder.Decode(&msg)
	require.NoError(t, err)
	require.Equal(t, kMsgUpdateStationStatus, msg.Type)
	//require.JSONEq(t, `{"station_id":"ab49b2f1-5ef1-11ec-9a91-3497f6278c0d","identifier":"station1","status":"Disconnected","changed_at":1639715259626}`, string(msg.Body))
	//io.Copy(os.Stdout, conn4)
	_ = conn4.Close()
}

func mockStationClient(t *testing.T, conn net.Conn, info common.StationInfoStruct) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	//send stationInfo
	err := encoder.Encode(info)
	require.NoError(t, err)

	// receive latest data time
	var itemsLatest common.StringMsecMap
	err = decoder.Decode(&itemsLatest)
	require.NoError(t, err)

	// receive rowId
	var latestStatusLogRowId int64
	err = decoder.Decode(&latestStatusLogRowId)
	require.NoError(t, err)

	var missData = make(map[string][]common.DataTimeStruct)
	for itemName, msec := range itemsLatest {
		for i := 0; i < 1; i++ {
			msec++
			missData[itemName] = append(missData[itemName], common.DataTimeStruct{Value: 1, Millisecond: msec})
		}
	}

	var missStatusLogs []common.RowIdItemStatusStruct
	for i := 0; i < 1; i++ {
		latestStatusLogRowId++
		missStatusLogs = append(missStatusLogs, common.RowIdItemStatusStruct{
			RowId: latestStatusLogRowId,
			ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "item1",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: 0},
			},
		})
	}

	//dataSub.SubscribeTopic(stream1, nil)
	//defer dataSub.Evict(stream1)

	// send missData
	err = encoder.Encode(missData)
	require.NoError(t, err)

	// send missStatusLogs
	err = encoder.Encode(missStatusLogs)
	require.NoError(t, err)
}
