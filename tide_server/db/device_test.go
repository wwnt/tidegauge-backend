package db

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDelDevice(t *testing.T) {
	initData(t)
	type args struct {
		stationId  uuid.UUID
		deviceName string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "del", args: args{stationId: station1.Id, deviceName: device1.Name}, want: 1, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DelDevice(tt.args.stationId, tt.args.deviceName)
			if !tt.wantErr(t, err, fmt.Sprintf("DelDevice(%v, %v)", tt.args.stationId, tt.args.deviceName)) {
				return
			}
			assert.Equalf(t, tt.want, got, "DelDevice(%v, %v)", tt.args.stationId, tt.args.deviceName)
		})
	}
}

func TestEditDevice(t *testing.T) {
	initData(t)
	type args struct {
		d Device
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{d: device1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, EditDevice(tt.args.d), fmt.Sprintf("EditDevice(%v)", tt.args.d))
		})
	}
}

func TestEditDeviceRecord(t *testing.T) {
	initData(t)
	type args struct {
		dr *DeviceRecord
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "update", args: args{dr: &deviceRecord1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, EditDeviceRecord(tt.args.dr), fmt.Sprintf("EditDeviceRecord(%v)", tt.args.dr))
		})
	}
}

func TestGetDeviceRecords(t *testing.T) {
	initData(t)
	tests := []struct {
		name    string
		want    []DeviceRecord
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", want: []DeviceRecord{deviceRecord1, upstream1DeviceRecord1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDeviceRecords()
			if !tt.wantErr(t, err, fmt.Sprintf("GetDeviceRecords()")) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetDeviceRecords()")
		})
	}
}

func TestGetDevices(t *testing.T) {
	initData(t)
	type args struct {
		stationId uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    []Device
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "get", args: args{stationId: station1.Id}, want: []Device{device1}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDevices(tt.args.stationId)
			if !tt.wantErr(t, err, fmt.Sprintf("GetDevices(%v)", tt.args.stationId)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetDevices(%v)", tt.args.stationId)
		})
	}
}

func TestSyncDevice(t *testing.T) {
	initData(t)
	type args struct {
		d Device
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "sync", args: args{device1}, want: 0, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SyncDevice(tt.args.d)
			if !tt.wantErr(t, err, fmt.Sprintf("SyncDevice(%v)", tt.args.d)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SyncDevice(%v)", tt.args.d)
		})
	}
}

func TestSyncDeviceRecord(t *testing.T) {
	initData(t)
	type args struct {
		dr DeviceRecord
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "sync", args: args{upstream1DeviceRecord1}, want: 0, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SyncDeviceRecord(tt.args.dr)
			if !tt.wantErr(t, err, fmt.Sprintf("SyncDeviceRecord(%v)", tt.args.dr)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SyncDeviceRecord(%v)", tt.args.dr)
		})
	}
}
