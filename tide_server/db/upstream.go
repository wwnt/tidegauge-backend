package db

import "github.com/google/uuid"

type Upstream struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
	Url      string `json:"url"`
}

func GetUpstreams() ([]Upstream, error) {
	rows, err := TideDB.Query(`select id, username, password, url from upstreams`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		u  Upstream
		us []Upstream
	)
	for rows.Next() {
		err = rows.Scan(&u.Id, &u.Username, &u.Password, &u.Url)
		if err != nil {
			return nil, err
		}
		us = append(us, u)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return us, err
}

// EditUpstream 新建（Id==0）或修改上游。修改时 Password 为空表示沿用已保存的密码；
// 返回时 up.Password 是实际生效的密码，调用方可以直接用它发起同步。
func EditUpstream(up *Upstream) (err error) {
	if up.Id == 0 {
		err = TideDB.QueryRow(
			`insert into upstreams (username, password, url) values ($1,$2,$3) returning id`,
			up.Username, up.Password, up.Url).Scan(&up.Id)
	} else {
		err = TideDB.QueryRow(
			`update upstreams set username=$1, password=case when $2='' then password else $2 end, url=$3 where id=$4 returning password`,
			up.Username, up.Password, up.Url, up.Id).Scan(&up.Password)
	}
	return err
}

// DelUpstream delete this upstream from upstream_stations，and
// then delete the stations that only belong to this upstream
func DelUpstream(id int) ([]uuid.UUID, error) {
	tx, err := TideDB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	_, err = tx.Exec(`delete from upstream_stations where upstream_id=$1`, id)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(
		`delete from stations where upstream=true and id not in (select station_id from upstream_stations) returning id`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		d  uuid.UUID
		ds []uuid.UUID
	)
	for rows.Next() {
		err = rows.Scan(&d)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	_, err = tx.Exec("delete from upstreams where id=$1", id)
	if err != nil {
		return nil, err
	}
	return ds, tx.Commit()
}

func GetStationsByUpstreamId(upId int) ([]Station, error) {
	rows, err := TideDB.Query(`select id, identifier, name, ip_addr, location, partner, cameras, status, status_changed_at from stations 
inner join upstream_stations on stations.id = upstream_stations.station_id where upstream_stations.upstream_id=$1 and stations.deleted_at is null`, upId)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		s  Station
		ss []Station
	)
	for rows.Next() {
		err = rows.Scan(&s.Id, &s.Identifier, &s.Name, &s.IpAddr, &s.Location, &s.Partner, &s.Cameras, &s.Status, &s.StatusChangedAt)
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

func IsUpstreamStation(stationId uuid.UUID) (upstream bool, err error) {
	err = TideDB.QueryRow("select upstream from stations where id=$1", stationId).Scan(&upstream)
	return upstream, err
}

func GetUpstreamsByStationId(stationId uuid.UUID) ([]Upstream, error) {
	rows, err := TideDB.Query(`select id, username, password, url from upstream_stations join upstreams on upstream_stations.upstream_id = upstreams.id where station_id=$1`, stationId)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		u  Upstream
		us []Upstream
	)
	for rows.Next() {
		err = rows.Scan(&u.Id, &u.Username, &u.Password, &u.Url)
		if err != nil {
			return nil, err
		}
		us = append(us, u)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return us, err
}
