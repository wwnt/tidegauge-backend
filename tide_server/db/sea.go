package db

import (
	"errors"
	"tide/common"
)

func GetSeaLevel() (any, error) {
	rows, err := TideDB.Query("select code, lat, lon, level from station_sea_level")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type seaLevel struct {
		Code  string     `json:"name"`
		Value [3]float64 `json:"value"`
	}
	var (
		s  seaLevel
		ss []seaLevel
	)
	for rows.Next() {
		err = rows.Scan(&s.Code, &s.Value[1], &s.Value[0], &s.Value[2])
		if err != nil {
			return nil, err
		}
		if s.Value[2] != -999 {
			ss = append(ss, s)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ss, err
}

type StationSeaLevel struct {
	Code  string
	Lat   float64
	Lon   float64
	Level float64
}

func UpdateSeaLevel(stationsLevel []StationSeaLevel) error {
	tx, err := TideDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec("truncate table station_sea_level")
	if err != nil {
		return err
	}
	for _, level := range stationsLevel {
		_, err = tx.Exec(`insert into station_sea_level (code, lat, lon, level) VALUES ($1,$2,$3,$4)`, level.Code, level.Lat, level.Lon, level.Level)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type SateAltimetry struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	SeaLevel float64 `json:"seaLevel"`
}

func GetSateAltimetry(tn string) (any, error) {
	if common.ContainsIllegalCharacter(tn) {
		return nil, errors.New("Table name contains illegal characters: " + tn)
	}
	rows, err := seaDB.Query(`select` + ` lat, lon, sealevel from "` + tn + `"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		s  SateAltimetry
		ss []SateAltimetry
	)
	for rows.Next() {
		err = rows.Scan(&s.Lat, &s.Lon, &s.SeaLevel)
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

type GlossStation struct {
	Id              int     `json:"@_id" xml:"id,attr"`
	Name            string  `json:"name" xml:"name"`
	Country         string  `json:"country" xml:"country"`
	Latitude        float64 `json:"latitude" xml:"latitude"`
	Longitude       float64 `json:"longitude" xml:"longitude"`
	LatestPsmsl     string  `json:"latestPsmsl" xml:"latestPsmsl"`
	LatestPsmslRlr  string  `json:"latestPsmslRlr" xml:"latestPsmslRlr"`
	LatestBodc      string  `json:"latestBodc" xml:"latestBodc"`
	LatestSonel     string  `json:"latestSonel" xml:"latestSonel"`
	LatestJasl      string  `json:"latestJasl" xml:"latestJasl"`
	LatestUhslcFast string  `json:"latestUhslcFast" xml:"latestUhslcFast"`
	LatestVliz      string  `json:"latestVliz" xml:"latestVliz"`
}

type Gloss struct {
	IOCCode     *string `json:"IOC code"`
	PSMSLNumber *int    `json:"PSMSL number"`
}

func GetGlossData() (any, error) {
	rows, err := TideDB.Query(`SELECT id, name, sga.country, latitude, longitude, latest_psmsl, latest_psmsl_rlr, latest_bodc, latest_sonel, latest_jasl, latest_uhslc_fast, latest_vliz, ioc_code, psmsl_number
FROM station_info_gloss_all sga left join station_info_gloss on sga.name = station_info_gloss.station`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type glossData struct {
		Gloss
		GlossStation
	}
	var (
		s  glossData
		ss []glossData
	)
	for rows.Next() {
		err = rows.Scan(&s.Id, &s.Name, &s.Country, &s.Latitude, &s.Longitude, &s.LatestPsmsl, &s.LatestPsmslRlr, &s.LatestBodc, &s.LatestSonel, &s.LatestJasl, &s.LatestUhslcFast, &s.LatestVliz, &s.IOCCode, &s.PSMSLNumber)
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

func UpdateStationInfoGlossAll(stations []GlossStation) error {
	tx, err := TideDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec("truncate table station_info_gloss_all")
	if err != nil {
		return err
	}
	for _, station := range stations {
		_, err = tx.Exec(`insert into station_info_gloss_all (id, name, country, latitude, longitude, latest_psmsl, latest_psmsl_rlr, latest_bodc, latest_sonel, latest_jasl, latest_uhslc_fast, latest_vliz)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, station.Id, station.Name, station.Country, station.Latitude, station.Longitude, station.LatestPsmsl, station.LatestPsmslRlr, station.LatestBodc, station.LatestSonel, station.LatestJasl, station.LatestUhslcFast, station.LatestVliz)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type SonelData struct {
	StaId   int      `json:"sta_id"`
	StaName *string  `json:"sta_name"`
	StaAcro *string  `json:"sta_acro"`
	StaType *string  `json:"sta_type"`
	StaEtat *int     `json:"sta_etat"`
	StaPays *string  `json:"sta_pays"`
	StaLon  *float64 `json:"sta_lon"`
	StaLat  *float64 `json:"sta_lat"`
}

func GetSonelData() (any, error) {
	rows, err := TideDB.Query(`select sta_id, sta_name, sta_acro, sta_type, sta_etat, sta_pays, sta_lon, sta_lat from station_info_tide`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		d  SonelData
		ds []SonelData
	)
	for rows.Next() {
		err = rows.Scan(&d.StaId, &d.StaName, &d.StaAcro, &d.StaType, &d.StaEtat, &d.StaPays, &d.StaLon, &d.StaLat)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, err
}

type PsmslData struct {
	Id          int      `json:"id"`
	StationName *string  `json:"Station Name"`
	Lat         *float64 `json:"Lat"`
	Lon         *float64 `json:"Lon"`
	Coastline   *int     `json:"Coastline"`
	Station     *int     `json:"Station"`
	GLOSSID     *int     `json:"GLOSS ID"`
	Country     *string  `json:"Country"`
	Date        *string  `json:"Date"`
}

func GetPsmslData() (any, error) {
	rows, err := TideDB.Query(`select station_name, id, lat, lon, gloss_id, country, date, coastline, station from station_info_psmsl`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		d  PsmslData
		ds []PsmslData
	)
	for rows.Next() {
		err = rows.Scan(&d.StationName, &d.Id, &d.Lat, &d.Lon, &d.GLOSSID, &d.Country, &d.Date, &d.Coastline, &d.Station)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ds, err
}
