package db

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDelLocalStation(t *testing.T) {
	initData(t)
	type args struct {
		id uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "del", args: args{id: station1.Id}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DelLocalStation(tt.args.id)
			if !tt.wantErr(t, err, fmt.Sprintf("DelLocalStation(%v)", tt.args.id)) {
				return
			}
			assert.Equalf(t, tt.want, got, "DelLocalStation(%v)", tt.args.id)
		})
	}
}

func TestDelUpstreamStation(t *testing.T) {
	initData(t)
	type args struct {
		upstreamId int
		stationId  uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "del", args: args{upstreamId: upstream1.Id, stationId: station1.Id}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DelUpstreamStation(tt.args.upstreamId, tt.args.stationId)
			if !tt.wantErr(t, err, fmt.Sprintf("DelUpstreamStation(%v, %v)", tt.args.upstreamId, tt.args.stationId)) {
				return
			}
			assert.Equalf(t, tt.want, got, "DelUpstreamStation(%v, %v)", tt.args.upstreamId, tt.args.stationId)
		})
	}
}

func TestEditStation(t *testing.T) {
	initData(t)
	type args struct {
		s *Station
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{s: &station1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, EditStation(tt.args.s), fmt.Sprintf("EditStation(%v)", tt.args.s))
		})
	}
}

func TestEditStationNoSync(t *testing.T) {
	initData(t)
	type args struct {
		id uuid.UUID
		ip string
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{id: station1.Id, ip: station1.IpAddr}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, EditStationNotSync(tt.args.id, tt.args.ip), fmt.Sprintf("EditStationNotSync(%v, %v)", tt.args.id, tt.args.ip))
		})
	}
}

func TestGetLocalStationIdByIdentifier(t *testing.T) {
	initData(t)
	type args struct {
		identifier string
	}
	tests := []struct {
		name    string
		args    args
		want    uuid.UUID
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{identifier: station1.Identifier}, want: station1.Id, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetLocalStationIdByIdentifier(tt.args.identifier)
			if !tt.wantErr(t, err, fmt.Sprintf("GetLocalStationIdByIdentifier(%v)", tt.args.identifier)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetLocalStationIdByIdentifier(%v)", tt.args.identifier)
		})
	}
}

func TestGetStations(t *testing.T) {
	initData(t)
	tests := []struct {
		name    string
		want    []Station
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", want: []Station{station1, upstream1Station1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetStations()
			if !tt.wantErr(t, err, fmt.Sprintf("GetStations()")) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetStations()")
		})
	}
}

func TestGetStationsFullInfo(t *testing.T) {
	initData(t)
	tests := []struct {
		name    string
		want    []StationFullInfo
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", want: []StationFullInfo{{
			Station: station1,
			Items:   []Item{item1},
			Devices: []Device{device1},
		}, {
			Station: upstream1Station1,
			Items:   []Item{upstream1Item1},
			Devices: []Device{upstream1Device1},
		}}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetStationsFullInfo()
			if !tt.wantErr(t, err, fmt.Sprintf("GetStationsFullInfo()")) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetStationsFullInfo()")
		})
	}
}

func TestSyncStation(t *testing.T) {
	initData(t)
	type args struct {
		upstreamId int
		s          Station
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "sync", args: args{upstreamId: upstream1.Id, s: upstream1Station1}, want: 0, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SyncStation(tt.args.upstreamId, tt.args.s)
			if !tt.wantErr(t, err, fmt.Sprintf("SyncStation(%v, %v)", tt.args.upstreamId, tt.args.s)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SyncStation(%v, %v)", tt.args.upstreamId, tt.args.s)
		})
	}
}

func TestSyncStationNoEdit(t *testing.T) {
	initData(t)
	type args struct {
		id      uuid.UUID
		cameras json.RawMessage
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "sync", args: args{id: upstream1Station1.Id, cameras: upstream1Station1.Cameras}, want: 0, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SyncStationCannotEdit(tt.args.id, tt.args.cameras)
			if !tt.wantErr(t, err, fmt.Sprintf("SyncStationCannotEdit(%v, %v)", tt.args.id, tt.args.cameras)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SyncStationCannotEdit(%v, %s)", tt.args.id, tt.args.cameras)
		})
	}
}
