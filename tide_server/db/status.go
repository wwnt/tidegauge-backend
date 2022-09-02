package db

import (
	"database/sql"
	"github.com/google/uuid"
	"tide/common"
	"time"
)

func UpdateStationStatus(id uuid.UUID, status common.Status, changedAt time.Time) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec("update stations set status=$2, status_changed_at=$3 where id=$1 and (status!=$2 or status_changed_at!=$3)", id, status, changedAt)
	if err != nil {
		return 0, err
	}
	err = tx.Commit()
	return checkResult(res, err)
}

func UpdateItemStatus(stationId uuid.UUID, itemName string, status common.Status, changeAt time.Time) (int64, error) {
	res, err := TideDB.Exec("update items set status=$3, status_changed_at=$4 where station_id=$1 and name=$2 and (status!=$3 or status_changed_at!=$4)", stationId, itemName, status, changeAt)
	return checkResult(res, err)
}

func GetLatestStatusLogRowId(stationId uuid.UUID) (int64, error) {
	var id sql.NullInt64
	err := TideDB.QueryRow(`select max(row_id) from item_status_log where station_id=$1`, stationId).Scan(&id)
	return id.Int64, err
}

func SaveItemStatusLog(stationId uuid.UUID, rowId int64, itemName string, status string, changedAt time.Time) (int64, error) {
	res, err := TideDB.Exec(`insert into item_status_log(station_id, row_id, item_name, status, changed_at) VALUES ($1,$2,$3,$4,$5) on conflict do nothing`,
		stationId, rowId, itemName, status, changedAt)
	return checkResult(res, err)
}

func UpdateAndSaveStatusLog(stationId uuid.UUID, rowId int64, itemName string, status string, changedAt time.Time) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	n, err := checkResult(
		tx.Exec(`insert into item_status_log(station_id, row_id, item_name, status, changed_at) VALUES ($1,$2,$3,$4,$5) on conflict do nothing`,
			stationId, rowId, itemName, status, changedAt),
	)
	if err != nil {
		return n, err
	}
	if n > 0 {
		_, err = tx.Exec(`update items set status=$3, status_changed_at=$4 where station_id=$1 and name=$2 and (status!=$3 or status_changed_at!=$4)`,
			stationId, itemName, status, changedAt)
		if err != nil {
			return n, err
		}
	}
	return n, tx.Commit()
}

type pagedItemStatusLogStruct struct {
	Total int                                `json:"total"`
	Data  []common.StationIdItemStatusStruct `json:"data"`
}

func PagedItemStatusLogs(pageNum, pageSize uint) (interface{}, error) {
	var ds pagedItemStatusLogStruct
	err := TideDB.QueryRow(`select count(*) from item_status_log`).Scan(&ds.Total)
	if err != nil {
		return nil, err
	}
	rows, err := TideDB.Query("select station_id, item_name, status, changed_at from item_status_log limit $1 offset $2", pageSize, (pageNum-1)*pageSize)
	if err != nil {
		return nil, err
	}
	var d common.StationIdItemStatusStruct
	for rows.Next() {
		err = rows.Scan(&d.StationId, &d.ItemName, &d.Status, &d.ChangedAt)
		if err != nil {
			return nil, err
		}
		ds.Data = append(ds.Data, d)
	}
	return ds, rows.Err()
}

func GetItemStatusLogs(stationId uuid.UUID, after int64) ([]common.RowIdItemStatusStruct, error) {
	rows, err := TideDB.Query(`select row_id, item_name, status, changed_at from item_status_log where station_id=$1 and row_id>$2 order by row_id`, stationId, after)
	if err != nil {
		return nil, err
	}
	var (
		h  common.RowIdItemStatusStruct
		hs []common.RowIdItemStatusStruct
	)
	for rows.Next() {
		err = rows.Scan(&h.RowId, &h.ItemName, &h.Status, &h.ChangedAt)
		if err != nil {
			return nil, err
		}
		hs = append(hs, h)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return hs, err
}
