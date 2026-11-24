package db

import (
	"database/sql"
	"encoding/json"
	"tide/pkg/custype"
	"time"

	"github.com/google/uuid"
)

type (
	Device struct {
		StationId       uuid.UUID       `json:"station_id" binding:"required"`
		Name            string          `json:"name" binding:"required"`
		Specs           json.RawMessage `json:"specs"`
		LastMaintenance custype.UnixMs  `json:"last_maintenance"`
	}
	DeviceRecord struct {
		Id         uuid.UUID      `json:"id"`
		StationId  uuid.UUID      `json:"station_id" binding:"required"`
		DeviceName string         `json:"device_name" binding:"required"`
		Record     string         `json:"record"`
		CreatedAt  custype.UnixMs `json:"created_at"`
		UpdatedAt  custype.UnixMs `json:"updated_at"`
		Version    int            `json:"version,omitempty"`
	}
)

func GetDevices(stationId uuid.UUID) ([]Device, error) {
	scan := func(rows *sql.Rows, d *Device) error {
		return rows.Scan(&d.StationId, &d.Name, &d.Specs, &d.LastMaintenance)
	}
	if stationId == uuid.Nil {
		return queryAll(TideDB, scan, "select station_id, name, specs, last_maintenance from devices")
	}
	return queryAll(TideDB, scan, "select station_id, name, specs, last_maintenance from devices where station_id=$1", stationId)
}

func EditDevice(d Device) (err error) {
	if d.Specs == nil && d.LastMaintenance == 0 {
		_, err = TideDB.Exec(`insert into devices(station_id,name) VALUES ($1,$2) on conflict do nothing`, d.StationId, d.Name)
	} else {
		_, err = TideDB.Exec(`update devices set specs=$3, last_maintenance=$4 where station_id=$1 and name=$2`,
			d.StationId, d.Name, d.Specs, d.LastMaintenance)
	}
	return err
}

// SyncDevice 按上游发来的内容新建或更新设备。
// specs 为空（nil）时按 JSON null 处理：列是 NOT NULL 的 jsonb，而 pgx 会把 nil 的
// json.RawMessage 编码成 SQL NULL，直接绑定会违反约束。
func SyncDevice(d Device) (int64, error) {
	res, err := TideDB.Exec(`insert into devices(station_id,name,specs,last_maintenance) VALUES ($1,$2,coalesce($3,'null'::jsonb),$4) on conflict (station_id,name) do
update set specs=excluded.specs, last_maintenance=excluded.last_maintenance where devices.specs!=excluded.specs or devices.last_maintenance!=excluded.last_maintenance`,
		d.StationId, d.Name, d.Specs, d.LastMaintenance)
	return checkResult(res, err)
}

func DelDevice(stationId uuid.UUID, deviceName string) (int64, error) {
	var (
		err error
		res sql.Result
	)
	if deviceName == "" {
		res, err = TideDB.Exec("delete from devices where station_id=$1", stationId)
	} else {
		res, err = TideDB.Exec("delete from devices where station_id=$1 and name=$2", stationId, deviceName)
	}
	return checkResult(res, err)
}

func GetDeviceRecords() ([]DeviceRecord, error) {
	return queryAll(TideDB, func(rows *sql.Rows, dr *DeviceRecord) error {
		return rows.Scan(&dr.Id, &dr.StationId, &dr.DeviceName, &dr.Record, &dr.CreatedAt, &dr.UpdatedAt, &dr.Version)
	}, `select id, station_id, device_name, record, created_at, updated_at, version from device_record
where station_id in (select id from stations where deleted_at is null)`)
}

func EditDeviceRecord(dr *DeviceRecord) (err error) {
	if dr.Id == uuid.Nil {
		if dr.Id, err = uuid.NewUUID(); err != nil {
			return err
		}
		dr.CreatedAt = custype.ToUnixMs(time.Now())
		dr.UpdatedAt = dr.CreatedAt
		dr.Version = 1
		_, err = TideDB.Exec("insert into device_record(id,station_id,device_name,record,created_at,updated_at,upstream_version,version) VALUES ($1,$2,$3,$4,$5,$5,0,1)",
			dr.Id, dr.StationId, dr.DeviceName, dr.Record, dr.CreatedAt)
	} else {
		dr.UpdatedAt = custype.ToUnixMs(time.Now())
		err = TideDB.QueryRow("update device_record set record=$2, updated_at=$3, version=version+1 where id=$1 returning version", dr.Id, dr.Record, dr.UpdatedAt).Scan(&dr.Version)
	}
	return err
}

func SyncDeviceRecord(dr DeviceRecord) (int64, error) {
	res, err := TideDB.Exec(`insert into device_record(id, station_id, device_name, record, created_at, updated_at, upstream_version, version) VALUES ($1,$2,$3,$4,$5,$6,$7,1) on conflict (id) do
update set record=excluded.record, updated_at=excluded.updated_at, upstream_version=excluded.upstream_version, version=device_record.version+1 where device_record.upstream_version != $7`,
		dr.Id, dr.StationId, dr.DeviceName, dr.Record, dr.CreatedAt, dr.UpdatedAt, dr.Version)
	return checkResult(res, err)
}
