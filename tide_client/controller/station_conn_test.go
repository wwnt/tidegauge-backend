package controller

import (
	"encoding/json"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"net"
	"testing"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/global"
	"time"
)

func Test_stationConn(t *testing.T) {
	db.InitData(t)

	conn1, conn2 := net.Pipe() //conn1: station conn client

	go func() {
		defer func() {
			log.Println("close station client")
			_ = conn1.Close()
		}()
		stationConn(conn1, dataBroker)
	}()

	stationInfo = station1Info

	session, err := yamux.Server(conn2, nil)
	if err != nil {
		return
	}
	defer func() { _ = session.Close() }()

	t.Run("stream1", func(t *testing.T) {
		conn, err := session.Open()
		if err != nil {
			return
		}

		decoder := json.NewDecoder(conn)
		encoder := json.NewEncoder(conn)

		var info common.StationInfoStruct
		err = decoder.Decode(&info)
		require.NoError(t, err)

		itemsLatest := common.StringMsecMap{"item1": 0}
		err = encoder.Encode(itemsLatest)
		require.NoError(t, err)

		latestStatusLogRowId := 0
		err = encoder.Encode(latestStatusLogRowId)
		require.NoError(t, err)

		var missData map[string][]common.DataTimeStruct
		err = decoder.Decode(&missData)
		require.NoError(t, err)
		assert.Equal(t, map[string][]common.DataTimeStruct{
			"item1": {common.DataTimeStruct{Value: 1, Millisecond: 1100}},
			"item2": {common.DataTimeStruct{Value: 1, Millisecond: 1100}},
		}, missData)

		var missStatusLogs []common.RowIdItemStatusStruct
		err = decoder.Decode(&missStatusLogs)
		require.NoError(t, err)
		assert.Equal(t, db.StatusLogs, missStatusLogs)
	})
	t.Run("camera", func(t *testing.T) {
		oldSnapshotFn := onvifSnapshot
		onvifSnapshot = func(url, username, password string) ([]byte, error) { return []byte("jpg1"), nil }
		t.Cleanup(func() { onvifSnapshot = oldSnapshotFn })

		// Keep config setup here to ensure we're still reading from the camera map.
		tmp := global.Config.Cameras.List["camera1"]
		tmp.Snapshot = "http://example.invalid/snapshot"
		global.Config.Cameras.List["camera1"] = tmp

		conn, err := session.Open()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		_, err = conn.Write([]byte{common.MsgCameraSnapShot})
		require.NoError(t, err)

		err = json.NewEncoder(conn).Encode("camera1")
		require.NoError(t, err)

		bytes, err := io.ReadAll(conn)
		require.NoError(t, err)
		assert.Equal(t, []byte("jpg1"), bytes)
	})
}

func Test_stationConn_OverflowClosesSession(t *testing.T) {
	db.InitData(t)

	broker := pubsub.NewBroker()
	conn1, conn2 := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		stationConn(conn1, broker)
	}()
	t.Cleanup(func() { _ = conn1.Close() })
	t.Cleanup(func() { _ = conn2.Close() })

	stationInfo = station1Info

	session, err := yamux.Server(conn2, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	conn, err := session.Open()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var info common.StationInfoStruct
	err = decoder.Decode(&info)
	require.NoError(t, err)

	err = encoder.Encode(common.StringMsecMap{"item1": 0})
	require.NoError(t, err)
	err = encoder.Encode(0)
	require.NoError(t, err)

	var missData map[string][]common.DataTimeStruct
	err = decoder.Decode(&missData)
	require.NoError(t, err)
	var missStatusLogs []common.RowIdItemStatusStruct
	err = decoder.Decode(&missStatusLogs)
	require.NoError(t, err)

	msg := common.SendMsgStruct{
		Type: common.MsgData,
		Body: common.ItemNameDataTimeStruct{
			ItemName: "item1",
			DataTimeStruct: common.DataTimeStruct{
				Value: 1,
			},
		},
	}
	for i := 0; i < 20000; i++ {
		broker.Publish(msg, nil)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for dropped subscriber to close station sync session")
	}
}
