package controller

import (
	"encoding/json"
	"log/slog"
	"net"
	"testing"

	"tide/common"
	"tide/tide_server/db"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
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
		slog.Error("Failed to create yamux client session in test", "error", err)
		return
	}
	defer func() {
		slog.Debug("Sync client closed in test")
		_ = session.Close()
	}()

	stream1, err := session.Open()
	if err != nil {
		slog.Error("Failed to open stream1 in test", "error", err)
		return
	}
	stream2, err := session.Open()
	if err != nil {
		slog.Error("Failed to open stream2 in test", "error", err)
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
	slog.Debug("Miss status logs in test", "data", missStatusLogs)

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
	slog.Debug("Permissions in test", "data", permissions)

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
	slog.Debug("Stations miss data in test", "data", stationsMissData)

	_ = stream4.Close()
	_ = stream3.Close()

	//syncDataClient(stream3)
	//_ = stream3.Close()

	//incrementSyncConfigClient(stream1, upstream)
	_ = stream1.Close()
}
