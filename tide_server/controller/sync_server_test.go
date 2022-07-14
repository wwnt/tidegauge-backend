package controller

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"log"
	"net"
	"testing"
	"tide/common"
	"tide/tide_server/db"
)

func Test_handleSyncServerConn(t *testing.T) {
	truncateDB(t)

	conn1, conn2 := net.Pipe() // conn3: sync server   , conn4: sync client

	username := "user01"
	go func() {
		defer func() { _ = conn1.Close() }()
		handleSyncServerConn(conn1, username, permissions)
	}()

	session, err := yamux.Client(conn2, nil)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	defer func() {
		logger.Debug("sync client closed")
		_ = session.Close()
	}()

	stream1, err := session.Open()
	if err != nil {
		logger.Error(err.Error())
		return
	}
	stream2, err := session.Open()
	if err != nil {
		logger.Error(err.Error())
		return
	}

	//if !fullSyncConfigClient(stream2, upstream) {
	//	return
	//}
	//_ = stream2.Close()
	decoder := json.NewDecoder(stream2)
	encoder := json.NewEncoder(stream2)
	var stationsFull []db.StationFullInfo
	err = decoder.Decode(&stationsFull)
	require.NoError(t, err)

	var deviceRecords []db.DeviceRecord
	err = decoder.Decode(&deviceRecords)
	require.NoError(t, err)

	err = encoder.Encode(map[uuid.UUID]int64{station1.Id: 0})
	require.NoError(t, err)

	var missStatusLogs map[uuid.UUID][]common.RowIdItemStatusStruct
	err = decoder.Decode(&missStatusLogs)
	require.NoError(t, err)
	log.Println(missStatusLogs) //map[]

	_ = stream2.Close()

	defer func() { _ = session.Close() }()
	stream3, err := session.Open()
	require.NoError(t, err)

	stream4, err := session.Open()
	require.NoError(t, err)

	encoder = json.NewEncoder(stream4)
	decoder = json.NewDecoder(stream4)

	var permissions common.UUIDStringsMap
	err = decoder.Decode(&permissions)
	require.NoError(t, err)
	log.Println(permissions) // map[4affa658-951e-472c-a6d5-d02ca0266267:[location1_air_humidity location1_air_visibility]]

	var stationsItemsLatest = make(map[uuid.UUID]common.StringMsecMap)
	for stationId, items := range permissions {
		var itemsLatest = make(common.StringMsecMap)
		for _, itemName := range items {
			itemsLatest[itemName] = 1
		}

		stationsItemsLatest[stationId] = itemsLatest
	}
	err = encoder.Encode(stationsItemsLatest)
	require.NoError(t, err)

	var stationsMissData map[uuid.UUID]map[string][]common.DataTimeStruct
	err = decoder.Decode(&stationsMissData)
	require.NoError(t, err)
	log.Println(stationsMissData) //map[4affa658-951e-472c-a6d5-d02ca0266267:map[]]

	_ = stream4.Close()
	_ = stream3.Close()

	//syncDataClient(stream3)
	//_ = stream3.Close()

	//incrementSyncConfigClient(stream1, upstream)
	_ = stream1.Close()
}
