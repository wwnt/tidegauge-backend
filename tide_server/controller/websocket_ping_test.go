package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// startPingTestServer runs a minimal upgraded WebSocket endpoint that only exercises
// the connection lifecycle: it derives the connection context exactly like the real
// handlers, starts the ping loop with the given schedule, and keeps a read loop running
// (required for pongs to be consumed).
//
// It deliberately avoids the real router, so these tests need neither Postgres nor a
// login token.
func startPingTestServer(t *testing.T, interval, timeout time.Duration) (wsURL string, cancelled <-chan struct{}) {
	t.Helper()

	cancelledCh := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			<-ctx.Done()
			close(cancelledCh)
		}()
		go runWebsocketPing(ctx, cancel, ws, interval, timeout)

		for {
			if _, _, err := ws.Read(ctx); err != nil {
				return
			}
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ping", cancelledCh
}

func dialPingTestConn(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial %s: %v", wsURL, err)
	}
	// Registered after srv.Close, so it runs before it: the handler's read loop must
	// observe the close before httptest.Server.Close waits for the handler.
	t.Cleanup(func() { _ = ws.CloseNow() })

	return ws
}

// TestWebsocketPingKeepsResponsiveConnectionAlive covers the healthy path: a peer that
// drains its socket (and therefore answers pings with pongs) must not be disconnected
// by the keepalive.
func TestWebsocketPingKeepsResponsiveConnectionAlive(t *testing.T) {
	const interval = 50 * time.Millisecond
	wsURL, cancelled := startPingTestServer(t, interval, 200*time.Millisecond)
	ws := dialPingTestConn(t, wsURL)

	readCtx, readCancel := context.WithCancel(context.Background())
	defer readCancel()
	go func() {
		for {
			if _, _, err := ws.Read(readCtx); err != nil {
				return
			}
		}
	}()

	// Several ping cycles must elapse so a false cancellation has time to happen.
	select {
	case <-cancelled:
		t.Fatal("responsive peer was disconnected by the ping keepalive")
	case <-time.After(6 * interval):
	}
}

// TestWebsocketPingCancelsContextOnUnresponsivePeer covers half-open detection: a peer
// that stops reading never answers pings, so the ping loop must cancel the connection
// context (which in turn closes the connection, waking the blocked read loop).
func TestWebsocketPingCancelsContextOnUnresponsivePeer(t *testing.T) {
	wsURL, cancelled := startPingTestServer(t, 50*time.Millisecond, 50*time.Millisecond)
	// No reader on this side: the client never consumes the ping frames, so it never
	// sends a pong back.
	_ = dialPingTestConn(t, wsURL)

	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("unresponsive peer did not cause the ping keepalive to cancel the connection")
	}
}

// TestWebsocketContextCancelledWhenPeerDisconnects verifies the lifecycle wiring rather
// than the keepalive itself: once the peer goes away the handler must return and cancel
// the connection context, so the ping goroutine never outlives the connection. A leaked
// handler would also make httptest.Server.Close (registered in cleanup) hang.
func TestWebsocketContextCancelledWhenPeerDisconnects(t *testing.T) {
	// A one-hour interval keeps the keepalive out of the way: this test is about the
	// read loop noticing the disconnect.
	wsURL, cancelled := startPingTestServer(t, time.Hour, time.Second)
	ws := dialPingTestConn(t, wsURL)

	_ = ws.CloseNow()

	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("connection context was not cancelled after the peer went away")
	}
}
