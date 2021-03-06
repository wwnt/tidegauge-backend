package db

import (
	"database/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"tide/common"
	"tide/pkg/custype"
	"time"
)

func GetItemsLatest(stationId uuid.UUID, itemsLatest common.StringMsecMap) error {
	var t sql.NullTime
	for itemName := range itemsLatest {
		err := TideDB.QueryRow("select max(timestamp) from "+itemName+" where station_id=$1", stationId).Scan(&t)
		if err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "42P01" {
				continue
			}
			return err
		}
		if t.Valid {
			itemsLatest[itemName] = custype.ToTimeMillisecond(t.Time)
		}
	}
	return nil
}

func GetDataHistory(stationId uuid.UUID, itemName string, start, end custype.TimeMillisecond) ([]common.DataTimeStruct, error) {
	var (
		err error
		d   common.DataTimeStruct
		ds  []common.DataTimeStruct
	)
	if start == 0 && end == 0 {
		err = TideDB.QueryRow("select timestamp, value from "+itemName+" where station_id=$1 order by timestamp desc limit 1", stationId).Scan(&d.Millisecond, &d.Value)
		if err != nil {
			if err == sql.ErrNoRows {
				err = nil
			}
			return ds, err
		}
		ds = append(ds, d)
	} else {
		var rows *sql.Rows
		if end == 0 {
			rows, err = TideDB.Query("select timestamp, value from "+itemName+" where station_id=$1 and timestamp>$2 order by timestamp", stationId, start)
		} else {
			rows, err = TideDB.Query("select timestamp, value from "+itemName+" where station_id=$1 and timestamp>$2 and timestamp<$3 order by timestamp", stationId, start, end)
		}
		if err != nil {
			return ds, err
		}
		for rows.Next() {
			err = rows.Scan(&d.Millisecond, &d.Value)
			if err != nil {
				return nil, err
			}
			ds = append(ds, d)
		}
		if err = rows.Err(); err != nil {
			return ds, err
		}
	}
	return ds, err
}

func SaveDataHistory(stationId uuid.UUID, itemName string, itemValue float64, tm time.Time) (int64, error) {
	res, err := TideDB.Exec(`insert into `+itemName+` (station_id, value, timestamp) VALUES ($1,$2,$3) on conflict do nothing`, stationId, itemValue, tm)
	return checkResult(res, err)
}

func GetLatestDataTime(stationId uuid.UUID, itemName string) (ts custype.TimeMillisecond, err error) {
	err = TideDB.QueryRow("select timestamp from "+itemName+" where station_id=$1 order by timestamp desc limit 1", stationId).Scan(&ts)
	if err != nil && err == sql.ErrNoRows {
		return 0, nil
	}
	return ts, err
}
