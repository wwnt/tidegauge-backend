package db

import (
	"database/sql"
	"errors"
	"fmt"

	"tide/common"
)

func SaveItemStatusLog(itemName string, status common.Status, changedAt int64) (int64, error) {
	res, err := db.Exec("insert into item_status_log(item_name, status, changed_at) values (?,?,?)", itemName, status, changedAt)
	return checkResultLastInsertId(res, err)
}

// cleanStatusLogBatchSize 是单次事务删除的状态日志行数上限。整表一次性删除会在
// 旧积压的首次清理时长时间占用 SQLite 写锁，把并发的传感器数据写入饿死（忙等超时
// 即丢一个数据点），所以分批执行，批次之间让写锁回流。
const cleanStatusLogBatchSize = 1000

// CleanItemStatusLog 删除 changed_at 早于 cutoffTime（unix ms）的状态日志。
// 状态日志的列结构与数据表不同，由本函数单独清理，调用方（CleanDBData）无需做表结构探测。
//
// 每个 item 最新的一行（rowid 最大者）无论多旧都保留，原因有二：
//
//  1. 它是该 item 的"当前状态"。重启时 GetItemsLatestStatus 靠它恢复状态缓存；删掉后
//     每个 item 的第一条读数都会被当成一次状态翻转，并盖上重启时刻的时间戳。
//  2. 表没有 AUTOINCREMENT，SQLite 分配的新 rowid 是 max(rowid)+1。全表最大 rowid 那行
//     必然是某个 item 的最新行，保留它就保证 rowid 单调不回退。服务端以
//     (station_id, row_id) 为主键并且 "on conflict do nothing"，rowid 一旦复用，
//     新的状态变化会被服务端当成已有行静默丢弃。
//
// 批删用 rowid 子查询而不是 "delete ... limit ?"：SQLite 对 DELETE 的 LIMIT 支持是
// 编译期可选语法（parse.y 的 where_opt_ret 规则带不带 limit_opt 分支），mattn/go-sqlite3
// 的 amalgamation（sqlite3-binding.c，SQLite 3.53.0）里的语法表预生成时就没带 LIMIT
// 分支——where_opt_ret 只有空/WHERE/RETURNING 四个变体，因此 cgo CFLAGS 里的
// -DSQLITE_ENABLE_UPDATE_DELETE_LIMIT 无法补救（那只影响执行期辅助函数，不影响已
// 烧录的语法表）。实测（2026-09-23，go1.26 + v1.14.44）运行时报
// 'near "limit": syntax error'。已查最新 master（2026-09-23）语法表同样没有 LIMIT
// 分支，升级驱动无用；除非换 libsqlite3 系统库构建或自行重生成语法表，勿改回 limit 写法。
func CleanItemStatusLog(cutoffTime int64) error {
	// changed_at 上没有索引时，删除积压会退化为全表扫描；这里幂等地补建，
	// 兼容早已按旧 schema.sql 初始化的存量库。
	if _, err := db.Exec("create index if not exists item_status_log_changed_at_index on item_status_log (changed_at)"); err != nil {
		return fmt.Errorf("ensure changed_at index: %w", err)
	}

	for {
		// exists 子查询走 item_name 索引（索引项自带 rowid），判断"同一 item 还有更新的行"
		// 是一次索引定位；被保留的行每批会被重新扫到，但数量不超过 item 个数。
		res, err := db.Exec(`delete from item_status_log where rowid in (
			select old.rowid from item_status_log as old
			where old.changed_at < ?
			  and exists (select 1 from item_status_log as newer
			              where newer.item_name = old.item_name and newer.rowid > old.rowid)
			limit ?)`,
			cutoffTime, cleanStatusLogBatchSize)
		if err != nil {
			return fmt.Errorf("failed to delete status log: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to count deleted status log: %w", err)
		}
		if n < cleanStatusLogBatchSize {
			return nil
		}
	}
}

func GetItemsLatestStatus() ([]common.ItemStatusStruct, error) {
	rows, err := db.Query(`select item_name, status, changed_at from item_status_log 
where ROWID in (select max(ROWID) from item_status_log group by item_name)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		d  common.ItemStatusStruct
		ds []common.ItemStatusStruct
	)
	for rows.Next() {
		err = rows.Scan(&d.ItemName, &d.Status, &d.ChangedAt)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, err
}

func GetItemStatusLogAfter(after int64) ([]common.RowIdItemStatusStruct, error) {
	rows, err := db.Query(`select ROWID, item_name, status, changed_at from item_status_log where ROWID>$1 order by ROWID`, after)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		d  common.RowIdItemStatusStruct
		ds []common.RowIdItemStatusStruct
	)
	for rows.Next() {
		err = rows.Scan(&d.RowId, &d.ItemName, &d.Status, &d.ChangedAt)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, err
}

// GetItemStatusLogLatest 返回 item_status_log 最大 ROWID 行的 (rowID, changed_at)。
// 表为空时返回 (0, 0, nil)。
func GetItemStatusLogLatest() (int64, int64, error) {
	var rowID, changedAt int64
	err := db.QueryRow(`select ROWID, changed_at from item_status_log order by ROWID desc limit 1`).Scan(&rowID, &changedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return rowID, changedAt, err
}
