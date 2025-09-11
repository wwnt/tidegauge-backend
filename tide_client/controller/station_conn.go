package controller

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/device"
	"tide/tide_client/device/camera"
	"tide/tide_client/global"
	"time"

	"github.com/hashicorp/yamux"
)

var stationInfo = common.StationInfoStruct{
	Devices: make(map[string]map[string]string),
}

func client(dataSub *pubsub.PubSub) {
	conn, err := net.Dial("tcp", global.Config.Server)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	slog.Info("Connected", "server", global.Config.Server, "local_addr", conn.LocalAddr())

	stationConn(conn, dataSub)
}

func stationConn(conn net.Conn, dataSub *pubsub.PubSub) {
	cnf := yamux.DefaultConfig()
	cnf.EnableKeepAlive = false
	cnf.ConnectionWriteTimeout = 30 * time.Second
	session, err := yamux.Client(conn, cnf)
	if err != nil {
		slog.Error("Failed to create yamux client", "error", err)
		return
	}
	defer func() { _ = session.Close() }()
	stream1, err := session.Accept()
	if err != nil {
		slog.Error("Failed to accept stream", "error", err)
		return
	}
	defer func() { _ = stream1.Close() }()
	var (
		encoder = json.NewEncoder(stream1)
		decoder = json.NewDecoder(stream1)
	)

	//send stationInfo
	if err = encoder.Encode(stationInfo); err != nil {
		slog.Error("Failed to send station info", "error", err)
		return
	}

	// receive latest data time
	var itemsLatest common.StringMsecMap
	if err = decoder.Decode(&itemsLatest); err != nil {
		slog.Error("Failed to receive latest data time", "error", err)
		return
	}
	// receive rowId
	var latestStatusLogRowId int64
	if err = decoder.Decode(&latestStatusLogRowId); err != nil {
		slog.Error("Failed to receive status log row id", "error", err)
		return
	}
	// lock database operation
	dataReceiveMu.Lock()

	{
		var missData = make(map[string][]common.DataTimeStruct)
		for _, dv := range stationInfo.Devices {
			for _, itemName := range dv {
				ds, err := db.GetDataHistory(itemName, itemsLatest[itemName].ToInt64(), 0)
				if err != nil {
					slog.Error("Failed to get data history", "item_name", itemName, "error", err)
					dataReceiveMu.Unlock()
					return
				}
				if len(ds) > 0 {
					missData[itemName] = ds
				}
			}
		}
		// send missData
		if err = encoder.Encode(missData); err != nil {
			slog.Error("Failed to send missing data", "error", err)
			dataReceiveMu.Unlock()
			return
		}

		missStatusLogs, err := db.GetItemStatusLogAfter(latestStatusLogRowId)
		if err != nil {
			slog.Error("Failed to get missing status logs", "after_row_id", latestStatusLogRowId, "error", err)
			dataReceiveMu.Unlock()
			return
		}
		// send missStatusLogs
		if err = encoder.Encode(missStatusLogs); err != nil {
			slog.Error("Failed to send missing status logs", "error", err)
			dataReceiveMu.Unlock()
			return
		}
	}
	subscriber := pubsub.NewSubscriber(session.CloseChan(), stream1)

	dataSub.SubscribeTopic(subscriber, nil)
	defer dataSub.Evict(subscriber)

	dataReceiveMu.Unlock()

	go func() {
		defer func() { _ = session.Close() }()
		for {
			stream, err := session.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = stream.Close() }()
				var buf = make([]byte, 1)
				if _, err = stream.Read(buf); err != nil {
					slog.Error("Failed to read stream data", "error", err)
					return
				}
				switch buf[0] {
				case common.MsgPortTerminal:
					portTerminal(stream)
				case common.MsgCameraSnapShot:
					cameraSnapshot(stream)
				default:
				}
			}()
		}
	}()
	// close the session if stream1 is closed
	_, _ = io.Copy(io.Discard, stream1)
}

func portTerminal(conn net.Conn) {
	var err error
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	for {
		var msg common.PortTerminalStruct
		if err = decoder.Decode(&msg); err != nil {
			slog.Error("Failed to decode terminal message", "error", err)
			return
		}
		slog.Debug("Terminal message", "device_name", msg.DeviceName, "command", msg.Command)
		deviceConn, ok := device.DevicesUartConn[msg.DeviceName]
		if !ok {
			//dataReceiveMu.Unlock()
			return
		}
		// Exclusive access to this connection
		received, err := deviceConn.CustomCommand([]byte(msg.Command))

		if err != nil {
			received = []byte(err.Error())
			slog.Error("Failed to execute device command", "device_name", msg.DeviceName, "command", msg.Command, "error", err)
			continue
		}
		if err = encoder.Encode(string(received)); err != nil {
			slog.Error("Failed to encode response data", "error", err)
			return
		}
	}
}

func cameraSnapshot(conn net.Conn) {
	var name string
	if json.NewDecoder(conn).Decode(&name) != nil {
		return
	}
	cam, ok := global.Config.Cameras.List[name]
	if !ok {
		return
	}
	bs, err := camera.OnvifSnapshot(cam.Snapshot, cam.Username, cam.Password)
	if err != nil {
		slog.Error("Camera snapshot failed", "camera_name", name, "error", err)
		return
	}
	_, _ = conn.Write(bs)
}
