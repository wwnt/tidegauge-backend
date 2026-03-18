package controller

import (
	"net/http"
	"strings"
	"tide/tide_server/db"

	"github.com/PuerkitoBio/goquery"
)

func GetSateAltimetry(w http.ResponseWriter, r *http.Request) {
	tn := r.URL.Query().Get("tableName")
	if tn == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ss, err := db.GetSateAltimetry(tn)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

func GetSeaLevel(w http.ResponseWriter, _ *http.Request) {
	ss, err := db.GetSeaLevel()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

func GetGlossDataList(w http.ResponseWriter, _ *http.Request) {
	ss, err := db.GetGlossData()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

func GetSonelDataList(w http.ResponseWriter, _ *http.Request) {
	ds, err := db.GetSonelData()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

func GetPsmslDataList(w http.ResponseWriter, _ *http.Request) {
	ds, err := db.GetPsmslData()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

func IOCHistory(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp, err := http.Get("https://www.ioc-sealevelmonitoring.org/bgraph.php?code=" + id + "&output=tab&period=0.5")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		return
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tr := doc.Find("tr")
	radIdx := 0
	tr.Eq(1).Children().Each(func(i int, selection *goquery.Selection) {
		if strings.Contains(selection.Text(), "rad") {
			radIdx = i
		}
	})
	if radIdx == 0 {
		return
	}

	// remove 2 row header
	tr = tr.Slice(2, goquery.ToEnd)
	if tr.Size() > 30 {
		tr = tr.Slice(-30, goquery.ToEnd)
	}

	var data [][2]string
	tr.Each(func(_ int, selection *goquery.Selection) {
		td := selection.Children()
		data = append(data, [2]string{td.Eq(0).Text(), td.Eq(radIdx).Text()})
	})
	writeJSON(w, http.StatusOK, data)
}
