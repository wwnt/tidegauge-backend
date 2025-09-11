package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tide/common"
	"tide/tide_server/global"

	_ "github.com/jackc/pgx/v5/stdlib"
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
	if err := openDB(); err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	if err := setAllDisconnected(); err != nil {
		slog.Error("Failed to set all stations disconnected", "error", err)
		os.Exit(1)
	}
	if err := setAllNotAvailable(); err != nil {
		slog.Error("Failed to set all upstream items not available", "error", err)
		os.Exit(1)
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
	now := time.Now().Truncate(time.Millisecond)
	_, err := TideDB.Exec(`update stations set status=$1, status_changed_at=$2 where upstream=false and status!=$1`, common.Disconnected, now)
	return err
}

func setAllNotAvailable() error {
	_, err := TideDB.Exec(`update items set available='' where station_id in (select id from stations where upstream=true)`)
	return err
}
