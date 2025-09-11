package controller

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tide/tide_server/db"

	"github.com/hashicorp/yamux"
	"golang.org/x/oauth2"
)

// recvConnections is connections with upstream or tide gauge. Used to get data.
var recvConnections sync.Map

type upstreamStorage struct {
	config     db.Upstream
	httpClient *http.Client
	ctx        context.Context
	cancelF    context.CancelFunc
}

func startSync(upstreamConfig db.Upstream) {
	ctx, cancel := context.WithCancel(context.Background())
	httpClient := oauth2.NewClient(ctx, &tokenSrc{ctx, upstreamConfig})
	upstream := &upstreamStorage{
		config:     upstreamConfig,
		httpClient: httpClient,
		ctx:        ctx,
		cancelF:    cancel,
	}
	recvConnections.Store(upstreamConfig.Id, upstream) // store first

	fetchUpstreamLoop(upstream)
}

func fetchUpstreamLoop(upstream *upstreamStorage) {
	for {
		dialUpstream(upstream)
		select {
		case <-time.After(10 * time.Second):
		case <-upstream.ctx.Done():
			return
		}
	}
}

func dialUpstream(upstream *upstreamStorage) {
	req, err := http.NewRequestWithContext(upstream.ctx, http.MethodPost, upstream.config.Url+syncPath, nil)
	if err != nil {
		slog.Error("Failed to create sync request", "url", upstream.config.Url, "error", err)
		return
	}
	// net/http/response.go: func isProtocolSwitchResponse()
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	resp, err := upstream.httpClient.Do(req)
	if err != nil {
		slog.Debug("Upstream connection failed", "url", upstream.config.Url, "error", err)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		slog.Debug("Upstream authentication failed", "url", upstream.config.Url, "status", "unauthorized")
		return
	}
	slog.Debug("Sync client connected", "url", upstream.config.Url)

	conn, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		slog.Error("Response body does not implement io.ReadWriteCloser", "url", upstream.config.Url)
		return
	}
	handleSyncClientConn(conn, upstream)
}

func handleSyncClientConn(conn io.ReadWriteCloser, upstream *upstreamStorage) {
	cnf := yamux.DefaultConfig()
	cnf.EnableKeepAlive = false
	cnf.ConnectionWriteTimeout = 120 * time.Second
	session, err := yamux.Client(conn, cnf)
	if err != nil {
		slog.Error("Failed to create yamux client session", "url", upstream.config.Url, "error", err)
		return
	}
	defer func() {
		slog.Debug("Sync client connection closed", "url", upstream.config.Url)
		_ = session.Close()
		// when the connection is closed, the upstream will be removed from the map
		if err = db.RemoveAvailableByUpstreamId(upstream.config.Id); err != nil {
			slog.Error("Failed to remove available items by upstream ID", "upstream_id", upstream.config.Id, "error", err)
		}
	}()

	stream1, err := session.Open()
	if err != nil {
		slog.Error("Failed to open stream1", "url", upstream.config.Url, "error", err)
		return
	}
	stream2, err := session.Open()
	if err != nil {
		slog.Error("Failed to open stream2", "url", upstream.config.Url, "error", err)
		return
	}

	if !fullSyncConfigClient(stream2, upstream) {
		return
	}
	_ = stream2.Close()

	go func() {
		defer func() { _ = session.Close() }()
		for {
			stream3, err := session.Open()
			if err != nil {
				slog.Error("Failed to open stream3", "url", upstream.config.Url, "error", err)
				return
			}
			stream4, err := session.Open()
			if err != nil {
				slog.Error("Failed to open stream4", "url", upstream.config.Url, "error", err)
				return
			}
			if !fillMissDataClient(stream4) {
				return
			}
			_ = stream4.Close()

			syncDataClient(stream3)
			_ = stream3.Close()
		}
	}()

	incrementSyncConfigClient(stream1, upstream)
}

type tokenSrc struct {
	ctx context.Context
	db.Upstream
}

func (ts *tokenSrc) Token() (*oauth2.Token, error) {
	oauth2Config := &oauth2.Config{
		Endpoint: oauth2.Endpoint{
			TokenURL: ts.Url + loginPath,
		},
	}
	return oauth2Config.PasswordCredentialsToken(ts.ctx, ts.Username, ts.Password)
}
