package db

import (
	"database/sql"
	"log/slog"
	"os"
	"tide/tide_client/global"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db *sql.DB
)

func Init() {
	var err error
	db, err = sql.Open("sqlite3", global.Config.Db.Dsn)
	if err != nil {
		slog.Error("Failed to open SQLite database", "dsn", global.Config.Db.Dsn, "error", err)
		os.Exit(1)
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
		slog.Error("Failed to read database initialization file", "error", err)
		os.Exit(1)
	}
	_, err = db.Exec(string(initSql))
	if err != nil {
		slog.Error("Failed to execute database initialization SQL", "error", err)
		os.Exit(1)
	}
}
