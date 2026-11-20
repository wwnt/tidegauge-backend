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
	"time"

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

// externalHTTPClient 用于抓取第三方站点（IOC、PSMSL）。必须带超时：
// http.Get 用的 DefaultClient 没有超时，对端挂起时调用方会永久阻塞。
var externalHTTPClient = &http.Client{Timeout: 60 * time.Second}

// refreshExternalSeaData 周期性抓取第三方海平面数据。两个抓取函数解析的是别人的网页/XML，
// 格式随时可能变；任何一次 panic 都只应让这一轮失败，不能让整个服务进程退出
// （本函数在启动时最先执行，进程退出后被 systemd 拉起又会立刻再崩）。
func refreshExternalSeaData() {
	run := func(name string, fn func()) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("External sea data refresh panicked", "task", name, "panic", r)
			}
		}()
		fn()
	}
	for {
		run("seaHeight", seaHeight)
		run("stationInfoGlossAll", stationInfoGlossAll)
		time.Sleep(120 * time.Second)
	}
}

func seaHeight() {
	resp, err := externalHTTPClient.Get("https://www.ioc-sealevelmonitoring.org/list.php?operator=&showall=all&output=general")
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

	const headerRows = 3
	tr := doc.Find("div.maincontent > table > tbody > tr > td > form > table > tbody > tr")
	// goquery 的 Slice 不做越界检查：页面不是预期的表格（维护页、改版）时直接切片会 panic。
	if tr.Length() <= headerRows {
		slog.Error("Unexpected IOC page layout, keeping previous sea level data", "rows", tr.Length())
		return
	}
	tr = tr.Slice(headerRows, goquery.ToEnd)
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
	// UpdateSeaLevel 先清空再写入；一行有效数据都没解析出来时不要用空结果覆盖上一轮的数据。
	if len(data) == 0 {
		slog.Error("No sea level rows parsed from IOC page, keeping previous data")
		return
	}
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
	resp, err := externalHTTPClient.Get("https://www.psmsl.org/products/gloss/data/glossCoreNetwork.xml")
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
