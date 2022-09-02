package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"tide/common"
	"tide/pkg/custype"
)

type Station struct {
	Id              uuid.UUID               `json:"id,omitempty"`
	Identifier      string                  `json:"identifier,omitempty" binding:"required"`
	Name            string                  `json:"name,omitempty"`
	IpAddr          string                  `json:"ip_addr,omitempty"`
	Location        json.RawMessage         `json:"location,omitempty"`
	Partner         json.RawMessage         `json:"partner,omitempty"`
	Cameras         json.RawMessage         `json:"cameras,omitempty"`
	Status          common.Status           `json:"status,omitempty"`
	StatusChangedAt custype.TimeMillisecond `json:"status_changed_at,omitempty"`
	Upstream        bool                    `json:"upstream"`
}

func GetLocalStationIdByIdentifier(identifier string) (uuid.UUID, error) {
	var stationId uuid.UUID
	err := TideDB.QueryRow(`select id from stations where identifier=$1 and upstream=false and deleted_at is null`, identifier).Scan(&stationId)
	return stationId, err
}

func GetStations() ([]Station, error) {
	var (
		rows *sql.Rows
		err  error
	)
	rows, err = TideDB.Query(`select id, identifier, name, ip_addr, location, partner, cameras, status, status_changed_at, upstream from stations where deleted_at is null`)
	if err != nil {
		return nil, err
	}
	var (
		s  Station
		ss []Station
	)
	for rows.Next() {
		err = rows.Scan(&s.Id, &s.Identifier, &s.Name, &s.IpAddr, &s.Location, &s.Partner, &s.Cameras, &s.Status, &s.StatusChangedAt, &s.Upstream)
		if err != nil {
			return nil, err
		}
		ss = append(ss, s)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ss, err
}

func EditStation(s *Station) (err error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if s.Id == uuid.Nil {
		// create
		var deletedAt interface{}
		if err = tx.QueryRow(`select id, deleted_at from stations where identifier=$1 and upstream=false`, s.Identifier).Scan(&s.Id, &deletedAt); err != nil {
			if err != sql.ErrNoRows {
				return err
			}
			if s.Id, err = uuid.NewUUID(); err != nil { //not found
				return err
			}
			_, err = tx.Exec(`insert into stations(id,identifier,name,location,partner,upstream) VALUES ($1,$2,$3,$4,$5,false)`, s.Id, s.Identifier, s.Name, s.Location, s.Partner)
		} else {
			if deletedAt == nil {
				return errors.New("already has the same identifier")
			}
			_, err = tx.Exec(`update stations set name=$2,location=$3,partner=$4,deleted_at=null where id=$1`, s.Id, s.Name, s.Location, s.Partner)
		}
	} else {
		// update
		if err = tx.QueryRow("select upstream from stations where id=$1", s.Id).Scan(&s.Upstream); err != nil {
			return err
		}
		if s.Upstream == false {
			_, err = tx.Exec(`update stations set name=$2,location=$3,partner=$4 where id=$1`, s.Id, s.Name, s.Location, s.Partner)
		} else {
			_, err = tx.Exec(`update stations set name=$2,partner=$3 where id=$1`, s.Id, s.Name, s.Partner)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func SyncStation(upstreamId int, s Station) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`insert into upstream_stations(upstream_id, station_id) VALUES ($1,$2) on conflict do nothing`, upstreamId, s.Id)
	if err != nil {
		return 0, err
	}

	res, err := tx.Exec(`insert into stations(id,identifier,name,location,upstream) VALUES ($1,$2,$3,$4,true) on conflict (id) 
do update set location=excluded.location, upstream=true, deleted_at=null where stations.location!=$4 or stations.upstream!=true or stations.deleted_at is not null`,
		s.Id, s.Identifier, s.Name, s.Location)
	n, err := checkResult(res, err)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

func SyncStationCannotEdit(id uuid.UUID, cameras json.RawMessage) (int64, error) {
	res, err := TideDB.Exec("update stations set cameras=$2 where id=$1 and (cameras!=$2)", id, cameras)
	return checkResult(res, err)
}

func EditStationNotSync(id uuid.UUID, ip string) (err error) {
	_, err = TideDB.Exec("update stations set ip_addr=$2 where id=$1", id, ip)
	return err
}

func DelLocalStation(id uuid.UUID) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	n, err := checkResult(
		tx.Exec("update stations set deleted_at=now() where id=$1 and upstream=false", id),
	)
	if err != nil || n == 0 {
		return n, err
	}
	if _, err = tx.Exec("delete from items   where station_id=$1", id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("delete from devices where station_id=$1", id); err != nil {
		return 0, err
	}
	err = tx.Commit()
	return n, err
}

// DelUpstreamStation delete this station from upstream_stations, and if it does not belong to any other upstream, delete it completely
func DelUpstreamStation(upstreamId int, stationId uuid.UUID) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	_, err = tx.Exec(`delete from upstream_stations where upstream_id=$1 and station_id=$2`, upstreamId, stationId)
	if err != nil {
		return 0, err
	}
	n, err := checkResult(
		tx.Exec(`delete from stations where id=$1 and id not in (select station_id from upstream_stations)`, stationId),
	)
	if err != nil {
		return 0, err
	}
	err = tx.Commit()
	return n, err
}

type StationFullInfo struct {
	Station
	Items   []Item   `json:"items,omitempty"`
	Devices []Device `json:"devices,omitempty"`
}

func GetStationsFullInfo() ([]StationFullInfo, error) {
	ss, err := GetStations()
	if err != nil {
		return nil, err
	}
	var stationsFull []StationFullInfo
	for _, s := range ss {
		ds, err := GetDevices(s.Id)
		if err != nil {
			return nil, err
		}
		is, err := GetItems(s.Id)
		if err != nil {
			return nil, err
		}
		stationsFull = append(stationsFull, StationFullInfo{Station: s, Items: is, Devices: ds})
	}
	return stationsFull, err
}
