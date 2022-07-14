package test

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"tide/tide_server/global"
)

func InitDB() {
	tideDb, err := sql.Open("pgx", fmt.Sprintf(
		"sslmode=disable host=%s port=%s user=%s password=%s",
		global.Config.Db.Tide.Host,
		global.Config.Db.Tide.Port,
		global.Config.Db.Tide.User,
		global.Config.Db.Tide.Password,
	))
	if err != nil {
		log.Fatal(err)
	}
	_, err = tideDb.Exec("drop database if exists " + global.Config.Db.Tide.DBName)
	if err != nil {
		log.Fatal(err)
	}
	_, err = tideDb.Exec("create database " + global.Config.Db.Tide.DBName)
	if err != nil {
		log.Fatal(err)
	}
	_ = tideDb.Close()

	tideDb, err = sql.Open("pgx", fmt.Sprintf(
		"sslmode=disable host=%s port=%s user=%s password=%s dbname=%s",
		global.Config.Db.Tide.Host,
		global.Config.Db.Tide.Port,
		global.Config.Db.Tide.User,
		global.Config.Db.Tide.Password,
		global.Config.Db.Tide.DBName,
	))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = tideDb.Close() }()

	initSql, err := os.ReadFile("../schema.sql")
	if err != nil {
		log.Fatal(err)
	}
	_, err = tideDb.Exec(string(initSql))
	if err != nil {
		log.Fatal(err)
	}
}
