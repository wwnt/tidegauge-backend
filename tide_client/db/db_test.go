package db

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tide/common"
)

func TestGetDataHistory(t *testing.T) {
	require.NoError(t, ResetFixtures())
	type args struct {
		itemName string
		start    int64
		end      int64
	}
	tests := []struct {
		name    string
		args    args
		want    []common.DataTimeStruct
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "1", args: args{itemName: "item1", start: 0, end: 0},
			want:    []common.DataTimeStruct{DataHis[0].DataTimeStruct},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDataHistory(tt.args.itemName, tt.args.start, tt.args.end)
			if !tt.wantErr(t, err, fmt.Sprintf("GetDataHistory(%v, %v, %v)", tt.args.itemName, tt.args.start, tt.args.end)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetDataHistory(%v, %v, %v)", tt.args.itemName, tt.args.start, tt.args.end)
		})
	}
}

func TestGetItemStatusLogAfter(t *testing.T) {
	require.NoError(t, ResetFixtures())
	type args struct {
		after int64
	}
	tests := []struct {
		name    string
		args    args
		want    []common.RowIdItemStatusStruct
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "1", args: args{after: 0}, want: StatusLogs, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetItemStatusLogAfter(tt.args.after)
			if !tt.wantErr(t, err, fmt.Sprintf("GetItemStatusLogAfter(%v)", tt.args.after)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetItemStatusLogAfter(%v)", tt.args.after)
		})
	}
}

func TestGetItemsLatestStatus(t *testing.T) {
	require.NoError(t, ResetFixtures())
	tests := []struct {
		name    string
		want    []common.ItemStatusStruct
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "1", want: []common.ItemStatusStruct{StatusLogs[0].ItemStatusStruct, StatusLogs[1].ItemStatusStruct}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetItemsLatestStatus()
			if !tt.wantErr(t, err, "GetItemsLatestStatus()") {
				return
			}
			assert.Equalf(t, tt.want, got, "GetItemsLatestStatus()")
		})
	}
}

func TestSaveItemStatusLog(t *testing.T) {
	require.NoError(t, ResetFixtures())
	type args struct {
		itemName  string
		status    common.Status
		changedAt int64
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "1", args: args{itemName: "item1", status: common.Normal, changedAt: 1}, want: 3, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SaveItemStatusLog(tt.args.itemName, tt.args.status, tt.args.changedAt)
			if !tt.wantErr(t, err, fmt.Sprintf("SaveItemStatusLog(%v, %v, %v)", tt.args.itemName, tt.args.status, tt.args.changedAt)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SaveItemStatusLog(%v, %v, %v)", tt.args.itemName, tt.args.status, tt.args.changedAt)
		})
	}
}

func TestSaveData(t *testing.T) {
	require.NoError(t, ResetFixtures())
	type args struct {
		itemName string
		val      float64
		msec     int64
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "1", args: args{itemName: "item1", val: 1, msec: 200}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, SaveData(tt.args.itemName, tt.args.val, tt.args.msec), fmt.Sprintf("SaveData(%v, %v, %v)", tt.args.itemName, tt.args.val, tt.args.msec))
		})
	}
}

func TestCleanItemStatusLog(t *testing.T) {
	require.NoError(t, ResetFixtures())
	// 种子是 rowid 1(item1, 1100)、rowid 2(item2, 1200)；再给 item1 追加两次翻转。
	for _, changedAt := range []int64{1300, 1400} {
		if _, err := SaveItemStatusLog("item1", common.Abnormal, changedAt); err != nil {
			t.Fatalf("seed status log: %v", err)
		}
	}
	rowIDs := func() []int64 {
		t.Helper()
		logs, err := GetItemStatusLogAfter(0)
		if err != nil {
			t.Fatalf("GetItemStatusLogAfter returned error: %v", err)
		}
		ids := make([]int64, 0, len(logs))
		for _, l := range logs {
			ids = append(ids, l.RowId)
		}
		return ids
	}

	// cutoff 早于所有行时不得误删。
	if err := CleanDBData(0, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}
	assert.Equal(t, []int64{1, 2, 3, 4}, rowIDs(), "a cutoff before every row must delete nothing")

	// cutoff 落在中间：item1 的旧行 1、3 被删；item2 只有一行（它的最新行），再旧也保留。
	if err := CleanDBData(1350, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}
	assert.Equal(t, []int64{2, 4}, rowIDs(), "old rows go, but each item keeps its newest row")

	// cutoff 晚于所有行：每个 item 的最新行仍然保留，状态缓存重启后还能恢复。
	if err := CleanDBData(5000, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}
	assert.Equal(t, []int64{2, 4}, rowIDs(), "the newest row of every item must survive any cutoff")
	latest, err := GetItemsLatestStatus()
	if err != nil {
		t.Fatalf("GetItemsLatestStatus returned error: %v", err)
	}
	assert.Len(t, latest, 2, "current status of every item must still be recoverable")

	// rowid 不得复用：服务端按 (station_id, row_id) 去重，复用会让新的状态变化被丢弃。
	rowID, err := SaveItemStatusLog("item2", common.Normal, 6000)
	if err != nil {
		t.Fatalf("SaveItemStatusLog returned error: %v", err)
	}
	assert.EqualValues(t, 5, rowID, "rowid must keep increasing after cleanup")

	// 数据表同样被清理：种子行的时间戳 1100 早于 cutoff 5000。
	his, err := GetDataHistory("item1", 0, 0)
	if err != nil {
		t.Fatalf("GetDataHistory returned error: %v", err)
	}
	assert.Empty(t, his, "data table rows older than the cutoff must be deleted")
}

// 清理批次按 1000 行切分；积压超过一批时也必须删干净并保留每个 item 的最新行。
func TestCleanItemStatusLogMultipleBatches(t *testing.T) {
	require.NoError(t, ResetFixtures())
	const backlog = cleanStatusLogBatchSize*2 + 50
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for i := 0; i < backlog; i++ {
		if _, err = tx.Exec("insert into item_status_log(item_name, status, changed_at) values (?,?,?)", "item1", common.Normal, 2000+i); err != nil {
			t.Fatalf("seed backlog: %v", err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err = CleanItemStatusLog(1_000_000); err != nil {
		t.Fatalf("CleanItemStatusLog returned error: %v", err)
	}
	logs, err := GetItemStatusLogAfter(0)
	if err != nil {
		t.Fatalf("GetItemStatusLogAfter returned error: %v", err)
	}
	if assert.Len(t, logs, 2, "only the newest row of item1 and of item2 may remain") {
		assert.EqualValues(t, 2, logs[0].RowId)
		assert.EqualValues(t, 2+backlog, logs[1].RowId)
	}
}

func TestCleanDBDataRejectsIllegalItemName(t *testing.T) {
	require.NoError(t, ResetFixtures())

	err := CleanDBData(0, []string{"item1\"; drop table item1"})
	if err == nil {
		t.Fatal("CleanDBData accepted an item name with SQL metacharacters")
	}
	assert.ErrorContains(t, err, "illegal item name", "the name must be rejected by validation, not by a SQL syntax error")
	if _, err = GetDataHistory("item1", 0, 0); err != nil {
		t.Fatalf("item1 must still exist: %v", err)
	}
}

// 一张表出错不能让其余表和状态日志的清理被跳过。
func TestCleanDBDataContinuesAfterTableError(t *testing.T) {
	require.NoError(t, ResetFixtures())
	if _, err := SaveItemStatusLog("item1", common.Abnormal, 1300); err != nil {
		t.Fatalf("seed status log: %v", err)
	}

	err := CleanDBData(5000, []string{"BAD-NAME", "item1", "item2"})
	assert.ErrorContains(t, err, "illegal item name")

	for _, item := range []string{"item1", "item2"} {
		his, hisErr := GetDataHistory(item, 0, 0)
		if hisErr != nil {
			t.Fatalf("GetDataHistory(%s) returned error: %v", item, hisErr)
		}
		assert.Emptyf(t, his, "%s must still be cleaned when another table fails", item)
	}
	logs, logErr := GetItemStatusLogAfter(0)
	if logErr != nil {
		t.Fatalf("GetItemStatusLogAfter returned error: %v", logErr)
	}
	assert.Len(t, logs, 2, "status log cleanup must still run when a table fails")
}

func TestPathFromDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dsn    string
		want   string
		wantOK bool
	}{
		// 相对路径的库文件在进程工作目录下：这是模板和现场配置的写法，必须能定位到目录。
		{name: "fileWithParams", dsn: "file:data.db?_journal_mode=WAL&_foreign_keys=on", want: ".", wantOK: true},
		{name: "fileNoParams", dsn: "file:data.db", want: ".", wantOK: true},
		{name: "bareName", dsn: "data.db", want: ".", wantOK: true},
		{name: "memory", dsn: "file::memory:", want: "", wantOK: false},
		{name: "bareMemory", dsn: ":memory:", want: "", wantOK: false},
		{name: "namedMemory", dsn: "file:data.db?cache=shared&mode=memory", want: "", wantOK: false},
		{name: "empty", dsn: "", want: "", wantOK: false},
		{name: "subDir", dsn: "file:tide/data.db", want: "tide", wantOK: true},
		{name: "absPath", dsn: `file:/home/pi/tide/data.db?_journal_mode=WAL`, want: `/home/pi/tide`, wantOK: true},
		{name: "rootDir", dsn: `file:/data.db`, want: `/`, wantOK: true},
		{name: "windowsPath", dsn: `file:C:/tide/data.db`, want: `C:/tide`, wantOK: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, ok := PathFromDSN(tt.dsn)
			if ok != tt.wantOK {
				t.Fatalf("PathFromDSN(%q) ok = %v, want %v", tt.dsn, ok, tt.wantOK)
			}
			// Windows 上 filepath.Dir 返回反斜杠分隔；断言按平台归一化。
			if ok && dir != filepath.FromSlash(tt.want) {
				t.Fatalf("PathFromDSN(%q) dir = %q, want %q", tt.dsn, dir, filepath.FromSlash(tt.want))
			}
		})
	}
}

// TestCleanDBDataCleansOrphanTables 覆盖设备/item 改名或移除后的场景：遗留的旧数据表
// 不在配置清单里，也必须按保留期清理，否则 data.db 只增不减。
func TestCleanDBDataCleansOrphanTables(t *testing.T) {
	require.NoError(t, ResetFixtures())

	// 造一张孤儿表：结构是数据表，但不在本次配置清单里。
	const orphan = "item_orphan_old_name"
	if err := MakeSureTableExist(orphan); err != nil {
		t.Fatalf("create orphan table: %v", err)
	}
	// 测试共用一个内存库，InitData 只重置 item1/item2；孤儿表要自己清掉，否则重复执行会累积行。
	t.Cleanup(func() {
		if _, err := db.Exec("drop table if exists " + orphan); err != nil {
			t.Errorf("drop orphan table: %v", err)
		}
	})
	if err := SaveData(orphan, 1, 100); err != nil {
		t.Fatalf("seed orphan table: %v", err)
	}
	if err := SaveData(orphan, 2, 5000); err != nil {
		t.Fatalf("seed orphan table: %v", err)
	}

	// 清理清单只含 item1/item2（模拟设备变更后的新配置），孤儿表也应被清理。
	if err := CleanDBData(1000, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}

	his, err := GetDataHistory(orphan, 0, 0)
	if err != nil {
		t.Fatalf("GetDataHistory(orphan) returned error: %v", err)
	}
	assert.Equal(t, []common.DataTimeStruct{{Value: 2, Millisecond: 5000}}, his,
		"only the rows newer than the cutoff must survive in the orphan table")
}
