package syncv2relay

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tide/common"
	"tide/internal/upstreamauth"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

type UpstreamStore interface {
	GetAvailableItems() ([]common.StationItemStruct, error)
	GetStationsFullInfo() ([]db.StationFullInfo, error)
	GetDeviceRecords() ([]db.DeviceRecord, error)
	GetItemStatusLogs(stationID uuid.UUID, afterRowID int64) ([]common.RowIdItemStatusStruct, error)
	GetItemsAllStations() ([]db.Item, error)
	GetDataHistory(stationID uuid.UUID, itemName string, after custype.UnixMs) ([]common.DataTimeStruct, error)
}

type UpstreamAuthDeps struct {
	UserManager auth.UserManager
	Permission  auth.Permission
}

type FrameSource interface {
	Subscribe(ctx context.Context, username string, permissionTopics pubsub.TopicSet) (config <-chan *syncpb.RelayMessage, data <-chan *syncpb.RelayMessage, stop func())
}

type UpstreamServer struct {
	Store  UpstreamStore
	Auth   UpstreamAuthDeps
	Frames FrameSource
	Logger *slog.Logger
}

type UpstreamHandler struct {
	Enabled             func() bool
	Server              *UpstreamServer
	MaxFrameBytes       int64
	UsernameFromRequest func(*http.Request) string
	Logger              *slog.Logger
}

type DownstreamConfig struct {
	UpstreamID int
	BaseURL    string
	Username   string
	Password   string
}

type DownstreamStore interface {
	// Available items.
	UpdateAvailableItems(upstreamID int, items common.UUIDStringsMap) (changed bool, err error)
	RemoveAvailableByUpstreamID(upstreamID int) error

	// Full config helpers.
	GetStationsByUpstreamID(upstreamID int) ([]db.Station, error)
	GetDevices(stationID uuid.UUID) ([]db.Device, error)
	GetItems(stationID uuid.UUID) ([]db.Item, error)

	SyncStation(upstreamID int, station db.Station) (changed bool, err error)
	SyncStationCannotEdit(stationID uuid.UUID, cameras json.RawMessage) (changed bool, err error)
	DelUpstreamStation(upstreamID int, stationID uuid.UUID) (changed bool, err error)

	UpdateStationStatus(stationID uuid.UUID, status common.Status, at time.Time) (changed bool, err error)

	SyncDevice(device db.Device) (changed bool, err error)
	DelDevice(stationID uuid.UUID, deviceName string) (changed bool, err error)

	SyncItem(item db.Item) (changed bool, err error)
	DelItem(stationID uuid.UUID, itemName string) (changed bool, err error)

	SyncDeviceRecord(dr db.DeviceRecord) (changed bool, err error)

	// Miss status / status events.
	LatestStatusLogRowID(stationID uuid.UUID) (int64, error)
	SaveItemStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (inserted bool, err error)
	UpdateAndSaveStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (inserted bool, err error)
	UpdateItemStatus(stationID uuid.UUID, itemName string, status common.Status, at time.Time) (changed bool, err error)

	// Miss data / data events.
	GetAllItems() ([]db.Item, error)
	ItemsLatest(stationID uuid.UUID, itemNames []string) (map[string]int64, error)
	EnsureDataTable(itemName string) error
	SaveDataHistory(stationID uuid.UUID, itemName string, value float64, at time.Time) (inserted bool, err error)
}

type DownstreamNotifier interface {
	PublishConfig(typeStr string, body any)
	PublishStatus(typeStr string, body any)
	BroadcastAvailableChange(items common.UUIDStringsMap)

	PublishMissData(topic common.StationItemStruct, data common.DataTimeStruct)
	PublishData(topic common.StationItemStruct, data common.DataTimeStruct, gpio bool)
}

type DownstreamDeps struct {
	AuthClient *upstreamauth.Client
	Store      DownstreamStore
	Notifier   DownstreamNotifier
	EditLock   sync.Locker

	MaxFrameBytes int64
	Logger        *slog.Logger
}
