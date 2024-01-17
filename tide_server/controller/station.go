package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"go.uber.org/zap"
	"net/http"
	"tide/pkg/custype"
	"tide/tide_server/auth"
	"tide/tide_server/db"
)

func ListStation(c *gin.Context) {
	stations, err := db.GetStations()
	if err != nil {
		logger.Error(err.Error())
		return
	}
	c.JSON(http.StatusOK, stations)
}

func EditStation(c *gin.Context) {
	var station db.Station
	if err := c.Bind(&station); err != nil {
		logger.Error("EditStation", zap.Error(err))
		return
	}
	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditStation(&station); err != nil {
		logger.Error(err.Error())
		return
	}
	if !station.Upstream { // Only sync the local station
		sendToConfigPubSub(kMsgSyncStation, station)
	}
	_, _ = c.Writer.Write([]byte("ok"))
}

func DelStation(c *gin.Context) {
	editMu.Lock()
	defer editMu.Unlock()
	stationId, err := uuid.Parse(c.PostForm("id"))
	if err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err).SetType(gin.ErrorTypeBind)
		return
	}
	// Can only delete local stations
	if n, err := db.DelLocalStation(stationId); err != nil {
		logger.Error(err.Error())
		return
	} else if n > 0 {
		if value, ok := recvConnections.Load(stationId); ok {
			_ = value.(*yamux.Session).Close()
		}
		sendToConfigPubSub(kMsgDelUpstreamStation, stationId)
	} //If there is no update, there is no need to publish
	_, _ = c.Writer.Write([]byte("ok"))
}

func ListDevice(c *gin.Context) {
	stationId, _ := uuid.Parse(c.Query("station_id"))
	devices, err := db.GetDevices(stationId)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	c.JSON(http.StatusOK, devices)
}

func EditDevice(c *gin.Context) {
	editMu.Lock()
	defer editMu.Unlock()
	var device db.Device
	if err := c.Bind(&device); err != nil {
		logger.Error("", zap.Error(err))
		return
	}
	if up, err := db.IsUpstreamStation(device.StationId); err != nil || up {
		return
	}
	// Can only edit local stations
	if err := db.EditDevice(device); err != nil {
		logger.Error(err.Error())
		return
	}
	sendToConfigPubSub(kMsgSyncDevice, device)
	_, _ = c.Writer.Write([]byte("ok"))
}

func ListDeviceRecord(c *gin.Context) {
	deviceRecords, err := db.GetDeviceRecords()
	if err != nil {
		logger.Error(err.Error())
		return
	}
	c.JSON(http.StatusOK, deviceRecords)
}

func EditDeviceRecord(c *gin.Context) {
	var dr db.DeviceRecord
	if err := c.Bind(&dr); err != nil {
		logger.Error(err.Error())
		return
	}
	if dr.Id == uuid.Nil { // Can only edit local stations
		if up, err := db.IsUpstreamStation(dr.StationId); err != nil || up {
			return
		}
	}
	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditDeviceRecord(&dr); err != nil {
		logger.Error(err.Error())
		return
	}
	sendToConfigPubSub(kMsgEditDeviceRecord, dr)
	_, _ = c.Writer.Write([]byte("ok"))
}

func ListItem(c *gin.Context) {
	var stationId uuid.UUID
	var err error
	s, ok := c.GetQuery("station_id")
	if ok {
		if stationId, err = uuid.Parse(s); err != nil {
			return
		}
	}
	items, err := db.GetItems(stationId)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	c.JSON(http.StatusOK, items)
}

func DataHistory(c *gin.Context) {
	var param struct {
		ItemName string `form:"item_name" binding:"required"`
		Start    int64  `form:"start"`
		End      int64  `form:"end"`
	}
	if err := c.Bind(&param); err != nil {
		logger.Error(err.Error())
		return
	}
	stationId, err := uuid.Parse(c.Query("station_id"))
	if err != nil {
		return
	}
	if c.GetInt(contextKeyRole) < auth.Admin {
		if !authorization.CheckPermission(c.GetString(contextKeyUsername), stationId, param.ItemName) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	ds, err := db.GetDataHistory(stationId, param.ItemName, custype.TimeMillisecond(param.Start), custype.TimeMillisecond(param.End))
	if err != nil {
		logger.Error(err.Error())
		return
	}
	c.JSON(http.StatusOK, ds)
}

//func bind(c *gin.Context, obj any) bool {
//	if errs := c.ShouldBind(obj); errs != nil {
//		logger.Error(errs.Error())
//		if errs, ok := errs.(validator.ValidationErrors); ok {
//			for _, err := range errs {
//				_, _ = c.Writer.WriteString(err.Field() + ": input error\n")
//			}
//		}
//		return false
//	} else {
//		return true
//	}
//}
