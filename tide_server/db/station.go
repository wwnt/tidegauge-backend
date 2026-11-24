package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"tide/common"
	"tide/pkg/custype"

	"github.com/google/uuid"
)

type Station struct {
	Id              uuid.UUID       `json:"id,omitempty"`
	Identifier      string          `json:"identifier,omitempty" binding:"required"`
	Name            string          `json:"name,omitempty"`
	IpAddr          string          `json:"ip_addr,omitempty"`
	Location        json.RawMessage `json:"location,omitempty"`
	Partner         json.RawMessage `json:"partner,omitempty"`
	Cameras         json.RawMessage `json:"cameras,omitempty"`
	Status          common.Status   `json:"status,omitempty"`
	StatusChangedAt custype.UnixMs  `json:"status_changed_at,omitempty"`
	Upstream        bool            `json:"upstream"`
}

// ErrStationIdentifierExists 表示新建本地站点时 identifier 已被另一个未删除的本地站点占用。
var ErrStationIdentifierExists = errors.New("already has the same identifier")

func GetLocalStationIdByIdentifier(identifier string) (uuid.UUID, error) {
	var stationId uuid.UUID
	err := TideDB.QueryRow(`select id from stations where identifier=$1 and upstream=false and deleted_at is null`, identifier).Scan(&stationId)
	return stationId, err
}

func GetStations() ([]Station, error) {
	return queryAll(TideDB, func(rows *sql.Rows, s *Station) error {
		return rows.Scan(&s.Id, &s.Identifier, &s.Name, &s.IpAddr, &s.Location, &s.Partner, &s.Cameras, &s.Status, &s.StatusChangedAt, &s.Upstream)
	}, `select id, identifier, name, ip_addr, location, partner, cameras, status, status_changed_at, upstream from stations where deleted_at is null`)
}

func EditStation(s *Station) (err error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if s.Id == uuid.Nil {
		// create
		var deletedAt any
		if err = tx.QueryRow(`select id, deleted_at from stations where identifier=$1 and upstream=false`, s.Identifier).Scan(&s.Id, &deletedAt); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if s.Id, err = uuid.NewUUID(); err != nil {
				return err
			}
			_, err = tx.Exec(`insert into stations(id,identifier,name,location,partner,upstream) VALUES ($1,$2,$3,coalesce($4,'null'::jsonb),coalesce($5,'null'::jsonb),false)`, s.Id, s.Identifier, s.Name, s.Location, s.Partner)
		} else {
			if deletedAt == nil {
				return ErrStationIdentifierExists
			}
			_, err = tx.Exec(`update stations set name=$2,location=coalesce($3,'null'::jsonb),partner=coalesce($4,'null'::jsonb),upstream=false,deleted_at=null where id=$1`, s.Id, s.Name, s.Location, s.Partner)
		}
	} else {
		// update
		if err = tx.QueryRow("select upstream from stations where id=$1", s.Id).Scan(&s.Upstream); err != nil {
			return err
		}
		if !s.Upstream {
			_, err = tx.Exec(`update stations set name=$2,location=coalesce($3,'null'::jsonb),partner=coalesce($4,'null'::jsonb) where id=$1`, s.Id, s.Name, s.Location, s.Partner)
		} else {
			_, err = tx.Exec(`update stations set name=$2,partner=coalesce($3,'null'::jsonb) where id=$1`, s.Id, s.Name, s.Partner)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SyncStation 按上游发来的内容新建或更新一个上游站点，并登记它与该上游的关系。
//
// 站点 id 来自上游的消息。如果它与一个正在使用的本地站点（upstream=false 且未删除）
// 相同——例如两台服务器互为上游时，本机的站点会被对端原样发回来——本函数什么都不做：
// 既不把本地站点改成上游站点（那样它自己的采集端会被拒之门外，之后删除上游还会把它
// 连同数据一起删掉），也不登记关联。
func SyncStation(upstreamId int, s Station) (int64, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`insert into stations(id,identifier,name,location,upstream) VALUES ($1,$2,$3,coalesce($4,'null'::jsonb),true) on conflict (id)
do update set location=excluded.location, upstream=true, deleted_at=null
where (stations.upstream=true or stations.deleted_at is not null)
  and (stations.location!=excluded.location or stations.upstream!=true or stations.deleted_at is not null)`,
		s.Id, s.Identifier, s.Name, s.Location)
	n, err := checkResult(res, err)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(`insert into upstream_stations(upstream_id, station_id)
select $1, id from stations where id=$2 and upstream=true on conflict do nothing`, upstreamId, s.Id)
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
		tx.Exec("update stations set deleted_at=now() where id = $1 and upstream=false", id),
	)
	if err != nil || n == 0 {
		return n, err
	}
	if _, err = tx.Exec("delete from items   where station_id = $1", id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("delete from devices where station_id = $1", id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("delete from item_status_log where station_id = $1", id); err != nil {
		return 0, err
	}
	err = tx.Commit()
	return n, err
}

// DelUpstreamStation delete this station from upstream_stations, and if it does not belong to any other upstream, delete it completely
//
// 只会删除 upstream=true 的站点（与 DelUpstream 的条件一致）：stationId 来自上游发来的
// 消息，不能让一条带着本地站点 UUID 的删除事件把本地站点连同设备、数据项和状态日志级联删掉。
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
		tx.Exec(`delete from stations where id=$1 and upstream=true and id not in (select station_id from upstream_stations)`, stationId),
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
