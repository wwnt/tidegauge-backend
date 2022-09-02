package db

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"tide/common"
)

func TestDelItem(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
		name      string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "del", args: args{stationId: station1.Id, name: item1.Name}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DelItem(tt.args.stationId, tt.args.name)
			if !tt.wantErr(t, err, fmt.Sprintf("DelItem(%v, %v)", tt.args.stationId, tt.args.name)) {
				return
			}
			assert.Equalf(t, tt.want, got, "DelItem(%v, %v)", tt.args.stationId, tt.args.name)
		})
	}
}

func TestEditItem(t *testing.T) {
	initData(t)
	type args struct {
		i Item
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update same", args: args{i: item1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, EditItem(tt.args.i), fmt.Sprintf("EditItem(%v)", tt.args.i))
		})
	}
}

func TestGetAvailableItems(t *testing.T) {
	initData(t)
	tests := []struct {
		name    string
		want    []common.StationItemStruct
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", want: []common.StationItemStruct{{StationId: station1.Id, ItemName: item1.Name}}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetAvailableItems()
			if !tt.wantErr(t, err, fmt.Sprintf("GetAvailableItems()")) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetAvailableItems()")
		})
	}
}

func TestGetItems(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    []Item
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id}, want: []Item{item1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetItems(tt.args.stationId)
			if !tt.wantErr(t, err, fmt.Sprintf("GetItems(%v)", tt.args.stationId)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetItems(%v)", tt.args.stationId)
		})
	}
}

func TestMakeSureTableExist(t *testing.T) {
	initData(t)
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "new", args: args{name: "test_table_name"}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, MakeSureTableExist(tt.args.name), fmt.Sprintf("MakeSureTableExist(%v)", tt.args.name))
		})
	}
}

func TestRemoveAllAvailable(t *testing.T) {
	initData(t)
	type args struct {
		upstreamId int
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "remove", args: args{upstreamId: upstream1.Id}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, RemoveAllAvailable(tt.args.upstreamId), fmt.Sprintf("RemoveAllAvailable(%v)", tt.args.upstreamId))
		})
	}
}

func TestSyncItem(t *testing.T) {
	initData(t)
	type args struct {
		i Item
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "sync", args: args{i: upstream1Item1}, want: 0, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SyncItem(tt.args.i)
			if !tt.wantErr(t, err, fmt.Sprintf("SyncItem(%v)", tt.args.i)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SyncItem(%v)", tt.args.i)
		})
	}
}

func TestUpdateAvailableItems(t *testing.T) {
	initData(t)
	type args struct {
		upstreamId int
		newAvail   common.UUIDStringsMap
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{upstreamId: upstream1.Id, newAvail: map[uuid.UUID][]string{upstream1Station1.Id: {item1.Name}}}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UpdateAvailableItems(tt.args.upstreamId, tt.args.newAvail)
			if !tt.wantErr(t, err, fmt.Sprintf("UpdateAvailableItems(%v, %v)", tt.args.upstreamId, tt.args.newAvail)) {
				return
			}
			assert.Equalf(t, tt.want, got, "UpdateAvailableItems(%v, %v)", tt.args.upstreamId, tt.args.newAvail)
		})
	}
}
