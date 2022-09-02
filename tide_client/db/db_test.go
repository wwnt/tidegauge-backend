package db

import (
	"fmt"
	"github.com/stretchr/testify/assert"
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
