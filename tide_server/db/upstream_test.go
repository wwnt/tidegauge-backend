package db

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDelUpstream(t *testing.T) {
	initData(t)
	type args struct {
		id int
	}
	tests := []struct {
		name    string
		args    args
		want    []uuid.UUID
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "del1", args: args{id: upstream1.Id}, want: nil, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DelUpstream(tt.args.id)
			if !tt.wantErr(t, err, fmt.Sprintf("DelUpstream(%v)", tt.args.id)) {
				return
			}
			assert.Equalf(t, tt.want, got, "DelUpstream(%v)", tt.args.id)
		})
	}
}

func TestEditUpstream(t *testing.T) {
	initData(t)
	type args struct {
		up *Upstream
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{up: &upstream1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, EditUpstream(tt.args.up), fmt.Sprintf("EditUpstream(%v)", tt.args.up))
		})
	}
}

func TestGetStationsByUpstreamId(t *testing.T) {
	initData(t)
	upstream1Station1.Upstream = false
	type args struct {
		upstreamId int
	}
	tests := []struct {
		name    string
		args    args
		want    []Station
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{upstreamId: upstream1.Id}, want: []Station{upstream1Station1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetStationsByUpstreamId(tt.args.upstreamId)
			if !tt.wantErr(t, err, fmt.Sprintf("GetStationsByUpstreamId(%v)", tt.args.upstreamId)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetStationsByUpstreamId(%v)", tt.args.upstreamId)
		})
	}
}

func TestGetUpstreams(t *testing.T) {
	initData(t)
	tests := []struct {
		name    string
		want    []Upstream
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", want: []Upstream{upstream1, upstream2}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUpstreams()
			if !tt.wantErr(t, err, fmt.Sprintf("GetUpstreams()")) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetUpstreams()")
		})
	}
}

func TestGetUpstreamsByStationId(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    []Upstream
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: upstream1Station1.Id}, want: []Upstream{upstream1, upstream2}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUpstreamsByStationId(tt.args.stationId)
			if !tt.wantErr(t, err, fmt.Sprintf("GetUpstreamsByStationId(%v)", tt.args.stationId)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetUpstreamsByStationId(%v)", tt.args.stationId)
		})
	}
}

func TestIsUpstreamStation(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
	}
	tests := []struct {
		name         string
		args         args
		wantUpstream bool
		wantErr      assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: upstream1Station1.Id}, wantUpstream: true, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUpstream, err := IsUpstreamStation(tt.args.stationId)
			if !tt.wantErr(t, err, fmt.Sprintf("IsUpstreamStation(%v)", tt.args.stationId)) {
				return
			}
			assert.Equalf(t, tt.wantUpstream, gotUpstream, "IsUpstreamStation(%v)", tt.args.stationId)
		})
	}
}
