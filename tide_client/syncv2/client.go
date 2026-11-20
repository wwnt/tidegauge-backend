package syncv2

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/pkg/pubsub"
)

type GetDataHistoryFn func(item string, startUnixMs, endUnixMs int64) ([]common.DataTimeStruct, error)
type GetItemStatusLogAfterFn func(afterRowID int64) ([]common.RowIdItemStatusStruct, error)
type GetItemStatusLogLatestFn func() (rowID int64, changedAtUnixMs int64, err error)
type SubscribeFn func(*pubsub.Subscriber, pubsub.TopicSet)
type UnsubscribeFn func(*pubsub.Subscriber)
type GetCameraFn func(name string) (snapshotURL, username, password string, ok bool)
type SnapshotFn func(url, username, password string) ([]byte, error)

type Config struct {
	Addr              string
	StationIdentifier string
}

type Deps struct {
	StationInfoFn          func() common.StationInfoStruct
	GetDataHistory         GetDataHistoryFn
	GetItemStatusLogAfter  GetItemStatusLogAfterFn
	GetItemStatusLogLatest GetItemStatusLogLatestFn
	Subscribe              SubscribeFn
	Unsubscribe            UnsubscribeFn
	IngestLock             sync.Locker
	GetCamera              GetCameraFn
	Snapshot               SnapshotFn
	HTTPClient             *http.Client
	Logger                 *slog.Logger
}

type Client struct {
	cfg  Config
	deps Deps

	mainSendMu sync.Mutex
}

func NewClient(cfg Config, deps Deps) (*Client, error) {
	if cfg.Addr == "" {
		return nil, errors.New("empty addr")
	}
	if cfg.StationIdentifier == "" {
		return nil, errors.New("empty station identifier")
	}
	if deps.StationInfoFn == nil {
		return nil, errors.New("station info func is nil")
	}
	if deps.GetDataHistory == nil {
		return nil, errors.New("get data history func is nil")
	}
	if deps.GetItemStatusLogAfter == nil {
		return nil, errors.New("get item status log after func is nil")
	}
	if deps.GetItemStatusLogLatest == nil {
		return nil, errors.New("get item status log latest func is nil")
	}
	if deps.Subscribe == nil {
		return nil, errors.New("subscribe func is nil")
	}
	if deps.Unsubscribe == nil {
		return nil, errors.New("unsubscribe func is nil")
	}
	if deps.IngestLock == nil {
		return nil, errors.New("lock is nil")
	}
	if deps.GetCamera == nil {
		return nil, errors.New("get camera func is nil")
	}
	if deps.Snapshot == nil {
		return nil, errors.New("snapshot func is nil")
	}
	if deps.HTTPClient == nil {
		deps.HTTPClient = &http.Client{}
	}

	return &Client{
		cfg:  cfg,
		deps: deps,
	}, nil
}

func (c *Client) Run(ctx context.Context) error {
	if c == nil {
		return errors.New("nil client")
	}

	stationURL, err := internalsyncv2.StationURL(c.cfg.Addr)
	if err != nil {
		return err
	}

	conn, resp, err := internalsyncv2.DoUpgrade(ctx, c.deps.HTTPClient, stationURL, nil)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// 升级后的连接是 net/http 的 101 响应体：它只实现 io.ReadWriteCloser，不是 net.Conn
	// （此前这里做 net.Conn 断言，对真实服务器必然失败）。yamux 只需要 io.ReadWriteCloser。
	return c.runOnConn(ctx, conn)
}

func (c *Client) logger() *slog.Logger {
	if c.deps.Logger != nil {
		return c.deps.Logger
	}
	return slog.Default()
}
