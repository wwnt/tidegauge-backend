package controller

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tide/internal/upstreamauth"
	"tide/tide_server/db"
	"tide/tide_server/global"
	syncv2relay "tide/tide_server/syncv2/relay"

	"github.com/hashicorp/yamux"
)

// recvConnections is connections with upstream or tide gauge. Used to get data.
var recvConnections sync.Map

type upstreamSyncState struct {
	config     db.Upstream
	httpClient *upstreamauth.Client
	ctx        context.Context
	cancel     context.CancelFunc
}

func startSync(upstreamCfg db.Upstream) {
	ctx, cancel := context.WithCancel(context.Background())
	httpClient := &http.Client{}
	authClient, authErr := upstreamauth.NewClient(upstreamauth.Config{
		BaseURL:      upstreamCfg.Url,
		Username:     upstreamCfg.Username,
		Password:     upstreamCfg.Password,
		LoginPath:    loginPath,
		LoginTimeout: 10 * time.Second,
		HTTPClient:   httpClient,
	})
	if authErr != nil {
		slog.Error("Failed to init upstream auth client", "upstream_id", upstreamCfg.Id, "url", upstreamCfg.Url, "error", authErr)
	}

	upstream := &upstreamSyncState{
		config:     upstreamCfg,
		httpClient: authClient,
		ctx:        ctx,
		cancel:     cancel,
	}
	recvConnections.Store(upstreamCfg.Id, upstream) // store first

	if global.Config.SyncV2.Enabled {
		fetchUpstreamLoopV2(upstream)
		return
	}

	fetchUpstreamLoop(upstream)
}

func fetchUpstreamLoopV2(upstream *upstreamSyncState) {
	syncv2relay.RunDownstreamWithRetry(
		upstream.ctx,
		relayDownstreamConfig(upstream),
		relayDownstreamDeps(upstream),
		10*time.Second,
		func(err error) {
			if err != nil {
				slog.Debug("v2 下游同步断开，准备重连", "url", upstream.config.Url, "error", err)
			}
			// 断开后清理可用 items (保持现有语义)
			if err := db.RemoveAvailableByUpstreamId(upstream.config.Id); err != nil {
				slog.Error("Failed to remove available items by upstream ID", "upstream_id", upstream.config.Id, "error", err)
			}
		},
	)
}

func fetchUpstreamLoop(upstream *upstreamSyncState) {
	for {
		dialUpstream(upstream)
		select {
		case <-time.After(10 * time.Second):
		case <-upstream.ctx.Done():
			return
		}
	}
}

func dialUpstream(upstream *upstreamSyncState) {
	if upstream.httpClient == nil {
		slog.Debug("Upstream auth client unavailable", "url", upstream.config.Url)
		return
	}

	resp, err := upstream.httpClient.DoWithAuth(upstream.ctx, func(token string) (*http.Request, error) {
		return newSyncUpgradeRequest(upstream.ctx, upstream.config.Url+syncPath, token)
	})
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
	if resp.StatusCode != http.StatusSwitchingProtocols {
		slog.Debug("Unexpected upstream sync status", "url", upstream.config.Url, "status", resp.StatusCode)
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

func newSyncUpgradeRequest(ctx context.Context, syncURL, bearerToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, syncURL, nil)
	if err != nil {
		return nil, err
	}
	// net/http/response.go: func isProtocolSwitchResponse()
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	addAuthorization(req, bearerToken)
	return req, nil
}

func handleSyncClientConn(conn io.ReadWriteCloser, upstream *upstreamSyncState) {
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
