package db

import (
	"database/sql"
	"errors"
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

// GetItemStatusLogChangedAt 返回 (station_id, row_id) 行的 changed_at。
// 行不存在时返回 (zeroTime, false, nil)。
func GetItemStatusLogChangedAt(stationId uuid.UUID, rowId int64) (time.Time, bool, error) {
	var changedAt time.Time
	err := TideDB.QueryRow(`select changed_at from item_status_log where station_id=$1 and row_id=$2`, stationId, rowId).Scan(&changedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return changedAt, true, nil
}

// PruneItemStatusLogAndRecomputeItems 删除 item_status_log 中受影响的行（all=true 删全部该站点；
// 否则删 row_id > rowId 的尾巴），并重算受影响 items 的 status / status_changed_at。
// 重算用与 db.SyncItem 一致的 latestStatus CTE，无日志时兜底为空状态和 epoch 时间。
func PruneItemStatusLogAndRecomputeItems(stationId uuid.UUID, rowId int64, all bool) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		deleteSQL  string
		affectArgs []any
	)
	if all {
		deleteSQL = `delete from item_status_log where station_id=$1`
		affectArgs = []any{stationId}
	} else {
		deleteSQL = `delete from item_status_log where station_id=$1 and row_id>$2`
		affectArgs = []any{stationId, rowId}
	}

	rows, err := tx.Query(`select distinct item_name from item_status_log where `+func() string {
		if all {
			return `station_id=$1`
		}
		return `station_id=$1 and row_id>$2`
	}(), affectArgs...)
	if err != nil {
		return 0, err
	}
	var affectedItems []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			_ = rows.Close()
			return 0, err
		}
		affectedItems = append(affectedItems, name)
	}
	_ = rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}

	res, err := tx.Exec(deleteSQL, affectArgs...)
	if err != nil {
		return 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	for _, itemName := range affectedItems {
		_, err = tx.Exec(`with latestStatus as ((select status, changed_at from item_status_log where station_id=$1 and item_name=$2 order by row_id desc limit 1)
union (select '', 'epoch'::timestamptz) order by changed_at desc limit 1)
update items set status=(select status from latestStatus), status_changed_at=(select changed_at from latestStatus) where station_id=$1 and name=$2`,
			stationId, itemName)
		if err != nil {
			return 0, err
		}
	}

	return deleted, tx.Commit()
}

type pagedItemStatusLogStruct struct {
	Total int                                `json:"total"`
	Data  []common.StationIdItemStatusStruct `json:"data"`
}

func PagedItemStatusLogs(pageNum, pageSize uint) (any, error) {
	var ds pagedItemStatusLogStruct
	err := TideDB.QueryRow(`select count(*) from item_status_log`).Scan(&ds.Total)
	if err != nil {
		return nil, err
	}
	rows, err := TideDB.Query("select station_id, item_name, status, changed_at from item_status_log order by changed_at desc limit $1 offset $2", pageSize, (pageNum-1)*pageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
