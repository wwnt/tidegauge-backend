package db

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"tide/common"
	"tide/pkg/custype"
	"time"
)

func TestGetDataHistory(t *testing.T) {
	type args struct {
		stationId uuid.UUID
		itemName  string
		start     custype.TimeMillisecond
		end       custype.TimeMillisecond
	}
	tests := []struct {
		name    string
		args    args
		want    []common.DataTimeStruct
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id, itemName: item1.Name, start: 0, end: 3}, want: data, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDataHistory(tt.args.stationId, tt.args.itemName, tt.args.start, tt.args.end)
			if !tt.wantErr(t, err, fmt.Sprintf("GetDataHistory(%v, %v, %v, %v)", tt.args.stationId, tt.args.itemName, tt.args.start, tt.args.end)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetDataHistory(%v, %v, %v, %v)", tt.args.stationId, tt.args.itemName, tt.args.start, tt.args.end)
		})
	}
}

func TestGetItemsLatest(t *testing.T) {
	type args struct {
		stationId   uuid.UUID
		itemsLatest common.StringMsecMap
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id, itemsLatest: map[string]custype.TimeMillisecond{item1.Name: 0}}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, GetItemsLatest(tt.args.stationId, tt.args.itemsLatest), fmt.Sprintf("GetItemsLatest(%v, %v)", tt.args.stationId, tt.args.itemsLatest))
		})
	}
}

func TestGetLatestDataTime(t *testing.T) {
	type args struct {
		stationId uuid.UUID
		itemName  string
	}
	tests := []struct {
		name    string
		args    args
		wantTs  custype.TimeMillisecond
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id, itemName: item1.Name}, wantTs: data[1].Millisecond, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTs, err := GetLatestDataTime(tt.args.stationId, tt.args.itemName)
			if !tt.wantErr(t, err, fmt.Sprintf("GetLatestDataTime(%v, %v)", tt.args.stationId, tt.args.itemName)) {
				return
			}
			assert.Equalf(t, tt.wantTs, gotTs, "GetLatestDataTime(%v, %v)", tt.args.stationId, tt.args.itemName)
		})
	}
}

func TestSaveDataHistory(t *testing.T) {
	type args struct {
		stationId uuid.UUID
		itemName  string
		itemValue float64
		tm        time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "insert already exist", args: args{stationId: station1.Id, itemName: item1.Name, itemValue: data[1].Value, tm: data[1].Millisecond.ToTime()}, want: 0, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SaveDataHistory(tt.args.stationId, tt.args.itemName, tt.args.itemValue, tt.args.tm)
			if !tt.wantErr(t, err, fmt.Sprintf("SaveDataHistory(%v, %v, %v, %v)", tt.args.stationId, tt.args.itemName, tt.args.itemValue, tt.args.tm)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SaveDataHistory(%v, %v, %v, %v)", tt.args.stationId, tt.args.itemName, tt.args.itemValue, tt.args.tm)
		})
	}
}
