package syncv2

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	dataByItem map[string][]common.DataTimeStruct
	statusLogs []common.RowIdItemStatusStruct
}

func (s fakeStore) GetDataHistory(item string, startUnixMs, endUnixMs int64) ([]common.DataTimeStruct, error) {
	return s.dataByItem[item], nil
}

func (s fakeStore) GetItemStatusLogAfter(afterRowID int64) ([]common.RowIdItemStatusStruct, error) {
	return s.statusLogs, nil
}

type fakeBroker struct {
	subscribeCh chan *pubsub.Subscriber
}

func (b *fakeBroker) Subscribe(ch *pubsub.Subscriber, _ pubsub.TopicSet) {
	if b.subscribeCh != nil {
		b.subscribeCh <- ch
	}
}

func (b *fakeBroker) Unsubscribe(_ *pubsub.Subscriber) {}

type fakeCameraLookup struct {
	ok bool
}

func (l fakeCameraLookup) GetCamera(name string) (snapshotURL, username, password string, ok bool) {
	if l.ok && name == "cam1" {
		return "http://example.invalid/snap", "u", "p", true
	}
	return "", "", "", false
}

type fakeSnapshotter struct {
	data []byte
	err  error
}

func (s fakeSnapshotter) Snapshot(url, username, password string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte(nil), s.data...), nil
}

func recvStationFrame(t *testing.T, stream internalsyncv2.StationMessageStream) *syncpb.StationMessage {
	t.Helper()

	type res struct {
		f   *syncpb.StationMessage
		err error
	}
	ch := make(chan res, 1)
	go func() {
		f, err := stream.Recv()
		ch <- res{f: f, err: err}
	}()
	select {
	case r := <-ch:
		require.NoError(t, r.err)
		require.NotNil(t, r.f)
		return r.f
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for station frame")
		return nil
	}
}

func openServerCommandStream(t *testing.T, session *yamux.Session) internalsyncv2.StationMessageStream {
	t.Helper()

	conn, err := session.Open()
	require.NoError(t, err)
	stream := internalsyncv2.NewStationMessageStream(conn, internalsyncv2.DefaultMaxFrameBytes)
	t.Cleanup(func() { _ = stream.Close() })
	return stream
}

func setupClientConn(t *testing.T, ctx context.Context, c *Client) (internalsyncv2.StationMessageStream, *yamux.Session, chan error) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
	t.Cleanup(func() { _ = serverConn.Close() })

	clientErrCh := make(chan error, 1)
	go func() { clientErrCh <- c.runOnConn(ctx, clientConn) }()

	serverSession, err := yamux.Server(serverConn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	mainConn, err := serverSession.Open()
	require.NoError(t, err)
	mainStream := internalsyncv2.NewStationMessageStream(mainConn, internalsyncv2.DefaultMaxFrameBytes)
	t.Cleanup(func() { _ = mainStream.Close() })

	return mainStream, serverSession, clientErrCh
}

func TestClient_RunOnConn_ReplayRealtimeAndSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := fakeStore{
		dataByItem: map[string][]common.DataTimeStruct{
			"item1": {{Value: 1, Millisecond: custype.UnixMs(1100)}},
			"item2": {{Value: 2, Millisecond: custype.UnixMs(1200)}},
		},
		statusLogs: []common.RowIdItemStatusStruct{
			{
				RowId: 1,
				ItemStatusStruct: common.ItemStatusStruct{
					ItemName: "item1",
					StatusChangeStruct: common.StatusChangeStruct{
						Status:    "Normal",
						ChangedAt: custype.UnixMs(1300),
					},
				},
			},
		},
	}
	broker := &fakeBroker{subscribeCh: make(chan *pubsub.Subscriber, 1)}
	cameraLookup := fakeCameraLookup{ok: true}
	snapshotter := fakeSnapshotter{data: []byte("abcd")}

	c, err := NewClient(
		Config{
			Addr:              "http://station.example",
			StationIdentifier: "station1",
		},
		Deps{
			StationInfoFn: func() common.StationInfoStruct {
				return common.StationInfoStruct{
					Identifier: "station1",
					Devices: common.StringMapMap{
						"dev1": {
							"t1": "item1",
							"t2": "item2",
						},
					},
					Cameras: []string{"cam1"},
				}
			},
			GetDataHistory:        store.GetDataHistory,
			GetItemStatusLogAfter: store.GetItemStatusLogAfter,
			Subscribe:             broker.Subscribe,
			Unsubscribe:           broker.Unsubscribe,
			IngestLock:            &sync.Mutex{},
			GetCamera:             cameraLookup.GetCamera,
			Snapshot:              snapshotter.Snapshot,
		},
	)
	require.NoError(t, err)

	serverStream, serverSession, clientErrCh := setupClientConn(t, ctx, c)

	// ---- handshake ----
	{
		f := recvStationFrame(t, serverStream)
		hello, ok := f.Body.(*syncpb.StationMessage_ClientHello)
		require.True(t, ok)
		require.Equal(t, "station1", hello.ClientHello.StationIdentifier)
		require.Equal(t, internalsyncv2.ProtocolVersion, hello.ClientHello.ProtocolVersion)
	}
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ServerHello{
		ServerHello: &syncpb.ServerHello{ServerVersion: "test"},
	}}))
	{
		f := recvStationFrame(t, serverStream)
		info, ok := f.Body.(*syncpb.StationMessage_StationInfo)
		require.True(t, ok)
		require.Equal(t, "station1", info.StationInfo.Identifier)
	}

	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ItemsLatest{
		ItemsLatest: &syncpb.ItemsLatest{LatestUnixMs: map[string]int64{"item1": 0, "item2": 0}},
	}}))
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_StatusLatest{
		StatusLatest: &syncpb.StatusLatest{LatestRowId: 0},
	}}))

	// ---- replay ----
	{
		f := recvStationFrame(t, serverStream)
		db, ok := f.Body.(*syncpb.StationMessage_DataBatch)
		require.True(t, ok)
		require.True(t, db.DataBatch.Replay)
		require.Len(t, db.DataBatch.Points, 2)

		got := make(map[string]*syncpb.DataPoint)
		for _, p := range db.DataBatch.Points {
			got[p.ItemName] = p
		}
		require.Contains(t, got, "item1")
		require.Contains(t, got, "item2")
		require.Equal(t, float64(1), got["item1"].Value)
		require.Equal(t, int64(1100), got["item1"].UnixMs)
		require.Equal(t, syncpb.DataKind_DATA_KIND_NORMAL, got["item1"].Kind)
		require.Equal(t, float64(2), got["item2"].Value)
		require.Equal(t, int64(1200), got["item2"].UnixMs)
		require.Equal(t, syncpb.DataKind_DATA_KIND_NORMAL, got["item2"].Kind)
	}
	{
		f := recvStationFrame(t, serverStream)
		sb, ok := f.Body.(*syncpb.StationMessage_ItemStatusBatch)
		require.True(t, ok)
		require.True(t, sb.ItemStatusBatch.Replay)
		require.Len(t, sb.ItemStatusBatch.Logs, 1)
		require.Equal(t, int64(1), sb.ItemStatusBatch.Logs[0].RowId)
		require.Equal(t, "item1", sb.ItemStatusBatch.Logs[0].ItemName)
		require.Equal(t, "Normal", sb.ItemStatusBatch.Logs[0].Status)
		require.Equal(t, int64(1300), sb.ItemStatusBatch.Logs[0].ChangedAtUnixMs)
	}

	// ---- snapshot (command substream) ----
	cmdStream := openServerCommandStream(t, serverSession)
	require.NoError(t, cmdStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotRequest{
		CameraSnapshotRequest: &syncpb.CameraSnapshotRequest{
			CameraName: "cam1",
		},
	}}))
	{
		f := recvStationFrame(t, cmdStream)
		resp, ok := f.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
		require.True(t, ok)
		require.Empty(t, resp.CameraSnapshotResponse.Error)
		require.Equal(t, []byte("abcd"), resp.CameraSnapshotResponse.Data)
	}

	// ---- realtime ----
	var subscriber *pubsub.Subscriber
	select {
	case subscriber = <-broker.subscribeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for broker.Subscribe")
	}

	subscriber.Ch <- common.SendMsgStruct{
		Type: common.MsgGpioData,
		Body: common.ItemNameDataTimeStruct{
			ItemName: "item1",
			DataTimeStruct: common.DataTimeStruct{
				Value:       9,
				Millisecond: custype.UnixMs(2000),
			},
		},
	}

	{
		f := recvStationFrame(t, serverStream)
		db, ok := f.Body.(*syncpb.StationMessage_DataBatch)
		require.True(t, ok)
		require.False(t, db.DataBatch.Replay)
		require.Len(t, db.DataBatch.Points, 1)
		require.Equal(t, "item1", db.DataBatch.Points[0].ItemName)
		require.Equal(t, float64(9), db.DataBatch.Points[0].Value)
		require.Equal(t, int64(2000), db.DataBatch.Points[0].UnixMs)
		require.Equal(t, syncpb.DataKind_DATA_KIND_GPIO, db.DataBatch.Points[0].Kind)
	}

	cancel()
	err = <-clientErrCh
	require.True(t, err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func TestClient_RunOnConn_SnapshotCameraNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := fakeStore{dataByItem: map[string][]common.DataTimeStruct{}}
	broker := &fakeBroker{}
	cameraLookup := fakeCameraLookup{ok: false}
	snapshotter := fakeSnapshotter{data: []byte("ignored")}

	c, err := NewClient(
		Config{
			Addr:              "http://station.example",
			StationIdentifier: "station1",
		},
		Deps{
			StationInfoFn: func() common.StationInfoStruct {
				return common.StationInfoStruct{
					Identifier: "station1",
					Devices: common.StringMapMap{
						"dev1": {"t1": "item1"},
					},
					Cameras: []string{},
				}
			},
			GetDataHistory:        store.GetDataHistory,
			GetItemStatusLogAfter: store.GetItemStatusLogAfter,
			Subscribe:             broker.Subscribe,
			Unsubscribe:           broker.Unsubscribe,
			IngestLock:            &sync.Mutex{},
			GetCamera:             cameraLookup.GetCamera,
			Snapshot:              snapshotter.Snapshot,
		},
	)
	require.NoError(t, err)

	serverStream, serverSession, clientErrCh := setupClientConn(t, ctx, c)

	_ = recvStationFrame(t, serverStream) // client hello
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ServerHello{
		ServerHello: &syncpb.ServerHello{ServerVersion: "test"},
	}}))
	_ = recvStationFrame(t, serverStream) // station info
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ItemsLatest{
		ItemsLatest: &syncpb.ItemsLatest{LatestUnixMs: map[string]int64{"item1": 0}},
	}}))
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_StatusLatest{
		StatusLatest: &syncpb.StatusLatest{LatestRowId: 0},
	}}))

	cmdStream := openServerCommandStream(t, serverSession)
	require.NoError(t, cmdStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotRequest{
		CameraSnapshotRequest: &syncpb.CameraSnapshotRequest{
			CameraName: "missing",
		},
	}}))
	f := recvStationFrame(t, cmdStream)
	resp, ok := f.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
	require.True(t, ok)
	require.Equal(t, []byte(nil), resp.CameraSnapshotResponse.Data)
	require.Equal(t, "camera not found", resp.CameraSnapshotResponse.Error)

	cancel()
	_ = <-clientErrCh
}

func TestClient_RunOnConn_DroppedSubscriberClosesSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := fakeStore{dataByItem: map[string][]common.DataTimeStruct{}}
	broker := &fakeBroker{subscribeCh: make(chan *pubsub.Subscriber, 1)}

	c, err := NewClient(
		Config{
			Addr:              "http://station.example",
			StationIdentifier: "station1",
		},
		Deps{
			StationInfoFn: func() common.StationInfoStruct {
				return common.StationInfoStruct{
					Identifier: "station1",
					Devices: common.StringMapMap{
						"dev1": {"t1": "item1"},
					},
				}
			},
			GetDataHistory:        store.GetDataHistory,
			GetItemStatusLogAfter: store.GetItemStatusLogAfter,
			Subscribe:             broker.Subscribe,
			Unsubscribe:           broker.Unsubscribe,
			IngestLock:            &sync.Mutex{},
			GetCamera:             fakeCameraLookup{}.GetCamera,
			Snapshot:              fakeSnapshotter{}.Snapshot,
		},
	)
	require.NoError(t, err)

	serverStream, _, clientErrCh := setupClientConn(t, ctx, c)

	_ = recvStationFrame(t, serverStream)
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ServerHello{
		ServerHello: &syncpb.ServerHello{ServerVersion: "test"},
	}}))
	_ = recvStationFrame(t, serverStream)
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ItemsLatest{
		ItemsLatest: &syncpb.ItemsLatest{LatestUnixMs: map[string]int64{"item1": 0}},
	}}))
	require.NoError(t, serverStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_StatusLatest{
		StatusLatest: &syncpb.StatusLatest{LatestRowId: 0},
	}}))

	var subscriber *pubsub.Subscriber
	select {
	case subscriber = <-broker.subscribeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for broker.Subscribe")
	}
	subscriber.Cancel()

	select {
	case err = <-clientErrCh:
		require.True(t, err == nil || errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "session shutdown"))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for dropped subscriber to close session")
	}
}
