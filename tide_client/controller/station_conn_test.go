package controller

import (
	"encoding/json"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"tide/common"
	"tide/tide_client/db"
	"tide/tide_client/global"
)

func Test_stationConn(t *testing.T) {
	db.InitData(t)

	conn1, conn2 := net.Pipe() //conn1: station conn client

	go func() {
		defer func() {
			log.Println("close station client")
			_ = conn1.Close()
		}()
		stationConn(conn1, dataPubSub)
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

		{
			var msg common.ReceiveMsgStruct
			err = decoder.Decode(&msg)
			require.NoError(t, err)
			assert.Equal(t, common.MsgItemStatus, msg.Type)
		}

		{
			var msg common.ReceiveMsgStruct
			err = decoder.Decode(&msg)
			require.NoError(t, err)
			assert.Equal(t, common.MsgData, msg.Type)
		}
	})
	t.Run("camera", func(t *testing.T) {
		tmpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("jpg1"))
		}))
		defer tmpServer.Close()
		tmp := global.Config.Cameras.List["camera1"]
		tmp.Snapshot = tmpServer.URL
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
