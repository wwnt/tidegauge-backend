package db

import (
	"tide/common"
)

func SaveItemStatusLog(itemName string, status common.Status, changedAt int64) (int64, error) {
	res, err := db.Exec("insert into item_status_log(item_name, status, changed_at) values (?,?,?)", itemName, status, changedAt)
	return checkResultLastInsertId(res, err)
}

func GetItemsLatestStatus() ([]common.ItemStatusStruct, error) {
	rows, err := db.Query(`select item_name, status, changed_at from item_status_log 
where ROWID in (select max(ROWID) from item_status_log group by item_name)`)
	if err != nil {
		return nil, err
	}
	var (
		d  common.ItemStatusStruct
		ds []common.ItemStatusStruct
	)
	for rows.Next() {
		err = rows.Scan(&d.ItemName, &d.Status, &d.ChangedAt)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, err
}

func GetItemStatusLogAfter(after int64) ([]common.RowIdItemStatusStruct, error) {
	rows, err := db.Query(`select ROWID, item_name, status, changed_at from item_status_log where ROWID>$1 order by ROWID`, after)
	if err != nil {
		return nil, err
	}
	var (
		d  common.RowIdItemStatusStruct
		ds []common.RowIdItemStatusStruct
	)
	for rows.Next() {
		err = rows.Scan(&d.RowId, &d.ItemName, &d.Status, &d.ChangedAt)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, err
}
