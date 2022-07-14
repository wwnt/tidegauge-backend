package controller

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"tide/tide_server/db"
)

func ListItemStatusLogs(c *gin.Context) {
	var param struct {
		PageNum  uint `form:"page_num" binding:"gte=1"`
		PageSize uint `form:"page_size" binding:"gte=1,lte=100"`
	}
	if c.Bind(&param) != nil {
		return
	}
	ds, err := db.PagedItemStatusLogs(param.PageNum, param.PageSize)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ds)
}
