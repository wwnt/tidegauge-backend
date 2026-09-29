package db

import (
	"database/sql"
	"fmt"
	"slices"
	"tide/common"
)

// dataItemColumns 是数据表（每个 item 一张）的固定列结构。
var dataItemColumns = []string{"timestamp", "value"}

// statusLogTable 是状态日志表：列结构与数据表不同，由 CleanItemStatusLog 单独清理。
const statusLogTable = "item_status_log"

// CleanDBData 删除超过保留期的时序数据与状态日志。
//
// 清理范围是两部分并集：
//   - dataItemNames：当前设备配置里的全部数据项（即 SQLite 数据表名），来自
//     controller.addDevices。首次启动清理跑在建表之前，此时这些表可能尚不存在，
//     但仍要校验并清理——新表一旦建出，数据就归入保留期管理。
//   - data.db 中实际存在、列结构符合数据表的其他表（孤儿表）：设备或 item 改名/
//     移除后遗留的旧表。若不处理会永久占盘；只删过期行，不删表（是否 DROP 留给
//     运维决定）。
func CleanDBData(cutoffTime int64, dataItemNames []string) error {
	tables, err := getAllTables()
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	// 待清理集合 = 库中实际存在的表 ∪ 配置清单（后者在首次启动清理跑在建表之前
	// 时可能尚不存在，DELETE 空表是无害空操作）。两者高度重叠，容量按 tables 预分配。
	toClean := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		toClean[table] = struct{}{}
	}
	for _, item := range dataItemNames {
		toClean[item] = struct{}{}
	}

	for table := range toClean {
		if table == statusLogTable {
			continue // 状态日志按 changed_at 清理，见 CleanItemStatusLog。
		}
		columns, err := tableColumns(table)
		if err != nil {
			return fmt.Errorf("inspect table %s: %w", table, err)
		}
		if !slices.Equal(columns, dataItemColumns) {
			continue // 非数据表（sqlite_sequence 等），跳过。
		}
		if err = deleteOldData(table, cutoffTime); err != nil {
			return fmt.Errorf("clean data table %s: %w", table, err)
		}
	}

	if err := CleanItemStatusLog(cutoffTime); err != nil {
		return fmt.Errorf("clean status log: %w", err)
	}
	return nil
}

// getAllTables 列出库中所有用户表（排除 SQLite 内部表）。
func getAllTables() ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
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
	return tables, rows.Err()
}

// tableColumns 返回表的列名列表。
func tableColumns(tableName string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

// DeleteOldData deletes old data from a specific table in the database.
func deleteOldData(tableName string, cutoffTime int64) error {
	if common.ContainsIllegalCharacter(tableName) {
		return fmt.Errorf("illegal item name: %q", tableName)
	}
	_, err := db.Exec("delete from"+" "+tableName+" where timestamp < ?", cutoffTime)
	if err != nil {
		return fmt.Errorf("delete rows: %w", err)
	}
	return nil
}
