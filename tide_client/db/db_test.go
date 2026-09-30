package db

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
	"tide/common"
)

func TestGetDataHistory(t *testing.T) {
	InitData(t)
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
	InitData(t)
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
	InitData(t)
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
			if !tt.wantErr(t, err, fmt.Sprintf("GetItemsLatestStatus()")) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetItemsLatestStatus()")
		})
	}
}

func TestSaveItemStatusLog(t *testing.T) {
	InitData(t)
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
	InitData(t)
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
	InitData(t)

	// cutoff 早于所有行时不得误删。
	if err := CleanDBData(0, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}
	logs, err := GetItemStatusLogAfter(0)
	if err != nil {
		t.Fatalf("GetItemStatusLogAfter returned error: %v", err)
	}
	assert.Equal(t, StatusLogs, logs, "a cutoff before every row must delete nothing")

	// cutoff 晚于所有行时全删（数据表与状态日志都在清理范围内）。
	if err = CleanDBData(2000, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}
	logs, err = GetItemStatusLogAfter(0)
	if err != nil {
		t.Fatalf("GetItemStatusLogAfter returned error: %v", err)
	}
	assert.Empty(t, logs, "a cutoff after every row must delete everything")

	// 重新播种：cutoff 取两行之间应删旧留新。
	InitData(t)
	if err = CleanDBData(1150, []string{"item1", "item2"}); err != nil {
		t.Fatalf("CleanDBData returned error: %v", err)
	}
	logs, err = GetItemStatusLogAfter(0)
	if err != nil {
		t.Fatalf("GetItemStatusLogAfter returned error: %v", err)
	}
	assert.Equal(t, StatusLogs[1:], logs, "expected only the rows newer than the cutoff")

	// 数据表同样被清理：cutoff 2000 应清空 item1（种子 200ms 那行在 2000 之后）。
	his, err := GetDataHistory("item1", 0, 0)
	if err != nil {
		t.Fatalf("GetDataHistory returned error: %v", err)
	}
	assert.Empty(t, his, "data table rows older than the cutoff must be deleted")
}

func TestCleanDBDataRejectsIllegalItemName(t *testing.T) {
	t.Parallel()

	if err := CleanDBData(0, []string{"item1\"; drop table item1"}); err == nil {
		t.Fatal("CleanDBData accepted an item name with SQL metacharacters")
	}
}

func TestPathFromDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dsn    string
		want   string
		wantOK bool
	}{
		{name: "fileWithParams", dsn: "file:data.db?_journal_mode=WAL&_foreign_keys=on", want: ".", wantOK: false},
		{name: "fileNoParams", dsn: "file:data.db", want: ".", wantOK: false},
		{name: "bareName", dsn: "data.db", want: ".", wantOK: false},
		{name: "memory", dsn: "file::memory:", want: "", wantOK: false},
		{name: "subDir", dsn: "file:tide/data.db", want: "tide", wantOK: true},
		{name: "absPath", dsn: `file:/home/pi/tide/data.db?_journal_mode=WAL`, want: `/home/pi/tide`, wantOK: true},
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
	InitData(t)

	// 造一张孤儿表：结构是数据表，但不在本次配置清单里。
	const orphan = "item_orphan_old_name"
	MakeSureTableExist(orphan)
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
