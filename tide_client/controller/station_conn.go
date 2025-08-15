package controller

import (
	"encoding/json"
	"github.com/hashicorp/yamux"
	"io"
	"net"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/device"
	"tide/tide_client/device/camera"
	"tide/tide_client/global"
	"time"
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
	global.Log.Debugf("dial to server %v, local addr: %v", global.Config.Server, conn.LocalAddr())

	stationConn(conn, dataSub)
}

func stationConn(conn net.Conn, dataSub *pubsub.PubSub) {
	cnf := yamux.DefaultConfig()
	cnf.EnableKeepAlive = false
	cnf.ConnectionWriteTimeout = 30 * time.Second
	session, err := yamux.Client(conn, nil)
	if err != nil {
		global.Log.Error(err)
		return
	}
	defer func() { _ = session.Close() }()
	stream1, err := session.Accept()
	if err != nil {
		global.Log.Error(err)
		return
	}
	defer func() { _ = stream1.Close() }()
	var (
		encoder = json.NewEncoder(stream1)
		decoder = json.NewDecoder(stream1)
	)

	//send stationInfo
	if err = encoder.Encode(stationInfo); err != nil {
		global.Log.Error(err)
		return
	}

	// receive latest data time
	var itemsLatest common.StringMsecMap
	if err = decoder.Decode(&itemsLatest); err != nil {
		global.Log.Error(err)
		return
	}
	// receive rowId
	var latestStatusLogRowId int64
	if err = decoder.Decode(&latestStatusLogRowId); err != nil {
		global.Log.Error(err)
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
					global.Log.Error(err)
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
			global.Log.Error(err)
			dataReceiveMu.Unlock()
			return
		}

		missStatusLogs, err := db.GetItemStatusLogAfter(latestStatusLogRowId)
		if err != nil {
			global.Log.Error(err)
			dataReceiveMu.Unlock()
			return
		}
		// send missStatusLogs
		if err = encoder.Encode(missStatusLogs); err != nil {
			global.Log.Error(err)
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
					global.Log.Error(err)
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
			global.Log.Error(err)
			return
		}
		global.Log.Debug(msg)
		deviceConn, ok := device.DevicesUartConn[msg.DeviceName]
		if !ok {
			//dataReceiveMu.Unlock()
			return
		}
		// Exclusive access to this connection
		received, err := deviceConn.CustomCommand([]byte(msg.Command))

		if err != nil {
			received = []byte(err.Error())
			global.Log.Error(err)
			continue
		}
		if err = encoder.Encode(string(received)); err != nil {
			global.Log.Error(err)
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
		global.Log.Error(err)
		return
	}
	_, _ = conn.Write(bs)
}
