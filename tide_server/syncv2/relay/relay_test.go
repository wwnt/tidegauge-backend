package syncv2relay

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/internal/upstreamauth"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ---- UpstreamServer tests ----

type fakeUserManager struct {
	user auth.User
}

func (m fakeUserManager) CheckUserPwd(username, password string) bool   { return false }
func (m fakeUserManager) Login(r *http.Request, w http.ResponseWriter)  {}
func (m fakeUserManager) Logout(r *http.Request, w http.ResponseWriter) {}
func (m fakeUserManager) GetLoginUser(r *http.Request) (string, error)  { return "", nil }
func (m fakeUserManager) AddUser(user auth.User) error                  { return nil }
func (m fakeUserManager) DelUser(username string) error                 { return nil }
func (m fakeUserManager) EditUserBaseInfo(user auth.UserBaseInfo) error { return nil }
func (m fakeUserManager) EditUser(user auth.User) error                 { return nil }
func (m fakeUserManager) ListUsers(condition int, role int) ([]auth.User, error) {
	return nil, nil
}
func (m fakeUserManager) GetUser(username string) (auth.User, error) { return m.user, nil }

type fakePermission struct {
	perms common.UUIDStringsMap
}

func (p fakePermission) CheckPermission(username string, stationId uuid.UUID, itemName string) bool {
	return true
}
func (p fakePermission) GetPermissions(username string) (map[uuid.UUID][]string, error) {
	return p.perms, nil
}
func (p fakePermission) EditPermission(username string, scopes map[uuid.UUID][]string) error {
	return nil
}
func (p fakePermission) CheckCameraStatusPermission(username string, stationId uuid.UUID, name string) bool {
	return true
}
func (p fakePermission) GetCameraStatusPermissions(username string) (map[uuid.UUID][]string, error) {
	return nil, nil
}
func (p fakePermission) EditCameraStatusPermission(username string, scopes map[uuid.UUID][]string) error {
	return nil
}

type fakeUpstreamStore struct {
	avail []common.StationItemStruct

	mu               sync.Mutex
	dataHistoryCalls []struct {
		stationID uuid.UUID
		itemName  string
		after     custype.UnixMs
	}
	dataHistoryByItem map[string][]common.DataTimeStruct
}

func (s *fakeUpstreamStore) GetAvailableItems() ([]common.StationItemStruct, error) {
	return s.avail, nil
}
func (s *fakeUpstreamStore) GetStationsFullInfo() ([]db.StationFullInfo, error) { return nil, nil }
func (s *fakeUpstreamStore) GetDeviceRecords() ([]db.DeviceRecord, error)       { return nil, nil }
func (s *fakeUpstreamStore) GetItemStatusLogs(stationID uuid.UUID, afterRowID int64) ([]common.RowIdItemStatusStruct, error) {
	return nil, nil
}
func (s *fakeUpstreamStore) GetItemsAllStations() ([]db.Item, error) { return nil, nil }
func (s *fakeUpstreamStore) GetDataHistory(stationID uuid.UUID, itemName string, after custype.UnixMs) ([]common.DataTimeStruct, error) {
	s.mu.Lock()
	s.dataHistoryCalls = append(s.dataHistoryCalls, struct {
		stationID uuid.UUID
		itemName  string
		after     custype.UnixMs
	}{stationID: stationID, itemName: itemName, after: after})
	s.mu.Unlock()

	return s.dataHistoryByItem[itemName], nil
}

type fakeFrameSource struct {
	configCh chan *syncpb.RelayMessage
	dataCh   chan *syncpb.RelayMessage
}

func (fs fakeFrameSource) Subscribe(ctx context.Context, username string, permissionTopics pubsub.TopicSet) (<-chan *syncpb.RelayMessage, <-chan *syncpb.RelayMessage, func()) {
	stop := func() {}
	return fs.configCh, fs.dataCh, stop
}

func TestUpstreamServer_StreamSync_HandshakeFilterMissDataAndIncremental(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stationAllowed := uuid.New()
	stationDenied := uuid.New()

	store := &fakeUpstreamStore{
		avail: []common.StationItemStruct{
			{StationId: stationAllowed, ItemName: "item_allowed"},
			{StationId: stationDenied, ItemName: "item_denied"},
		},
		dataHistoryByItem: map[string][]common.DataTimeStruct{
			"item_allowed": {{Value: 1, Millisecond: custype.UnixMs(1100)}},
		},
	}

	fs := fakeFrameSource{
		configCh: make(chan *syncpb.RelayMessage, 1),
		dataCh:   make(chan *syncpb.RelayMessage, 1),
	}

	srv := &UpstreamServer{
		Store: store,
		Auth: UpstreamAuthDeps{
			UserManager: fakeUserManager{user: auth.User{UserAuthority: auth.UserAuthority{Role: auth.NormalUser}}},
			Permission:  fakePermission{perms: common.UUIDStringsMap{stationAllowed: {"item_allowed"}}},
		},
		Frames: fs,
	}

	downConn, upConn := net.Pipe()
	t.Cleanup(func() { _ = downConn.Close() })
	t.Cleanup(func() { _ = upConn.Close() })

	downStream := internalsyncv2.NewRelayMessageStream(downConn, internalsyncv2.DefaultMaxFrameBytes)
	upStream := internalsyncv2.NewRelayMessageStream(upConn, internalsyncv2.DefaultMaxFrameBytes)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.StreamRelay(ctx, upStream, "user1") }()

	// RelayDownstreamHello
	require.NoError(t, downStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_DownstreamHello{
		DownstreamHello: &syncpb.RelayDownstreamHello{Username: "user1", ProtocolVersion: internalsyncv2.ProtocolVersion},
	}}))

	// RelayUpstreamHello
	f, err := downStream.Recv()
	require.NoError(t, err)
	_, ok := f.Body.(*syncpb.RelayMessage_UpstreamHello)
	require.True(t, ok)

	// RelayAvailableItems should be filtered by permission topics.
	f, err = downStream.Recv()
	require.NoError(t, err)
	avail, ok := f.Body.(*syncpb.RelayMessage_AvailableItems)
	require.True(t, ok)
	require.Contains(t, avail.AvailableItems.Stations, stationAllowed.String())
	require.NotContains(t, avail.AvailableItems.Stations, stationDenied.String())
	require.Equal(t, []string{"item_allowed"}, avail.AvailableItems.Stations[stationAllowed.String()].ItemNames)

	// Full config batch (can be empty; semantics: unfiltered).
	f, err = downStream.Recv()
	require.NoError(t, err)
	cfg, ok := f.Body.(*syncpb.RelayMessage_ConfigBatch)
	require.True(t, ok)
	require.True(t, cfg.ConfigBatch.FullSync)

	// Miss status: send latest row IDs (empty is fine).
	require.NoError(t, downStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_StationsStatusLatest{
		StationsStatusLatest: &syncpb.RelayStatusLatest{Stations: map[string]int64{}},
	}}))
	f, err = downStream.Recv()
	require.NoError(t, err)
	_, ok = f.Body.(*syncpb.RelayMessage_ConfigBatch)
	require.True(t, ok)

	// Miss data: request with afterMs>0 only for allowed item.
	require.NoError(t, downStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_StationsItemsLatest{
		StationsItemsLatest: &syncpb.RelayItemsLatest{Stations: map[string]*syncpb.ItemsLatest{
			stationAllowed.String(): {LatestUnixMs: map[string]int64{"item_allowed": 1000, "item_denied": 1000}},
		}},
	}}))

	f, err = downStream.Recv()
	require.NoError(t, err)
	dbatch, ok := f.Body.(*syncpb.RelayMessage_DataBatch)
	require.True(t, ok)
	require.Equal(t, stationAllowed.String(), dbatch.DataBatch.StationId)
	require.Equal(t, MsgMissData, dbatch.DataBatch.DataType)
	require.Len(t, dbatch.DataBatch.Points, 1)
	require.Equal(t, "item_allowed", dbatch.DataBatch.Points[0].ItemName)
	require.Equal(t, int64(1100), dbatch.DataBatch.Points[0].UnixMs)

	// Termination empty batch.
	f, err = downStream.Recv()
	require.NoError(t, err)
	dbatch, ok = f.Body.(*syncpb.RelayMessage_DataBatch)
	require.True(t, ok)
	require.Empty(t, dbatch.DataBatch.StationId)
	require.Empty(t, dbatch.DataBatch.Points)

	store.mu.Lock()
	require.Len(t, store.dataHistoryCalls, 1)
	require.Equal(t, "item_allowed", store.dataHistoryCalls[0].itemName)
	store.mu.Unlock()

	// Incremental frames should be forwarded.
	fs.configCh <- &syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
		ConfigBatch: &syncpb.RelayConfigBatch{
			FullSync: false,
			Events: []*syncpb.RelayConfigEvent{{
				Type:    MsgUpdateAvailable,
				Payload: []byte(`{}`),
			}},
		},
	}}
	fs.dataCh <- &syncpb.RelayMessage{Body: &syncpb.RelayMessage_DataBatch{
		DataBatch: &syncpb.RelayDataBatch{
			StationId: stationAllowed.String(),
			DataType:  MsgData,
			Points: []*syncpb.DataPoint{
				{ItemName: "item_allowed", Value: 9, UnixMs: 2000, Kind: syncpb.DataKind_DATA_KIND_NORMAL},
			},
		},
	}}

	gotConfig := false
	gotData := false
	for i := 0; i < 2; i++ {
		f, err = downStream.Recv()
		require.NoError(t, err)
		switch f.Body.(type) {
		case *syncpb.RelayMessage_ConfigBatch:
			gotConfig = true
		case *syncpb.RelayMessage_DataBatch:
			gotData = true
		default:
			t.Fatalf("unexpected incremental frame: %T", f.Body)
		}
	}
	require.True(t, gotConfig)
	require.True(t, gotData)

	cancel()
	_ = <-errCh
}

// ---- Downstream tests ----

type pipeRoundTripper struct {
	conn      ioReadWriteCloser
	username  string
	password  string
	token     string
	authMu    sync.Mutex
	lastAuthz string
}

type ioReadWriteCloser interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

func (rt *pipeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/login":
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		if values.Get("username") != rt.username || values.Get("password") != rt.password {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"` + rt.token + `"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	case internalsyncv2.RelayPath:
		rt.authMu.Lock()
		rt.lastAuthz = req.Header.Get("Authorization")
		rt.authMu.Unlock()

		if req.Header.Get("Authorization") != "Bearer "+rt.token {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Body: rt.conn.(interface {
				Read([]byte) (int, error)
				Close() error
			}),
			Header:  make(http.Header),
			Request: req,
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
}

type fakeDownstreamStore struct {
	mu sync.Mutex

	updateAvailableCalls int
	updateStationCalls   int
	saveDataCalls        int
	ensureTableCalls     int
}

func (s *fakeDownstreamStore) UpdateAvailableItems(upstreamID int, items common.UUIDStringsMap) (bool, error) {
	s.mu.Lock()
	s.updateAvailableCalls++
	s.mu.Unlock()
	return true, nil
}
func (s *fakeDownstreamStore) RemoveAvailableByUpstreamID(upstreamID int) error { return nil }
func (s *fakeDownstreamStore) GetStationsByUpstreamID(upstreamID int) ([]db.Station, error) {
	return nil, nil
}
func (s *fakeDownstreamStore) GetDevices(stationID uuid.UUID) ([]db.Device, error) { return nil, nil }
func (s *fakeDownstreamStore) GetItems(stationID uuid.UUID) ([]db.Item, error)     { return nil, nil }
func (s *fakeDownstreamStore) SyncStation(upstreamID int, station db.Station) (bool, error) {
	return false, nil
}
func (s *fakeDownstreamStore) SyncStationCannotEdit(stationID uuid.UUID, cameras json.RawMessage) (bool, error) {
	return false, nil
}
func (s *fakeDownstreamStore) DelUpstreamStation(upstreamID int, stationID uuid.UUID) (bool, error) {
	return false, nil
}
func (s *fakeDownstreamStore) UpdateStationStatus(stationID uuid.UUID, status common.Status, at time.Time) (bool, error) {
	s.mu.Lock()
	s.updateStationCalls++
	s.mu.Unlock()
	return true, nil
}
func (s *fakeDownstreamStore) SyncDevice(device db.Device) (bool, error) { return false, nil }
func (s *fakeDownstreamStore) DelDevice(stationID uuid.UUID, deviceName string) (bool, error) {
	return false, nil
}
func (s *fakeDownstreamStore) SyncItem(item db.Item) (bool, error) { return false, nil }
func (s *fakeDownstreamStore) DelItem(stationID uuid.UUID, itemName string) (bool, error) {
	return false, nil
}
func (s *fakeDownstreamStore) SyncDeviceRecord(dr db.DeviceRecord) (bool, error) { return false, nil }
func (s *fakeDownstreamStore) LatestStatusLogRowID(stationID uuid.UUID) (int64, error) {
	return 0, nil
}
func (s *fakeDownstreamStore) SaveItemStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	return false, nil
}
func (s *fakeDownstreamStore) UpdateAndSaveStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	return true, nil
}
func (s *fakeDownstreamStore) UpdateItemStatus(stationID uuid.UUID, itemName string, status common.Status, at time.Time) (bool, error) {
	return true, nil
}
func (s *fakeDownstreamStore) GetAllItems() ([]db.Item, error) { return nil, nil }
func (s *fakeDownstreamStore) ItemsLatest(stationID uuid.UUID, itemNames []string) (map[string]int64, error) {
	return map[string]int64{}, nil
}
func (s *fakeDownstreamStore) EnsureDataTable(itemName string) error {
	s.mu.Lock()
	s.ensureTableCalls++
	s.mu.Unlock()
	return nil
}
func (s *fakeDownstreamStore) SaveDataHistory(stationID uuid.UUID, itemName string, value float64, at time.Time) (bool, error) {
	s.mu.Lock()
	s.saveDataCalls++
	s.mu.Unlock()
	return true, nil
}

type fakeDownstreamNotifier struct {
	mu sync.Mutex

	broadcastAvailCalls int
	publishStatusCalls  int
	publishDataCalls    int
}

func (n *fakeDownstreamNotifier) PublishConfig(typeStr string, body any) {}
func (n *fakeDownstreamNotifier) PublishStatus(typeStr string, body any) {
	n.mu.Lock()
	n.publishStatusCalls++
	n.mu.Unlock()
}
func (n *fakeDownstreamNotifier) BroadcastAvailableChange(items common.UUIDStringsMap) {
	n.mu.Lock()
	n.broadcastAvailCalls++
	n.mu.Unlock()
}
func (n *fakeDownstreamNotifier) PublishMissData(topic common.StationItemStruct, data common.DataTimeStruct) {
}
func (n *fakeDownstreamNotifier) PublishData(topic common.StationItemStruct, data common.DataTimeStruct, gpio bool) {
	n.mu.Lock()
	n.publishDataCalls++
	n.mu.Unlock()
}

func TestRunDownstream_AppliesIncrementalFrames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	downConn, upConn := net.Pipe()
	t.Cleanup(func() { _ = downConn.Close() })
	t.Cleanup(func() { _ = upConn.Close() })

	rt := &pipeRoundTripper{
		conn:     downConn,
		username: "user1",
		password: "pass1",
		token:    "token-user1",
	}
	httpClient := &http.Client{Transport: rt}
	authClient, err := upstreamauth.NewClient(upstreamauth.Config{
		BaseURL:    "http://example.invalid",
		Username:   "user1",
		Password:   "pass1",
		HTTPClient: httpClient,
	})
	require.NoError(t, err)

	store := &fakeDownstreamStore{}
	notifier := &fakeDownstreamNotifier{}
	deps := DownstreamDeps{
		AuthClient: authClient,
		Store:      store,
		Notifier:   notifier,
		EditLock:   &sync.Mutex{},
	}
	cfg := DownstreamConfig{
		UpstreamID: 1,
		BaseURL:    "http://example.invalid",
		Username:   "user1",
		Password:   "pass1",
	}

	// Upstream simulation on the other end of the pipe.
	upStream := internalsyncv2.NewRelayMessageStream(upConn, internalsyncv2.DefaultMaxFrameBytes)

	upDone := make(chan struct{})
	go func() {
		defer close(upDone)

		// RelayDownstreamHello
		f, err := upStream.Recv()
		require.NoError(t, err)
		_, ok := f.Body.(*syncpb.RelayMessage_DownstreamHello)
		require.True(t, ok)

		// RelayUpstreamHello
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_UpstreamHello{
			UpstreamHello: &syncpb.RelayUpstreamHello{ServerVersion: "test"},
		}}))

		// RelayAvailableItems
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_AvailableItems{
			AvailableItems: &syncpb.RelayAvailableItems{Stations: map[string]*syncpb.RelayAvailableItemList{}},
		}}))

		// Full config (empty)
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
			ConfigBatch: &syncpb.RelayConfigBatch{FullSync: true},
		}}))

		// Miss status request from downstream -> respond empty.
		f, err = upStream.Recv()
		require.NoError(t, err)
		_, ok = f.Body.(*syncpb.RelayMessage_StationsStatusLatest)
		require.True(t, ok)
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
			ConfigBatch: &syncpb.RelayConfigBatch{FullSync: false},
		}}))

		// Miss data request -> respond termination.
		f, err = upStream.Recv()
		require.NoError(t, err)
		_, ok = f.Body.(*syncpb.RelayMessage_StationsItemsLatest)
		require.True(t, ok)
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_DataBatch{
			DataBatch: &syncpb.RelayDataBatch{},
		}}))

		// Incremental frames.
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
			ConfigBatch: &syncpb.RelayConfigBatch{
				FullSync: false,
				Events: []*syncpb.RelayConfigEvent{{
					Type:    MsgUpdateAvailable,
					Payload: []byte(`{}`),
				}},
			},
		}}))
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_StatusEvent{
			StatusEvent: &syncpb.RelayStatusEvent{
				StationId:       uuid.NewString(),
				Identifier:      "st",
				Status:          common.Normal,
				ChangedAtUnixMs: 123,
			},
		}}))
		require.NoError(t, upStream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_DataBatch{
			DataBatch: &syncpb.RelayDataBatch{
				StationId: uuid.NewString(),
				DataType:  MsgData,
				Points: []*syncpb.DataPoint{
					{ItemName: "i1", Value: 1, UnixMs: 1000, Kind: syncpb.DataKind_DATA_KIND_NORMAL},
				},
			},
		}}))

		<-ctx.Done()
	}()

	downErrCh := make(chan error, 1)
	go func() { downErrCh <- RunDownstream(ctx, cfg, deps) }()

	// Give incremental frames time to apply.
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Connection teardown error is not meaningful for protocol validation.
	_ = <-downErrCh

	store.mu.Lock()
	require.GreaterOrEqual(t, store.updateAvailableCalls, 2) // handshake + incremental update_available
	require.GreaterOrEqual(t, store.updateStationCalls, 1)
	require.GreaterOrEqual(t, store.ensureTableCalls, 1)
	require.GreaterOrEqual(t, store.saveDataCalls, 1)
	store.mu.Unlock()

	notifier.mu.Lock()
	require.GreaterOrEqual(t, notifier.broadcastAvailCalls, 1)
	require.GreaterOrEqual(t, notifier.publishStatusCalls, 1)
	require.GreaterOrEqual(t, notifier.publishDataCalls, 1)
	notifier.mu.Unlock()

	rt.authMu.Lock()
	require.Equal(t, "Bearer token-user1", rt.lastAuthz)
	rt.authMu.Unlock()

	<-upDone
}
