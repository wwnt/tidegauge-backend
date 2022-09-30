package controller

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"tide/tide_server/db"
)

func GetSateAltimetry(c *gin.Context) {
	tn := c.Query("tableName")
	if tn == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	ss, err := db.GetSateAltimetry(tn)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ss)
}

func GetSeaHeight(c *gin.Context) {
	ss, err := db.GetSeaHeight()
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ss)
}

func GetGlossDataList(c *gin.Context) {
	ss, err := db.GetGlossData()
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ss)
}

func GetSonelDataList(c *gin.Context) {
	ds, err := db.GetSonelData()
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ds)
}

func GetPsmslDataList(c *gin.Context) {
	ds, err := db.GetPsmslData()
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ds)
}

func IOCHistory(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	resp, err := http.Get("http://www.ioc-sealevelmonitoring.org/bgraph.php?code=" + id + "&output=tab&period=0.5")
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.AbortWithStatus(resp.StatusCode)
		return
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	tr := doc.Find("tr")

	var radIdx = 0

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
	tr.Each(func(i int, selection *goquery.Selection) {
		td := selection.Children()
		data = append(data, [2]string{td.Eq(0).Text(), td.Eq(radIdx).Text()})
	})
	c.JSON(http.StatusOK, data)
}
