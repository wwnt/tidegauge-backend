package db

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"tide/common"
	"time"
)

func TestGetItemStatusLogs(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
		after     int64
	}
	tests := []struct {
		name    string
		args    args
		want    []common.RowIdItemStatusStruct
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id, after: 0}, want: station1StatusLogs, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetItemStatusLogs(tt.args.stationId, tt.args.after)
			if !tt.wantErr(t, err, fmt.Sprintf("GetItemStatusLogs(%v, %v)", tt.args.stationId, tt.args.after)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetItemStatusLogs(%v, %v)", tt.args.stationId, tt.args.after)
		})
	}
}

func TestGetLatestStatusLogRowId(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id}, want: station1StatusLogs[len(station1StatusLogs)-1].RowId, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetLatestStatusLogRowId(tt.args.stationId)
			if !tt.wantErr(t, err, fmt.Sprintf("GetLatestStatusLogRowId(%v)", tt.args.stationId)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetLatestStatusLogRowId(%v)", tt.args.stationId)
		})
	}
}

func TestPagedItemStatusLogs(t *testing.T) {
	initData(t)
	type args struct {
		pageNum  uint
		pageSize uint
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{pageNum: 1, pageSize: 10}, want: pagedItemStatusLogStruct{Total: len(station1StatusLogs), Data: []common.StationIdItemStatusStruct{
			{StationId: station1.Id, ItemStatusStruct: station1StatusLogs[0].ItemStatusStruct},
			{StationId: station1.Id, ItemStatusStruct: station1StatusLogs[1].ItemStatusStruct},
		}}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PagedItemStatusLogs(tt.args.pageNum, tt.args.pageSize)
			if !tt.wantErr(t, err, fmt.Sprintf("PagedItemStatusLogs(%v, %v)", tt.args.pageNum, tt.args.pageSize)) {
				return
			}
			assert.Equalf(t, tt.want, got, "PagedItemStatusLogs(%v, %v)", tt.args.pageNum, tt.args.pageSize)
		})
	}
}

func TestSaveItemStatusLog(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
		rowId     int64
		itemName  string
		status    string
		changedAt time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "save", args: args{stationId: station1.Id, rowId: station1StatusLogs[len(station1StatusLogs)-1].RowId + 1, itemName: item1.Name, status: common.Normal, changedAt: time.Now()}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SaveItemStatusLog(tt.args.stationId, tt.args.rowId, tt.args.itemName, tt.args.status, tt.args.changedAt)
			if !tt.wantErr(t, err, fmt.Sprintf("SaveItemStatusLog(%v, %v, %v, %v, %v)", tt.args.stationId, tt.args.rowId, tt.args.itemName, tt.args.status, tt.args.changedAt)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SaveItemStatusLog(%v, %v, %v, %v, %v)", tt.args.stationId, tt.args.rowId, tt.args.itemName, tt.args.status, tt.args.changedAt)
		})
	}
}

func TestUpdateAndSaveStatusLog(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
		rowId     int64
		itemName  string
		status    string
		changedAt time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{stationId: station1.Id, rowId: station1StatusLogs[len(station1StatusLogs)-1].RowId + 1, itemName: item1.Name, status: common.Normal, changedAt: time.Now()}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UpdateAndSaveStatusLog(tt.args.stationId, tt.args.rowId, tt.args.itemName, tt.args.status, tt.args.changedAt)
			if !tt.wantErr(t, err, fmt.Sprintf("UpdateAndSaveStatusLog(%v, %v, %v, %v, %v)", tt.args.stationId, tt.args.rowId, tt.args.itemName, tt.args.status, tt.args.changedAt)) {
				return
			}
			assert.Equalf(t, tt.want, got, "UpdateAndSaveStatusLog(%v, %v, %v, %v, %v)", tt.args.stationId, tt.args.rowId, tt.args.itemName, tt.args.status, tt.args.changedAt)
		})
	}
}

func TestUpdateItemStatus(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
		itemName  string
		status    common.Status
		changeAt  time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{stationId: station1.Id, itemName: item1.Name, status: common.NoStatus, changeAt: time.Now()}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UpdateItemStatus(tt.args.stationId, tt.args.itemName, tt.args.status, tt.args.changeAt)
			if !tt.wantErr(t, err, fmt.Sprintf("UpdateItemStatus(%v, %v, %v, %v)", tt.args.stationId, tt.args.itemName, tt.args.status, tt.args.changeAt)) {
				return
			}
			assert.Equalf(t, tt.want, got, "UpdateItemStatus(%v, %v, %v, %v)", tt.args.stationId, tt.args.itemName, tt.args.status, tt.args.changeAt)
		})
	}
}

func TestUpdateStationStatus(t *testing.T) {
	initData(t)
	type args struct {
		id        uuid.UUID
		status    common.Status
		changedAt time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{id: station1.Id, status: common.Disconnected, changedAt: time.Now()}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UpdateStationStatus(tt.args.id, tt.args.status, tt.args.changedAt)
			if !tt.wantErr(t, err, fmt.Sprintf("UpdateStationStatus(%v, %v, %v)", tt.args.id, tt.args.status, tt.args.changedAt)) {
				return
			}
			assert.Equalf(t, tt.want, got, "UpdateStationStatus(%v, %v, %v)", tt.args.id, tt.args.status, tt.args.changedAt)
		})
	}
}
