package syncv2relay

import (
	"encoding/json"
	"time"

	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

type DBUpstreamStore struct{}

func (DBUpstreamStore) GetAvailableItems() ([]common.StationItemStruct, error) {
	return db.GetAvailableItems()
}

func (DBUpstreamStore) GetStationsFullInfo() ([]db.StationFullInfo, error) {
	return db.GetStationsFullInfo()
}

func (DBUpstreamStore) GetDeviceRecords() ([]db.DeviceRecord, error) {
	return db.GetDeviceRecords()
}

func (DBUpstreamStore) GetItemStatusLogs(stationID uuid.UUID, afterRowID int64) ([]common.RowIdItemStatusStruct, error) {
	return db.GetItemStatusLogs(stationID, afterRowID)
}

func (DBUpstreamStore) GetItemsAllStations() ([]db.Item, error) {
	return db.GetItems(uuid.Nil)
}

func (DBUpstreamStore) GetDataHistory(stationID uuid.UUID, itemName string, after custype.UnixMs) ([]common.DataTimeStruct, error) {
	return db.GetDataHistory(stationID, itemName, after, 0)
}

type DBDownstreamStore struct{}

func (DBDownstreamStore) UpdateAvailableItems(upstreamID int, items common.UUIDStringsMap) (bool, error) {
	n, err := db.UpdateAvailableItems(upstreamID, items)
	return n > 0, err
}

func (DBDownstreamStore) RemoveAvailableByUpstreamID(upstreamID int) error {
	return db.RemoveAvailableByUpstreamId(upstreamID)
}

func (DBDownstreamStore) GetStationsByUpstreamID(upstreamID int) ([]db.Station, error) {
	return db.GetStationsByUpstreamId(upstreamID)
}

func (DBDownstreamStore) GetDevices(stationID uuid.UUID) ([]db.Device, error) {
	return db.GetDevices(stationID)
}

func (DBDownstreamStore) GetItems(stationID uuid.UUID) ([]db.Item, error) {
	return db.GetItems(stationID)
}

func (DBDownstreamStore) SyncStation(upstreamID int, station db.Station) (bool, error) {
	n, err := db.SyncStation(upstreamID, station)
	return n > 0, err
}

func (DBDownstreamStore) SyncStationCannotEdit(stationID uuid.UUID, cameras json.RawMessage) (bool, error) {
	n, err := db.SyncStationCannotEdit(stationID, cameras)
	return n > 0, err
}

func (DBDownstreamStore) DelUpstreamStation(upstreamID int, stationID uuid.UUID) (bool, error) {
	n, err := db.DelUpstreamStation(upstreamID, stationID)
	return n > 0, err
}

func (DBDownstreamStore) UpdateStationStatus(stationID uuid.UUID, status common.Status, at time.Time) (bool, error) {
	n, err := db.UpdateStationStatus(stationID, status, at)
	return n > 0, err
}

func (DBDownstreamStore) SyncDevice(device db.Device) (bool, error) {
	n, err := db.SyncDevice(device)
	return n > 0, err
}

func (DBDownstreamStore) DelDevice(stationID uuid.UUID, deviceName string) (bool, error) {
	n, err := db.DelDevice(stationID, deviceName)
	return n > 0, err
}

func (DBDownstreamStore) SyncItem(item db.Item) (bool, error) {
	n, err := db.SyncItem(item)
	return n > 0, err
}

func (DBDownstreamStore) DelItem(stationID uuid.UUID, itemName string) (bool, error) {
	n, err := db.DelItem(stationID, itemName)
	return n > 0, err
}

func (DBDownstreamStore) SyncDeviceRecord(dr db.DeviceRecord) (bool, error) {
	n, err := db.SyncDeviceRecord(dr)
	return n > 0, err
}

func (DBDownstreamStore) LatestStatusLogRowID(stationID uuid.UUID) (int64, error) {
	return db.GetLatestStatusLogRowId(stationID)
}

func (DBDownstreamStore) SaveItemStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	n, err := db.SaveItemStatusLog(stationID, rowID, itemName, status, at)
	return n > 0, err
}

func (DBDownstreamStore) UpdateAndSaveStatusLog(stationID uuid.UUID, rowID int64, itemName string, status common.Status, at time.Time) (bool, error) {
	n, err := db.UpdateAndSaveStatusLog(stationID, rowID, itemName, status, at)
	return n > 0, err
}

func (DBDownstreamStore) UpdateItemStatus(stationID uuid.UUID, itemName string, status common.Status, at time.Time) (bool, error) {
	n, err := db.UpdateItemStatus(stationID, itemName, status, at)
	return n > 0, err
}

func (DBDownstreamStore) GetAllItems() ([]db.Item, error) {
	return db.GetItems(uuid.Nil)
}

func (DBDownstreamStore) ItemsLatest(stationID uuid.UUID, itemNames []string) (map[string]int64, error) {
	itemsLatest := make(common.StringMsecMap)
	for _, name := range itemNames {
		itemsLatest[name] = 0
	}
	if err := db.GetItemsLatest(stationID, itemsLatest); err != nil {
		return nil, err
	}
	latest := make(map[string]int64, len(itemsLatest))
	for name, msec := range itemsLatest {
		latest[name] = msec.ToInt64()
	}
	return latest, nil
}

func (DBDownstreamStore) EnsureDataTable(itemName string) error {
	return db.MakeSureTableExist(itemName)
}

func (DBDownstreamStore) SaveDataHistory(stationID uuid.UUID, itemName string, value float64, at time.Time) (bool, error) {
	n, err := db.SaveDataHistory(stationID, itemName, value, at)
	return n > 0, err
}
