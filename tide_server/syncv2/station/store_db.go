package syncv2station

import (
	"time"

	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

type DBStore struct{}

func (DBStore) StationIDByIdentifier(identifier string) (uuid.UUID, error) {
	return db.GetLocalStationIdByIdentifier(identifier)
}

func (DBStore) SetStationIP(stationID uuid.UUID, remoteAddr string) error {
	return db.EditStationNotSync(stationID, remoteAddr)
}

func (DBStore) ItemsLatest(stationID uuid.UUID, devices common.StringMapMap) (map[string]int64, error) {
	itemsLatest := make(common.StringMsecMap)
	for _, items := range devices {
		for _, itemName := range items {
			itemsLatest[itemName] = 0
		}
	}
	if err := db.GetItemsLatest(stationID, itemsLatest); err != nil {
		return nil, err
	}
	ret := make(map[string]int64, len(itemsLatest))
	for itemName, msec := range itemsLatest {
		ret[itemName] = msec.ToInt64()
	}
	return ret, nil
}

func (DBStore) LatestStatusLogRowID(stationID uuid.UUID) (int64, error) {
	return db.GetLatestStatusLogRowId(stationID)
}

func (DBStore) UpdateStationStatus(stationID uuid.UUID, status common.Status, at time.Time) (bool, error) {
	n, err := db.UpdateStationStatus(stationID, status, at)
	return n > 0, err
}

func (DBStore) SaveRpiStatus(stationID uuid.UUID, cpuTemp float64, at time.Time) error {
	return db.SaveRpiStatus(stationID, cpuTemp, at)
}

func (DBStore) UpdateItemStatus(stationID uuid.UUID, itemName string, status common.Status, at time.Time) (bool, error) {
	n, err := db.UpdateItemStatus(stationID, itemName, status, at)
	return n > 0, err
}

func (DBStore) SaveDataHistory(stationID uuid.UUID, itemName string, value float64, at time.Time) (bool, error) {
	n, err := db.SaveDataHistory(stationID, itemName, value, at)
	return n > 0, err
}

func (DBStore) SaveItemStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	n, err := db.SaveItemStatusLog(stationID, rowID, itemName, status, at)
	return n > 0, err
}

func (DBStore) UpdateAndSaveStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	n, err := db.UpdateAndSaveStatusLog(stationID, rowID, itemName, status, at)
	return n > 0, err
}

var _ = custype.UnixMs(0)
