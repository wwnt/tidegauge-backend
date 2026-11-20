package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"tide/common"
)

// dataItemColumns 是数据表（每个 item 一张）的固定列结构。
var dataItemColumns = []string{"timestamp", "value"}

// statusLogTable 是状态日志表：列结构与数据表不同，由 CleanItemStatusLog 单独清理。
const statusLogTable = "item_status_log"

// PathFromDSN 从 SQLite DSN 中提取数据库文件所在目录。
// 兼容 "file:data.db?params"、"file:/abs/path/data.db" 与裸 "data.db" 形式；
// 相对路径的库文件位于进程工作目录，返回 "."（模板和现场配置都是这种写法，
// 磁盘余量检查必须覆盖到）。内存库（:memory: 或 mode=memory）返回 ok=false。
func PathFromDSN(dsn string) (dir string, ok bool) {
	path := strings.TrimPrefix(dsn, "file:")
	var params string
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path, params = path[:i], path[i+1:]
	}
	if path == "" || path == ":memory:" {
		return "", false
	}
	for _, param := range strings.Split(params, "&") {
		if param == "mode=memory" {
			return "", false
		}
	}
	return filepath.Dir(path), true
}

// CleanDBData 删除超过保留期的时序数据与状态日志。
//
// 清理范围是两部分并集：
//   - dataItemNames：当前设备配置里的全部数据项（即 SQLite 数据表名），来自
//     controller.addDevices。
//   - data.db 中实际存在、列结构符合数据表的其他表（孤儿表）：设备或 item 改名/
//     移除后遗留的旧表。若不处理会永久占盘；只删过期行，不删表（是否 DROP 留给
//     运维决定）。
//
// 单张表失败不会中断整体清理：清理与传感器写入并发，个别 DELETE 撞上写锁是常态
// （cache=shared 的 DSN 下会立即报 "database table is locked"），此时记下错误、继续处理
// 其余表，状态日志也照常清理；失败的表留到下一轮。所有错误合并后返回。
func CleanDBData(cutoffTime int64, dataItemNames []string) error {
	tables, err := getAllTables()
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	// 待清理集合 = 库中实际存在的表 ∪ 配置清单（两者高度重叠，容量按 tables 预分配）。
	toClean := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		toClean[table] = struct{}{}
	}
	for _, item := range dataItemNames {
		toClean[item] = struct{}{}
	}

	var errs []error
	for table := range toClean {
		if table == statusLogTable {
			continue // 状态日志按 changed_at 清理，见 CleanItemStatusLog。
		}
		// 表名会被拼进 SQL，必须先于任何使用点校验。
		if common.ContainsIllegalCharacter(table) {
			errs = append(errs, fmt.Errorf("illegal item name: %q", table))
			continue
		}
		columns, err := tableColumns(table)
		if err != nil {
			errs = append(errs, fmt.Errorf("inspect table %s: %w", table, err))
			continue
		}
		if !slices.Equal(columns, dataItemColumns) {
			continue // 非数据表，或配置里有但尚未建表（无列），跳过。
		}
		if err = deleteOldData(table, cutoffTime); err != nil {
			errs = append(errs, fmt.Errorf("clean data table %s: %w", table, err))
		}
	}

	if err := CleanItemStatusLog(cutoffTime); err != nil {
		errs = append(errs, fmt.Errorf("clean status log: %w", err))
	}
	return errors.Join(errs...)
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

// tableColumns 返回表的列名列表；表不存在时返回空列表。
// 调用方必须先用 common.ContainsIllegalCharacter 校验 tableName（PRAGMA 不支持参数绑定）。
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
