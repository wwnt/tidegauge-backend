package controller

import (
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"tide/tide_server/db"

	"github.com/PuerkitoBio/goquery"
)

//go:embed sea_stations_pos.json
var stationsPosJson []byte

var stationsPos map[string]struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func init() {
	if err := json.Unmarshal(stationsPosJson, &stationsPos); err != nil {
		slog.Error("Failed to unmarshal sea stations's position json", "error", err)
		os.Exit(1)
	}
}

func seaHeight() {
	resp, err := http.Get("https://www.ioc-sealevelmonitoring.org/list.php?operator=&showall=all&output=general")
	if err != nil {
		slog.Error("Failed to fetch IOC sea level data", "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("HTTP request failed", "status_code", resp.StatusCode)
		return
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		slog.Error("Failed to parse IOC response", "error", err)
		return
	}

	tr := doc.Find("div.maincontent > table > tbody > tr > td > form > table > tbody > tr")
	tr = tr.Slice(3, goquery.ToEnd)
	var data []db.StationSeaLevel
	tr.Each(func(i int, selection *goquery.Selection) {
		td := selection.Children()
		var levelStr = strings.TrimSpace(td.Eq(6).Text())
		if levelStr == "" || levelStr == "-999" {
			return
		}
		level, err := strconv.ParseFloat(levelStr, 64)
		if err != nil {
			return
		}

		var code = strings.TrimSpace(td.Eq(0).Text())
		pos, ok := stationsPos[code]
		if !ok {
			return
		}
		data = append(data, db.StationSeaLevel{Code: code, Lat: pos.Lat, Lon: pos.Lon, Level: level})
	})
	err = db.UpdateSeaLevel(data)
	if err != nil {
		slog.Error("Failed to update sea level data", "error", err)
	}
}

type glossCoreNetwork struct {
	XMLName          xml.Name         `xml:"glossCoreNetwork"`
	ActiveDefinition activeDefinition `xml:"activeDefinition"`
	GlossStations    glossStations    `xml:"glossStations"`
}

type activeDefinition struct {
	XMLName   xml.Name `xml:"activeDefinition"`
	Bodc      string   `xml:"bodc"`
	Psmsl     string   `xml:"psmsl"`
	PsmslRlr  string   `xml:"psmslRlr"`
	Jasl      string   `xml:"jasl"`
	Sonel     string   `xml:"sonel"`
	UhslcFast string   `xml:"uhslcFast"`
	Vliz      string   `xml:"vliz"`
}

type glossStations struct {
	XMLName  xml.Name          `xml:"glossStations"`
	Stations []db.GlossStation `xml:"station"`
}

func stationInfoGlossAll() {
	resp, err := http.Get("https://www.psmsl.org/products/gloss/data/glossCoreNetwork.xml")
	if err != nil {
		slog.Error("Failed to fetch GLOSS core network data", "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return
	}
	var tmp glossCoreNetwork
	err = xml.NewDecoder(resp.Body).Decode(&tmp)
	if err != nil {
		slog.Error("Failed to decode GLOSS XML data", "error", err)
		return
	}
	tmp.GlossStations.Stations = append(tmp.GlossStations.Stations, db.GlossStation{
		Id:              0,
		Name:            "LastestDate",
		Country:         "-",
		Latitude:        0,
		Longitude:       0,
		LatestPsmsl:     tmp.ActiveDefinition.Psmsl,
		LatestPsmslRlr:  tmp.ActiveDefinition.PsmslRlr,
		LatestBodc:      tmp.ActiveDefinition.Bodc,
		LatestSonel:     tmp.ActiveDefinition.Sonel,
		LatestJasl:      tmp.ActiveDefinition.Jasl,
		LatestUhslcFast: tmp.ActiveDefinition.UhslcFast,
		LatestVliz:      tmp.ActiveDefinition.Vliz,
	})
	err = db.UpdateStationInfoGlossAll(tmp.GlossStations.Stations)
	if err != nil {
		slog.Error("Failed to update GLOSS station data", "error", err)
		return
	}
}
