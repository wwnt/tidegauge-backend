package controller

import (
	"net/http"
	"net/url"
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
	// 本接口无需登录。id 必须转义后再拼进 URL，否则调用方可以借它给第三方站点追加任意
	// 查询参数；请求绑定到入站请求的 context 并带超时，客户端断开或对端挂起时不会一直占着。
	iocURL := "https://www.ioc-sealevelmonitoring.org/bgraph.php?code=" + url.QueryEscape(id) + "&output=tab&period=0.5"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, iocURL, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	const headerRows = 2
	tr := doc.Find("tr")
	// goquery 的 Slice 不做越界检查，行数不足（未知站点代码、页面改版）时先行返回。
	if tr.Length() <= headerRows {
		writeJSON(w, http.StatusOK, [][2]string{})
		return
	}
	radIdx := 0
	tr.Eq(1).Children().Each(func(i int, selection *goquery.Selection) {
		if strings.Contains(selection.Text(), "rad") {
			radIdx = i
		}
	})
	if radIdx == 0 {
		writeJSON(w, http.StatusOK, [][2]string{})
		return
	}

	// remove 2 row header
	tr = tr.Slice(headerRows, goquery.ToEnd)
	if tr.Size() > 30 {
		tr = tr.Slice(-30, goquery.ToEnd)
	}

	data := make([][2]string, 0, tr.Size())
	tr.Each(func(_ int, selection *goquery.Selection) {
		td := selection.Children()
		data = append(data, [2]string{td.Eq(0).Text(), td.Eq(radIdx).Text()})
	})
	writeJSON(w, http.StatusOK, data)
}
