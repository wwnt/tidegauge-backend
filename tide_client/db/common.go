package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"log"
	"os"
	"testing"
	"tide/common"
	"tide/tide_client/global"
)

var (
	db *sql.DB
)

func Init() {
	var err error
	db, err = sql.Open("sqlite3", global.Config.Db.Dsn)
	if err != nil {
		log.Fatal(err)
	}
}

func Close() {
	_ = db.Close()
}

func checkResultLastInsertId(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func MakeSureTableExist(name string) (err error) {
	_, err = db.Exec(`create table if not exists ` + name + `
(
    timestamp int              not null,
    value     double precision not null
);
create index if not exists ` + name + `_timestamp_index on ` + name + ` (timestamp);`)
	return err
}

func InitDB() {
	initSql, err := os.ReadFile("../schema.sql")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(string(initSql))
	if err != nil {
		log.Fatal(err)
	}
}

var (
	StatusLogs = []common.RowIdItemStatusStruct{
		{
			RowId: 1,
			ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "item1",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: 1100},
			},
		},
		{
			RowId: 2,
			ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "item2",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: 1200},
			},
		},
	}
	DataHis = []common.ItemNameDataTimeStruct{
		{
			ItemName:       "item1",
			DataTimeStruct: common.DataTimeStruct{Value: 1, Millisecond: 1100},
		},
		{
			ItemName:       "item2",
			DataTimeStruct: common.DataTimeStruct{Value: 1, Millisecond: 1100},
		},
	}
)

func InitData(t *testing.T) {
	_, err := db.Exec(`
delete from item_status_log;
drop table if exists item1;
drop table if exists item2;
`)
	require.NoError(t, err)

	for _, statusLog := range StatusLogs {
		rowId, err := SaveItemStatusLog(statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToInt64())
		require.NoError(t, err)
		require.EqualValues(t, statusLog.RowId, rowId)
	}
	var tmp = make(map[string]struct{})
	for _, data := range DataHis {
		if _, ok := tmp[data.ItemName]; !ok {
			err = MakeSureTableExist(data.ItemName)
			require.NoError(t, err)
		}
		tmp[data.ItemName] = struct{}{}
		err = SaveData(data.ItemName, data.Value, data.Millisecond.ToInt64())
		require.NoError(t, err)
	}
}
