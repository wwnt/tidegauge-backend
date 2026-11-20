package syncv2station

import (
	"context"
	"testing"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	syncpb "tide/pkg/pb/syncproto"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newLifecycleServer(stationID uuid.UUID) *Server {
	return &Server{
		Store:      &fakeStore{stationID: stationID, itemsLatest: map[string]int64{}},
		InfoSyncer: &fakeInfoSyncer{},
		Notifier:   &fakeNotifier{stationStatusCh: make(chan common.StationStatusStruct, 8)},
	}
}

// 只发 ClientHello 就不再说话的对端不能一直占着站点的登记槽位。
func TestServer_StreamStation_HandshakeTimeoutFreesRegistrySlot(t *testing.T) {
	stationID := uuid.New()
	srv := newLifecycleServer(stationID)
	srv.HandshakeTimeout = 100 * time.Millisecond

	sessions := newStationSessions(t)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(context.Background(), sessions.serverMainStream, sessions.openServerCommandStream, "pipe")
	}()

	require.NoError(t, sessions.clientMainStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ClientHello{
		ClientHello: &syncpb.ClientHello{StationIdentifier: "station1", ProtocolVersion: internalsyncv2.ProtocolVersion},
	}}))
	f, err := sessions.clientMainStream.Recv()
	require.NoError(t, err)
	_, ok := f.Body.(*syncpb.StationMessage_ServerHello)
	require.True(t, ok)
	// 不再发送 StationInfo。

	select {
	case err = <-errCh:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a silent peer kept the station stream open past the handshake timeout")
	}
	_, connected := srv.reg.Load(stationID)
	require.False(t, connected, "the registry slot must be released so the real station can connect")
}

// 握手在时限内完成后，连接不受握手时限影响；Disconnect 能把它断开。
func TestServer_Disconnect_ClosesEstablishedStation(t *testing.T) {
	stationID := uuid.New()
	srv := newLifecycleServer(stationID)
	srv.HandshakeTimeout = 300 * time.Millisecond

	require.False(t, srv.Disconnect(stationID), "disconnecting a station that is not connected is a no-op")

	sessions := newStationSessions(t)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StreamStation(context.Background(), sessions.serverMainStream, sessions.openServerCommandStream, "pipe")
	}()
	doHandshake(t, sessions.clientMainStream, nil)

	// 等过握手时限，确认已建立的连接不会被握手计时器误杀。
	select {
	case err := <-errCh:
		t.Fatalf("established station stream ended unexpectedly: %v", err)
	case <-time.After(600 * time.Millisecond):
	}
	_, connected := srv.reg.Load(stationID)
	require.True(t, connected)

	require.True(t, srv.Disconnect(stationID))
	select {
	case err := <-errCh:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Disconnect did not end the station stream")
	}
	_, connected = srv.reg.Load(stationID)
	require.False(t, connected)
}
