package db

import (
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"strconv"
	"tide/common"
	"tide/pkg/custype"
)

type Item struct {
	StationId       uuid.UUID               `json:"station_id" binding:"required"`
	Name            string                  `json:"name" binding:"max=20"`
	Type            string                  `json:"type"`
	DeviceName      string                  `json:"device_name"`
	Status          common.Status           `json:"status"`
	StatusChangedAt custype.TimeMillisecond `json:"status_changed_at"`
	Available       bool                    `json:"available"`
}

func GetItems(stationId uuid.UUID) ([]Item, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if stationId == uuid.Nil {
		rows, err = TideDB.Query(`select station_id, name, type ,device_name, status, status_changed_at, available!='' from items`)
	} else {
		rows, err = TideDB.Query(`select station_id, name, type ,device_name, status, status_changed_at, available!='' from items where station_id=$1`, stationId)
	}
	if err != nil {
		return nil, err
	}
	var (
		i     Item
		items []Item
	)
	for rows.Next() {
		err = rows.Scan(&i.StationId, &i.Name, &i.Type, &i.DeviceName, &i.Status, &i.StatusChangedAt, &i.Available)
		if err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return items, err
}

func MakeSureTableExist(name string) (err error) {
	if common.ContainsIllegalCharacter(name) {
		return errors.New("Table name contains illegal characters: " + name)
	}
	var n int
	err = TideDB.QueryRow("select count(to_regclass($1))", name).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = TideDB.Exec(`create table ` + name + `
(
    station_id uuid             not null,
    value      double precision not null,
    timestamp  timestamptz      not null
);
create unique index on ` + name + ` (station_id, timestamp);`)
		return err
	}
	return nil
}

func EditItem(i Item) (err error) {
	if err = MakeSureTableExist(i.Name); err != nil {
		return err
	}
	tx, err := TideDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`insert into items(station_id,name,type,device_name,available) values ($1,$2,$3,$4,hstore('0', NULL)) on conflict (station_id,name) do update
set type=excluded.type, device_name=excluded.device_name`, i.StationId, i.Name, i.Type, i.DeviceName)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func SyncItem(i Item) (int64, error) {
	res, err := TideDB.Exec(`insert into items(station_id,name,type,device_name) values ($1,$2,$3,$4) on conflict (station_id,name) do update
set type=excluded.type, device_name=excluded.device_name where items.type!=$3 or items.device_name!=$4`, i.StationId, i.Name, i.Type, i.DeviceName)
	return checkResult(res, err)
}

func DelItem(stationId uuid.UUID, name string) (int64, error) {
	res, err := TideDB.Exec(`delete from items where station_id=$1 and name=$2`, stationId, name)
	return checkResult(res, err)
}

func GetAvailableItems() ([]common.StationItemStruct, error) {
	rows, err := TideDB.Query(`select station_id, name from items where available!=''`)
	if err != nil {
		return nil, err
	}
	ss, err := scanStationItem(rows)
	if err != nil {
		return nil, err
	}
	return ss, err
}

func scanStationItem(rows *sql.Rows) ([]common.StationItemStruct, error) {
	var (
		err error
		s   common.StationItemStruct
		ss  []common.StationItemStruct
	)
	for rows.Next() {
		err = rows.Scan(&s.StationId, &s.ItemName)
		if err != nil {
			return ss, err
		}
		ss = append(ss, s)
	}
	return ss, rows.Err()
}

func UpdateAvailableItems(upstreamId int, newAvail common.UUIDStringsMap) (int64, error) {
	var n int64
	var err error
	tx, err := TideDB.Begin()
	if err != nil {
		return n, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	rows, err := tx.Query(`select station_id, name from items where available ? $1 and 
station_id in (select station_id from upstream_stations where upstream_id=$2)`, strconv.Itoa(upstreamId), upstreamId)
	if err != nil {
		return n, err
	}
	var (
		s   common.StationItemStruct
		old = make(map[common.StationItemStruct]struct{})
	)
	for rows.Next() {
		err = rows.Scan(&s.StationId, &s.ItemName)
		if err != nil {
			return n, err
		}
		old[s] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return n, err
	}
	for stationId, items := range newAvail {
		for _, itemName := range items {
			key := common.StationItemStruct{StationId: stationId, ItemName: itemName}
			if _, ok := old[key]; ok {
				delete(old, key)
			} else {
				if err = addAvailableTx(tx, stationId, itemName, upstreamId); err != nil {
					return n, err
				}
				n++
			}
		}
	}
	for s := range old {
		if err = removeAvailableTx(tx, s.StationId, s.ItemName, upstreamId); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

func addAvailableTx(tx *sql.Tx, stationId uuid.UUID, itemName string, upstreamId int) error {
	_, err := tx.Exec(`update items set available = items.available||hstore($3, NULL) where station_id=$1 and name=$2`, stationId, itemName, strconv.Itoa(upstreamId))
	return err
}

func removeAvailableTx(tx *sql.Tx, stationId uuid.UUID, itemName string, upstreamId int) error {
	_, err := tx.Exec(`update items set available = delete(available,$3) where station_id=$1 and name=$2`, stationId, itemName, strconv.Itoa(upstreamId))
	return err
}

func RemoveAllAvailable(upstreamId int) error {
	tx, err := TideDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`update items set available = delete(available,$1::text) where station_id in
(select station_id from upstream_stations where upstream_id=$1)`, upstreamId)
	if err != nil {
		return err
	}
	return tx.Commit()
}

//func AddAvailable(upstreamId int, stationId uuid.UUID, itemName string) error {
//	tx, err := TideDB.Begin()
//	if err != nil {
//		return err
//	}
//	defer func() {
//		_ = tx.Rollback()
//	}()
//	_, err = tx.Exec(`insert into items(station_id,name,available) values ($1, $2, hstore($3, NULL))
//on conflict(station_id,name) do update set available = items.available||hstore($3, NULL)`,
//		stationId, itemName, strconv.Itoa(upstreamId))
//	if err != nil {
//		return err
//	}
//	return tx.Commit()
//}
