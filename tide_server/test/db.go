package test

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"tide/tide_server/global"
)

const postgresIdentifierMaxLen = 63

func InitDB() (cleanup func(), err error) {
	uniqueName, err := uniqueTestDBName(global.Config.Db.Tide.DBName)
	if err != nil {
		return func() {}, err
	}

	// Must happen before db.Init() opens TideDB.
	global.Config.Db.Tide.DBName = uniqueName

	cleanup = func() {
		adminDB, err := openAdminDB()
		if err != nil {
			log.Printf("test db cleanup: open admin db failed: %v", err)
			return
		}
		defer func() { _ = adminDB.Close() }()

		_, err = adminDB.Exec(
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND pid <> pg_backend_pid()`,
			uniqueName,
		)
		if err != nil {
			log.Printf("test db cleanup: terminate backends failed: %v", err)
		}

		_, err = adminDB.Exec("DROP DATABASE IF EXISTS " + quoteIdentifier(uniqueName))
		if err != nil {
			log.Printf("test db cleanup: drop database failed: %v", err)
		}
	}

	if err := createAndInitDatabase(uniqueName); err != nil {
		cleanup()
		return cleanup, err
	}

	return cleanup, nil
}

func createAndInitDatabase(dbName string) error {
	adminDB, err := openAdminDB()
	if err != nil {
		return err
	}
	defer func() { _ = adminDB.Close() }()

	_, err = adminDB.Exec("CREATE DATABASE " + quoteIdentifier(dbName))
	if err != nil {
		return err
	}

	tideDB, err := openTideDB(dbName)
	if err != nil {
		return err
	}
	defer func() { _ = tideDB.Close() }()

	initSql, err := os.ReadFile("../schema.sql")
	if err != nil {
		return err
	}
	_, err = tideDB.Exec(string(initSql))
	return err
}

func openAdminDB() (*sql.DB, error) {
	return sql.Open("pgx", fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		global.Config.Db.Tide.Host,
		global.Config.Db.Tide.Port,
		global.Config.Db.Tide.User,
		global.Config.Db.Tide.Password,
	))
}

func openTideDB(dbName string) (*sql.DB, error) {
	return sql.Open("pgx", fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		global.Config.Db.Tide.Host,
		global.Config.Db.Tide.Port,
		global.Config.Db.Tide.User,
		global.Config.Db.Tide.Password,
		dbName,
	))
}

func uniqueTestDBName(baseName string) (string, error) {
	baseName = sanitizeIdentifier(baseName)
	if baseName != "" && baseName[0] >= '0' && baseName[0] <= '9' {
		baseName = "t_" + baseName
	}

	rnd, err := randomHex(4) // 8 hex chars
	if err != nil {
		return "", err
	}
	suffix := fmt.Sprintf("_p%d_%s", os.Getpid(), rnd)

	if len(suffix) >= postgresIdentifierMaxLen {
		return "", fmt.Errorf("test db suffix too long: %q", suffix)
	}
	maxBaseLen := postgresIdentifierMaxLen - len(suffix)
	if len(baseName) > maxBaseLen {
		baseName = baseName[:maxBaseLen]
	}

	name := baseName + suffix
	if name == "" {
		return "", fmt.Errorf("test db name is empty")
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "t_" + name
		if len(name) > postgresIdentifierMaxLen {
			name = name[:postgresIdentifierMaxLen]
		}
	}
	return name, nil
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sanitizeIdentifier(s string) string {
	s = strings.ToLower(s)
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			out = append(out, c)
		case c >= '0' && c <= '9':
			out = append(out, c)
		case c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

func quoteIdentifier(identifier string) string {
	// Identifiers cannot be parameterized, so we sanitize and then quote.
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
