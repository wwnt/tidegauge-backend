package syncv2station

import (
	"time"

	"tide/common"

	"github.com/google/uuid"
)

type Store interface {
	StationIDByIdentifier(identifier string) (uuid.UUID, error)
	SetStationIP(stationID uuid.UUID, remoteAddr string) error

	ItemsLatest(stationID uuid.UUID, devices common.StringMapMap) (map[string]int64, error)
	LatestStatusLogRowID(stationID uuid.UUID) (int64, error)

	UpdateStationStatus(stationID uuid.UUID, status common.Status, at time.Time) (changed bool, err error)

	SaveRpiStatus(stationID uuid.UUID, cpuTemp float64, at time.Time) error

	UpdateItemStatus(stationID uuid.UUID, itemName string, status common.Status, at time.Time) (changed bool, err error)
	SaveDataHistory(stationID uuid.UUID, itemName string, value float64, at time.Time) (inserted bool, err error)

	SaveItemStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (inserted bool, err error)
	UpdateAndSaveStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (inserted bool, err error)
}

type InfoSyncer interface {
	SyncStationInfo(stationID uuid.UUID, info common.StationInfoStruct) error
}

type Notifier interface {
	PublishMissData(stationItem common.StationItemStruct, data common.DataTimeStruct)
	PublishRealtimeData(stationItem common.StationItemStruct, data common.DataTimeStruct, gpio bool)
	PublishMissItemStatus(status common.FullItemStatusStruct)
	PublishUpdateItemStatus(status common.FullItemStatusStruct)
	PublishStationStatus(status common.StationStatusStruct)
}
