package controller

import (
	"context"
	"github.com/hashicorp/yamux"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"sync"
	"tide/tide_server/db"
	"time"
)

// connections with upstream or tide gauge. Used to get data
var connections sync.Map

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
	connections.Store(upstreamConfig.Id, upstream) // store first

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
	req, err := http.NewRequestWithContext(upstream.ctx, http.MethodPost, upstream.config.Sync, nil)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	resp, err := upstream.httpClient.Do(req)
	if err != nil {
		logger.Debug(err.Error())
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		logger.Debug(http.StatusText(http.StatusUnauthorized))
		return
	}
	logger.Debug("sync client", zap.String("url", upstream.config.Sync))

	conn, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		logger.Error("the resp body does not impl io.Writer")
		return
	}
	handleSyncClientConn(conn, upstream)
}

func handleSyncClientConn(conn io.ReadWriteCloser, upstream *upstreamStorage) {
	session, err := yamux.Client(conn, nil)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	defer func() {
		logger.Debug("sync client closed", zap.String("url", upstream.config.Sync))
		_ = session.Close()

		if err = db.RemoveAllAvailable(upstream.config.Id); err != nil {
			logger.Error(err.Error())
		}
	}()

	stream1, err := session.Open()
	if err != nil {
		logger.Error(err.Error())
		return
	}
	stream2, err := session.Open()
	if err != nil {
		logger.Error(err.Error())
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
				return
			}
			stream4, err := session.Open()
			if err != nil {
				logger.Error(err.Error())
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
			TokenURL: ts.Login,
		},
	}
	return oauth2Config.PasswordCredentialsToken(ts.ctx, ts.Username, ts.Password)
}
