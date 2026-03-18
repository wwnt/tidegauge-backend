package syncv2station

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	stationID uuid.UUID

	gotIdentifier string
	gotRemoteAddr string

	itemsLatest       map[string]int64
	latestStatusRowID int64

	mu                  sync.Mutex
	updateItemStatusLog []common.RowIdItemStatusStruct
	updateItemStatus    []struct {
		item   string
		status common.Status
		at     time.Time
	}
}

func (s *fakeStore) StationIDByIdentifier(identifier string) (uuid.UUID, error) {
	s.gotIdentifier = identifier
	return s.stationID, nil
}

func (s *fakeStore) SetStationIP(stationID uuid.UUID, remoteAddr string) error {
	s.gotRemoteAddr = remoteAddr
	return nil
}

func (s *fakeStore) ItemsLatest(stationID uuid.UUID, devices common.StringMapMap) (map[string]int64, error) {
	return s.itemsLatest, nil
}

func (s *fakeStore) LatestStatusLogRowID(stationID uuid.UUID) (int64, error) {
	return s.latestStatusRowID, nil
}

func (s *fakeStore) UpdateStationStatus(stationID uuid.UUID, status common.Status, at time.Time) (bool, error) {
	return true, nil
}

func (s *fakeStore) SaveRpiStatus(stationID uuid.UUID, cpuTemp float64, at time.Time) error {
	return nil
}

func (s *fakeStore) UpdateItemStatus(stationID uuid.UUID, itemName string, status common.Status, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateItemStatus = append(s.updateItemStatus, struct {
		item   string
		status common.Status
		at     time.Time
	}{item: itemName, status: status, at: at})
	return true, nil
}

func (s *fakeStore) SaveDataHistory(stationID uuid.UUID, itemName string, value float64, at time.Time) (bool, error) {
	return true, nil
}

func (s *fakeStore) SaveItemStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	return true, nil
}

func (s *fakeStore) UpdateAndSaveStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateItemStatusLog = append(s.updateItemStatusLog, common.RowIdItemStatusStruct{
		RowId: rowID,
		ItemStatusStruct: common.ItemStatusStruct{
			ItemName: itemName,
			StatusChangeStruct: common.StatusChangeStruct{
				Status:    status,
				ChangedAt: custype.ToUnixMs(at),
			},
		},
	})
	return true, nil
}

type fakeInfoSyncer struct {
	mu        sync.Mutex
	lastID    uuid.UUID
	lastInfo  common.StationInfoStruct
	callCount int
}

func (s *fakeInfoSyncer) SyncStationInfo(stationID uuid.UUID, info common.StationInfoStruct) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastID = stationID
	s.lastInfo = info
	s.callCount++
	return nil
}

type fakeNotifier struct {
	stationStatusCh chan common.StationStatusStruct
	missDataCh      chan common.DataTimeStruct
	realtimeDataCh  chan bool // gpio flag
}

func (n *fakeNotifier) PublishMissData(stationItem common.StationItemStruct, data common.DataTimeStruct) {
	if n.missDataCh != nil {
		n.missDataCh <- data
	}
}

func (n *fakeNotifier) PublishRealtimeData(stationItem common.StationItemStruct, data common.DataTimeStruct, gpio bool) {
	if n.realtimeDataCh != nil {
		n.realtimeDataCh <- gpio
	}
}

func (n *fakeNotifier) PublishMissItemStatus(status common.FullItemStatusStruct) {}

func (n *fakeNotifier) PublishUpdateItemStatus(status common.FullItemStatusStruct) {}

func (n *fakeNotifier) PublishStationStatus(status common.StationStatusStruct) {
	if n.stationStatusCh != nil {
		n.stationStatusCh <- status
	}
}

type stationSessions struct {
	clientSession *yamux.Session
	serverSession *yamux.Session

	clientMainStream internalsyncv2.StationMessageStream
	serverMainStream internalsyncv2.StationMessageStream
}

func newStationSessions(t *testing.T) *stationSessions {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
	t.Cleanup(func() { _ = serverConn.Close() })

	serverSession, err := yamux.Server(serverConn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	clientSession, err := yamux.Client(clientConn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSession.Close() })

	serverMainConn, err := serverSession.Open()
	require.NoError(t, err)
	clientMainConn, err := clientSession.Accept()
	require.NoError(t, err)

	clientMainStream := internalsyncv2.NewStationMessageStream(clientMainConn, internalsyncv2.DefaultMaxFrameBytes)
	serverMainStream := internalsyncv2.NewStationMessageStream(serverMainConn, internalsyncv2.DefaultMaxFrameBytes)

	t.Cleanup(func() { _ = clientMainStream.Close() })
	t.Cleanup(func() { _ = serverMainStream.Close() })

	return &stationSessions{
		clientSession:    clientSession,
		serverSession:    serverSession,
		clientMainStream: clientMainStream,
		serverMainStream: serverMainStream,
	}
}

func (s *stationSessions) openServerCommandStream() (internalsyncv2.StationMessageStream, error) {
	conn, err := s.serverSession.Open()
	if err != nil {
		return nil, err
	}
	return internalsyncv2.NewStationMessageStream(conn, internalsyncv2.DefaultMaxFrameBytes), nil
}

func (s *stationSessions) acceptClientCommandStream(timeout time.Duration) (internalsyncv2.StationMessageStream, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := s.clientSession.Accept()
		ch <- result{conn: conn, err: err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return internalsyncv2.NewStationMessageStream(r.conn, internalsyncv2.DefaultMaxFrameBytes), nil
	case <-time.After(timeout):
		return nil, context.DeadlineExceeded
	}
}

func doHandshake(t *testing.T, clientStream internalsyncv2.StationMessageStream, cameras []string) {
	t.Helper()

	require.NoError(t, clientStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ClientHello{
		ClientHello: &syncpb.ClientHello{StationIdentifier: "station1", ProtocolVersion: internalsyncv2.ProtocolVersion},
	}}))
	f, err := clientStream.Recv()
	require.NoError(t, err)
	_, ok := f.Body.(*syncpb.StationMessage_ServerHello)
	require.True(t, ok)

	require.NoError(t, clientStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_StationInfo{
		StationInfo: internalsyncv2.StationInfoToPB(common.StationInfoStruct{
			Identifier: "station1",
			Devices:    common.StringMapMap{"dev1": {"t1": "item1"}},
			Cameras:    cameras,
		}),
	}}))

	f, err = clientStream.Recv()
	require.NoError(t, err)
	_, ok = f.Body.(*syncpb.StationMessage_ItemsLatest)
	require.True(t, ok)

	f, err = clientStream.Recv()
	require.NoError(t, err)
	_, ok = f.Body.(*syncpb.StationMessage_StatusLatest)
	require.True(t, ok)
}

func snapshotRequestCameraName(t *testing.T, stream internalsyncv2.StationMessageStream) string {
	t.Helper()

	frame, err := stream.Recv()
	require.NoError(t, err)
	req, ok := frame.Body.(*syncpb.StationMessage_CameraSnapshotRequest)
	require.True(t, ok)
	require.NotNil(t, req.CameraSnapshotRequest)
	return req.CameraSnapshotRequest.CameraName
}

func TestServer_StreamStation_HandshakeAndLatest(t *testing.T) {
	stationID := uuid.New()
	store := &fakeStore{
		stationID:         stationID,
		itemsLatest:       map[string]int64{"item1": 1234},
		latestStatusRowID: 42,
	}
	infoSyncer := &fakeInfoSyncer{}
	notifier := &fakeNotifier{stationStatusCh: make(chan common.StationStatusStruct, 4)}
	srv := &Server{
		Store:      store,
		InfoSyncer: infoSyncer,
		Notifier:   notifier,
	}

	sessions := newStationSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(ctx, sessions.serverMainStream, sessions.openServerCommandStream, "1.2.3.4:5555")
	}()

	require.NoError(t, sessions.clientMainStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ClientHello{
		ClientHello: &syncpb.ClientHello{StationIdentifier: "station1", ProtocolVersion: internalsyncv2.ProtocolVersion},
	}}))

	f, err := sessions.clientMainStream.Recv()
	require.NoError(t, err)
	_, ok := f.Body.(*syncpb.StationMessage_ServerHello)
	require.True(t, ok)

	require.NoError(t, sessions.clientMainStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_StationInfo{
		StationInfo: internalsyncv2.StationInfoToPB(common.StationInfoStruct{
			Identifier: "station1",
			Devices:    common.StringMapMap{"dev1": {"t1": "item1"}},
			Cameras:    []string{"cam1"},
		}),
	}}))

	f, err = sessions.clientMainStream.Recv()
	require.NoError(t, err)
	items, ok := f.Body.(*syncpb.StationMessage_ItemsLatest)
	require.True(t, ok)
	require.Equal(t, map[string]int64{"item1": 1234}, items.ItemsLatest.LatestUnixMs)

	f, err = sessions.clientMainStream.Recv()
	require.NoError(t, err)
	status, ok := f.Body.(*syncpb.StationMessage_StatusLatest)
	require.True(t, ok)
	require.Equal(t, int64(42), status.StatusLatest.LatestRowId)

	require.Equal(t, "station1", store.gotIdentifier)
	require.Equal(t, "1.2.3.4:5555", store.gotRemoteAddr)

	infoSyncer.mu.Lock()
	require.Equal(t, 1, infoSyncer.callCount)
	require.Equal(t, stationID, infoSyncer.lastID)
	require.Equal(t, "station1", infoSyncer.lastInfo.Identifier)
	infoSyncer.mu.Unlock()

	_ = sessions.clientSession.Close()
	cancel()
	_ = <-errCh
}

func TestServer_RequestSnapshot_RoundTrip(t *testing.T) {
	stationID := uuid.New()
	store := &fakeStore{stationID: stationID, itemsLatest: map[string]int64{}, latestStatusRowID: 0}
	srv := &Server{
		Store:      store,
		InfoSyncer: &fakeInfoSyncer{},
		Notifier:   &fakeNotifier{},
	}

	sessions := newStationSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(ctx, sessions.serverMainStream, sessions.openServerCommandStream, "1.2.3.4:5555")
	}()
	doHandshake(t, sessions.clientMainStream, []string{"cam1"})

	snapCh := make(chan []byte, 1)
	snapErrCh := make(chan error, 1)
	go func() {
		b, err := srv.RequestSnapshot(stationID, "cam1", 2*time.Second)
		if err != nil {
			snapErrCh <- err
			return
		}
		snapCh <- b
	}()

	cmdStream, err := sessions.acceptClientCommandStream(2 * time.Second)
	require.NoError(t, err)
	defer func() { _ = cmdStream.Close() }()

	require.Equal(t, "cam1", snapshotRequestCameraName(t, cmdStream))
	require.NoError(t, cmdStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
		CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{Data: []byte("abcd")},
	}}))

	select {
	case b := <-snapCh:
		require.Equal(t, []byte("abcd"), b)
	case err := <-snapErrCh:
		t.Fatalf("snapshot error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for snapshot")
	}

	_ = sessions.clientSession.Close()
	cancel()
	_ = <-errCh
}

func TestServer_RequestSnapshot_EmptyResponseError(t *testing.T) {
	stationID := uuid.New()
	store := &fakeStore{stationID: stationID, itemsLatest: map[string]int64{}, latestStatusRowID: 0}
	srv := &Server{
		Store:      store,
		InfoSyncer: &fakeInfoSyncer{},
		Notifier:   &fakeNotifier{},
	}

	sessions := newStationSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(ctx, sessions.serverMainStream, sessions.openServerCommandStream, "1.2.3.4:5555")
	}()
	doHandshake(t, sessions.clientMainStream, []string{"cam1"})

	snapErrCh := make(chan error, 1)
	go func() {
		_, err := srv.RequestSnapshot(stationID, "cam1", 2*time.Second)
		snapErrCh <- err
	}()

	cmdStream, err := sessions.acceptClientCommandStream(2 * time.Second)
	require.NoError(t, err)
	defer func() { _ = cmdStream.Close() }()

	require.Equal(t, "cam1", snapshotRequestCameraName(t, cmdStream))
	require.NoError(t, cmdStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
		CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{},
	}}))

	select {
	case err := <-snapErrCh:
		require.EqualError(t, err, "empty snapshot")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for snapshot error")
	}

	_ = sessions.clientSession.Close()
	cancel()
	_ = <-errCh
}

func TestServer_RequestSnapshot_ConcurrentSubstreams(t *testing.T) {
	stationID := uuid.New()
	store := &fakeStore{stationID: stationID, itemsLatest: map[string]int64{}, latestStatusRowID: 0}
	srv := &Server{
		Store:      store,
		InfoSyncer: &fakeInfoSyncer{},
		Notifier:   &fakeNotifier{},
	}

	sessions := newStationSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(ctx, sessions.serverMainStream, sessions.openServerCommandStream, "1.2.3.4:5555")
	}()
	doHandshake(t, sessions.clientMainStream, []string{"cam1", "cam2"})

	snap1Ch := make(chan []byte, 1)
	snap1ErrCh := make(chan error, 1)
	go func() {
		b, err := srv.RequestSnapshot(stationID, "cam1", 2*time.Second)
		if err != nil {
			snap1ErrCh <- err
			return
		}
		snap1Ch <- b
	}()

	snap2Ch := make(chan []byte, 1)
	snap2ErrCh := make(chan error, 1)
	go func() {
		b, err := srv.RequestSnapshot(stationID, "cam2", 2*time.Second)
		if err != nil {
			snap2ErrCh <- err
			return
		}
		snap2Ch <- b
	}()

	firstStream, err := sessions.acceptClientCommandStream(2 * time.Second)
	require.NoError(t, err)
	defer func() { _ = firstStream.Close() }()

	type secondResult struct {
		stream internalsyncv2.StationMessageStream
		err    error
	}
	secondCh := make(chan secondResult, 1)
	go func() {
		stream, acceptErr := sessions.acceptClientCommandStream(2 * time.Second)
		secondCh <- secondResult{stream: stream, err: acceptErr}
	}()

	var secondStream internalsyncv2.StationMessageStream
	select {
	case r := <-secondCh:
		require.NoError(t, r.err)
		secondStream = r.stream
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected second snapshot command stream before first response")
	}
	defer func() { _ = secondStream.Close() }()

	firstCamera := snapshotRequestCameraName(t, firstStream)
	secondCamera := snapshotRequestCameraName(t, secondStream)

	streamByCamera := map[string]internalsyncv2.StationMessageStream{
		firstCamera:  firstStream,
		secondCamera: secondStream,
	}
	require.Contains(t, streamByCamera, "cam1")
	require.Contains(t, streamByCamera, "cam2")

	require.NoError(t, streamByCamera["cam2"].Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
		CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{Data: []byte("img2")},
	}}))
	select {
	case b := <-snap2Ch:
		require.Equal(t, []byte("img2"), b)
	case err := <-snap2ErrCh:
		t.Fatalf("second snapshot error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting second snapshot result")
	}

	require.NoError(t, streamByCamera["cam1"].Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
		CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{Data: []byte("img1")},
	}}))
	select {
	case b := <-snap1Ch:
		require.Equal(t, []byte("img1"), b)
	case err := <-snap1ErrCh:
		t.Fatalf("first snapshot error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting first snapshot result")
	}

	_ = sessions.clientSession.Close()
	cancel()
	_ = <-errCh
}

func TestServer_StreamStation_DataBatchReplayVsRealtime(t *testing.T) {
	stationID := uuid.New()
	store := &fakeStore{stationID: stationID, itemsLatest: map[string]int64{}, latestStatusRowID: 0}
	infoSyncer := &fakeInfoSyncer{}
	notifier := &fakeNotifier{
		missDataCh:     make(chan common.DataTimeStruct, 1),
		realtimeDataCh: make(chan bool, 2),
	}
	srv := &Server{
		Store:      store,
		InfoSyncer: infoSyncer,
		Notifier:   notifier,
	}

	sessions := newStationSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(ctx, sessions.serverMainStream, sessions.openServerCommandStream, "1.2.3.4:5555")
	}()
	doHandshake(t, sessions.clientMainStream, nil)

	require.NoError(t, sessions.clientMainStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_DataBatch{
		DataBatch: &syncpb.DataBatch{
			Replay: true,
			Points: []*syncpb.DataPoint{
				{ItemName: "item1", Value: 1, UnixMs: 1000, Kind: syncpb.DataKind_DATA_KIND_NORMAL},
			},
		},
	}}))
	select {
	case d := <-notifier.missDataCh:
		require.Equal(t, float64(1), d.Value)
		require.Equal(t, int64(1000), d.Millisecond.ToInt64())
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for miss data notifier")
	}

	require.NoError(t, sessions.clientMainStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_DataBatch{
		DataBatch: &syncpb.DataBatch{
			Replay: false,
			Points: []*syncpb.DataPoint{
				{ItemName: "item1", Value: 2, UnixMs: 2000, Kind: syncpb.DataKind_DATA_KIND_GPIO},
			},
		},
	}}))
	select {
	case gpio := <-notifier.realtimeDataCh:
		require.True(t, gpio)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for realtime data notifier")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	foundNoStatus := false
	for _, c := range store.updateItemStatus {
		if c.item == "item1" && c.status == common.NoStatus {
			foundNoStatus = true
			break
		}
	}
	require.True(t, foundNoStatus, "expected UpdateItemStatus(NoStatus) call for GPIO data")

	_ = sessions.clientSession.Close()
	cancel()
	_ = <-errCh
}
