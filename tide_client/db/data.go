package db

import (
	"database/sql"
	"tide/common"
)

func SaveData(itemName string, val float64, msec int64) error {
	_, err := db.Exec("insert"+" into "+itemName+" (timestamp, value) VALUES (?,?)", msec, val)
	return err
}
func GetDataHistory(itemName string, start, end int64) ([]common.DataTimeStruct, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if end == 0 {
		rows, err = db.Query("select"+" timestamp, value from "+itemName+" where timestamp>? order by timestamp", start)
	} else {
		rows, err = db.Query("select"+" timestamp, value from "+itemName+" where timestamp>? and timestamp<? order by timestamp", start, end)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		d  common.DataTimeStruct
		ds []common.DataTimeStruct
	)
	for rows.Next() {
		err = rows.Scan(&d.Millisecond, &d.Value)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, nil
}
