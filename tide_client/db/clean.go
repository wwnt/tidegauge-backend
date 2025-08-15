package db

import (
	"database/sql"
	"fmt"
	"log"
	"slices"
)

func CleanDBData(cutoffTime int64) {
	tables, err := GetAllTables(db)
	if err != nil {
		log.Println(err)
		return
	}
	// Iterate over each table
	for _, table := range tables {
		valid, err := IsValidTable(db, table)
		if err != nil {
			log.Printf("Error validating table %s: %v", table, err)
			continue
		}
		if !valid {
			continue
		}

		if err = DeleteOldData(db, table, cutoffTime); err != nil {
			log.Printf("Error cleaning table %s: %v", table, err)
		}
	}
}

// GetAllTables retrieves all table names from the SQLite database.
func GetAllTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
}

// IsValidTable checks if the table matches the required structure.
func IsValidTable(db *sql.DB, tableName string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s);", tableName))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		columns = append(columns, name)
	}

	// Validate columns
	if slices.Equal(columns, []string{"timestamp", "value"}) {
		return true, nil
	}
	return false, nil
}

// DeleteOldData deletes old data from a specific table in the database.
func DeleteOldData(db *sql.DB, tableName string, before int64) error {
	_, err := db.Exec(fmt.Sprintf("delete from"+" %s where timestamp < ?", tableName), before)
	if err != nil {
		return fmt.Errorf("failed to delete data from table %s: %w", tableName, err)
	}
	return nil
}
