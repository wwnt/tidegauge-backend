package syncv2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

// TestClient_Run_OverHTTPUpgrade 让客户端走完整的 Run 路径：真实的 HTTP 请求、
// 101 升级、yamux，再到握手的第一帧。其余测试都从 runOnConn + net.Pipe 开始，
// 覆盖不到"升级后的连接是 http 的响应体而不是 net.Conn"这一段。
func TestClient_Run_OverHTTPUpgrade(t *testing.T) {
	helloCh := make(chan *syncpb.ClientHello, 1)
	handlerErrCh := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != internalsyncv2.StationPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		conn, err := internalsyncv2.HijackUpgrade(w)
		if err != nil {
			handlerErrCh <- err
			return
		}
		defer func() { _ = conn.Close() }()

		session, err := yamux.Server(conn, nil)
		if err != nil {
			handlerErrCh <- err
			return
		}
		defer func() { _ = session.Close() }()
		mainConn, err := session.Open()
		if err != nil {
			handlerErrCh <- err
			return
		}
		stream := internalsyncv2.NewStationMessageStream(mainConn, internalsyncv2.DefaultMaxFrameBytes)
		frame, err := stream.Recv()
		if err != nil {
			handlerErrCh <- err
			return
		}
		hello, ok := frame.Body.(*syncpb.StationMessage_ClientHello)
		if !ok {
			helloCh <- nil
			return
		}
		helloCh <- hello.ClientHello
		// 收到 hello 即结束：关闭会话让客户端的 Run 返回。
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Config{Addr: srv.URL, StationIdentifier: "station1"},
		Deps{
			StationInfoFn: func() common.StationInfoStruct {
				return common.StationInfoStruct{Identifier: "station1"}
			},
			GetDataHistory:         func(string, int64, int64) ([]common.DataTimeStruct, error) { return nil, nil },
			GetItemStatusLogAfter:  func(int64) ([]common.RowIdItemStatusStruct, error) { return nil, nil },
			GetItemStatusLogLatest: func() (int64, int64, error) { return 0, 0, nil },
			Subscribe:              func(*pubsub.Subscriber, pubsub.TopicSet) {},
			Unsubscribe:            func(*pubsub.Subscriber) {},
			IngestLock:             &sync.Mutex{},
			GetCamera:              func(string) (string, string, string, bool) { return "", "", "", false },
			Snapshot:               func(string, string, string) ([]byte, error) { return nil, nil },
		},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	runErrCh := make(chan error, 1)
	go func() { runErrCh <- c.Run(ctx) }()

	select {
	case hello := <-helloCh:
		require.NotNil(t, hello, "first frame must be client_hello")
		require.Equal(t, "station1", hello.StationIdentifier)
		require.Equal(t, internalsyncv2.ProtocolVersion, hello.ProtocolVersion)
	case err := <-handlerErrCh:
		t.Fatalf("server side failed before receiving client_hello: %v", err)
	case err := <-runErrCh:
		t.Fatalf("Run returned before the server saw client_hello: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client_hello over a real HTTP upgrade")
	}

	select {
	case <-runErrCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the server closed the session")
	}
}
