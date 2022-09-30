package db

import (
	"errors"
	"tide/common"
)

type seaHeight struct {
	Name  string     `json:"name"`
	Value [3]float64 `json:"value"`
}

func GetSeaHeight() (interface{}, error) {
	rows, err := TideDB.Query("select code, lat, lon, sea_height from sea_height_test")
	if err != nil {
		return nil, err
	}
	var (
		s  seaHeight
		ss []seaHeight
	)
	for rows.Next() {
		err = rows.Scan(&s.Name, &s.Value[1], &s.Value[0], &s.Value[2])
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

type SateAltimetry struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	SeaLevel float64 `json:"seaLevel"`
}

func GetSateAltimetry(tn string) (interface{}, error) {
	if common.ContainsIllegalCharacter(tn) {
		return nil, errors.New("Table name contains illegal characters: " + tn)
	}
	rows, err := seaDB.Query("select lat, lon, sealevel from " + tn)
	if err != nil {
		return nil, err
	}
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

type Gloss struct {
	Id              *string `json:"@_id"`
	Name            *string `json:"name"`
	Country         *string `json:"country"`
	Latitude        *string `json:"latitude"`
	Longitude       *string `json:"longitude"`
	LatestPsmsl     *string `json:"latestPsmsl"`
	LatestBodc      *string `json:"latestBodc"`
	LatestSonel     *string `json:"latestSonel"`
	LatestJasl      *string `json:"latestJasl"`
	LatestUhslcFast *string `json:"latestUhslcFast"`
	LatestVliz      *string `json:"latestVliz"`
	LatestPsmslRlr  *string `json:"latestPsmslRlr"`
	IOCCode         *string `json:"IOC code"`
	PSMSLNumber     *int    `json:"PSMSL number"`
}

func GetGlossData() (interface{}, error) {
	rows, err := TideDB.Query(`SELECT "@id", name, sga.country, latitude, longitude, "latestPsmsl", "latestBodc", "latestSonel", "latestJasl", "latestUhslcFast", "latestVliz", "latestPsmslRlr", "IOC code", "PSMSL number" FROM stationinfo_gloss_all sga left join stationinfo_gloss sg on sga.name = sg."Station"`)
	if err != nil {
		return nil, err
	}
	var (
		s  Gloss
		ss []Gloss
	)
	for rows.Next() {
		err = rows.Scan(&s.Id, &s.Name, &s.Country, &s.Latitude, &s.Longitude, &s.LatestPsmsl, &s.LatestBodc, &s.LatestSonel, &s.LatestJasl, &s.LatestUhslcFast, &s.LatestVliz, &s.LatestPsmslRlr, &s.IOCCode, &s.PSMSLNumber)
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

func GetSonelData() (interface{}, error) {
	rows, err := TideDB.Query(`select sta_id, sta_name, sta_acro, sta_type, sta_etat, sta_pays, sta_lon, sta_lat from station_info_tide`)
	if err != nil {
		return nil, err
	}
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
	Coastline   *int     `json:"Coastline"`
	Station     *int     `json:"Station"`
	GLOSSID     *int     `json:"GLOSS ID"`
	Country     *string  `json:"Country"`
	Lon         *float64 `json:"Lon"`
	StationName *string  `json:"Station Name"`
	Lat         *float64 `json:"Lat"`
	Date        *string  `json:"Date"`
}

func GetPsmslData() (interface{}, error) {
	rows, err := TideDB.Query(`select station_name, id, lat, lon, gloss_id, country, date, coastline, station from station_info_psmsl`)
	if err != nil {
		return nil, err
	}
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
