package db

import (
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/global"
	"time"
)

var (
	TideDB *sql.DB
	seaDB  *sql.DB
)

func openDB() error {
	var err error
	TideDB, err = sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		global.Config.Db.Tide.Host,
		global.Config.Db.Tide.Port,
		global.Config.Db.Tide.User,
		global.Config.Db.Tide.Password,
		global.Config.Db.Tide.DBName,
	))
	if err != nil {
		return err
	}
	seaDB, err = sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		global.Config.Db.Sea.Host,
		global.Config.Db.Sea.Port,
		global.Config.Db.Sea.User,
		global.Config.Db.Sea.Password,
		global.Config.Db.Sea.DBName,
	))
	return err
}

func Init() {
	var (
		err    error
		logger = zap.L()
	)
	if err = openDB(); err != nil {
		logger.Fatal("db init", zap.Error(err))
	}
	if err = setAllDisconnected(); err != nil {
		logger.Fatal(err.Error())
	}
	if err = setAllNotAvailable(); err != nil {
		logger.Fatal(err.Error())
	}
}

func CloseDB() {
	_ = TideDB.Close()
	_ = seaDB.Close()
}

func checkResult(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func setAllDisconnected() error {
	now := custype.ToTimeMillisecond(time.Now()).ToTime()
	_, err := TideDB.Exec(`update stations set status=$1, status_changed_at=$2 where upstream=false and status!=$1`, common.Disconnected, now)
	return err
}

func setAllNotAvailable() error {
	_, err := TideDB.Exec(`update items set available='' where station_id in (select id from stations where upstream=true)`)
	return err
}
