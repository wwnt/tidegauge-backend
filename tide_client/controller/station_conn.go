package controller

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/device/camera"
	"tide/tide_client/global"
	"time"

	"github.com/hashicorp/yamux"
)

var stationInfo = common.StationInfoStruct{
	Devices: make(map[string]map[string]string),
}

// onvifSnapshot is overridden in tests to avoid binding/listening to real sockets.
var onvifSnapshot = camera.OnvifSnapshot

func client(dataBroker *pubsub.Broker) {
	conn, err := net.Dial("tcp", global.Config.Server)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	slog.Info("Connected", "server", global.Config.Server, "local_addr", conn.LocalAddr())

	stationConn(conn, dataBroker)
}

func stationConn(conn net.Conn, dataBroker *pubsub.Broker) {
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
	ingestMu.Lock()

	{
		var missData = make(map[string][]common.DataTimeStruct)
		for _, dv := range stationInfo.Devices {
			for _, itemName := range dv {
				// NOTE: custype.UnixMs methods use pointer receivers; map index is not addressable.
				start := itemsLatest[itemName]
				ds, err := db.GetDataHistory(itemName, start.ToInt64(), 0)
				if err != nil {
					slog.Error("Failed to get data history", "item_name", itemName, "error", err)
					ingestMu.Unlock()
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
			ingestMu.Unlock()
			return
		}

		missStatusLogs, err := db.GetItemStatusLogAfter(latestStatusLogRowId)
		if err != nil {
			slog.Error("Failed to get missing status logs", "after_row_id", latestStatusLogRowId, "error", err)
			ingestMu.Unlock()
			return
		}
		// send missStatusLogs
		if err = encoder.Encode(missStatusLogs); err != nil {
			slog.Error("Failed to send missing status logs", "error", err)
			ingestMu.Unlock()
			return
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-session.CloseChan()
		cancel()
	}()
	subscriber := newSubscriber(ctx, func() { _ = session.Close() }, jsonWriter(stream1))

	dataBroker.Subscribe(subscriber, nil)
	defer dataBroker.Unsubscribe(subscriber)

	ingestMu.Unlock()

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

func cameraSnapshot(conn net.Conn) {
	var name string
	if json.NewDecoder(conn).Decode(&name) != nil {
		return
	}
	cam, ok := global.Config.Cameras.List[name]
	if !ok {
		return
	}
	bs, err := onvifSnapshot(cam.Snapshot, cam.Username, cam.Password)
	if err != nil {
		slog.Error("Camera snapshot failed", "camera_name", name, "error", err)
		return
	}
	_, _ = conn.Write(bs)
}

// newSubscriber creates a subscriber. A goroutine reads values from Messages
// and passes them to write until ctx is canceled, the subscriber is closed,
// or write returns an error.
func newSubscriber(ctx context.Context, cancel context.CancelFunc, write func(any) error) *pubsub.Subscriber {
	subscriber := pubsub.NewSubscriber(1000, cancel)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case val, ok := <-subscriber.Ch:
				if !ok {
					return
				}
				if write(val) != nil {
					return
				}
			}
		}
	}()
	return subscriber
}

// jsonWriter returns a write function that JSON-encodes values to w.
func jsonWriter(w io.Writer) func(any) error {
	return func(val any) error {
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
}
