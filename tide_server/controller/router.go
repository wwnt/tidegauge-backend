package controller

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"tide/pkg/wsutil"
	"tide/tide_server/auth"
	"tide/tide_server/global"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const syncPath = "/sync"
const loginPath = "/login"
const cameraLatestSnapshotPath = "/cameraLatestSnapshot"

func setupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	//must be setup first
	r.Use(SlogLogger)

	r.GET("/getSeaLevelList", GetSateAltimetry)
	r.GET("/seaHeightData", GetSeaLevel)
	r.GET("/getGlossDataList", GetGlossDataList)
	r.GET("/getSonelDataList", GetSonelDataList)
	r.GET("/getPsmslDataList", GetPsmslDataList)
	r.GET("/IOCHistory", IOCHistory)

	r.GET("/ws/global", upgradeWs, validateWs, GlobalWebsocket)
	r.GET("/ws/data", upgradeWs, validateWs, DataWebsocket)

	r.POST("/applyAccount", ApplyAccount)

	login := r.Group("/", validate)

	r.POST(loginPath, Login)
	login.POST("/logout", Logout)

	login.POST(syncPath, Sync)

	login.GET("/listUpstream", validateAdmin, ListUpstream)
	login.POST("/editUpstream", validateAdmin, EditUpstream)
	login.POST("/delUpstream", validateAdmin, DelUpstream)

	login.GET("/listUser", validateAdmin, ListUser)

	login.GET("/userInfo", UserInfo)
	login.POST("/editUser", EditUser)
	login.POST("/passApplication", validateAdmin, PassApplication)
	login.POST("/delUser", validateAdmin, DelUser)

	login.GET("/listPermission", ListPermission)
	login.POST("/editPermission", validateAdmin, EditPermission)
	login.GET("/listCameraStatusPermission", ListCameraStatusPermission)
	login.POST("/editCameraStatusPermission", validateAdmin, EditCameraStatusPermission)

	login.GET("/dataHistory", DataHistory)
	login.GET("/itemStatusLogs", ListItemStatusLogs)

	login.GET("/listStation", ListStation)
	login.POST("/editStation", validateAdmin, EditStation)
	login.POST("/delStation", validateAdmin, DelStation)

	login.GET("/listDevice", ListDevice)
	login.POST("/editDevice", validateAdmin, EditDevice)
	login.GET("/listDeviceRecord", ListDeviceRecord)
	login.POST("/editDeviceRecord", validateAdmin, EditDeviceRecord)

	login.GET("/listItem", ListItem)

	login.GET("/cameraSnapshot", validateLiveSnapshot, CameraLiveSnapshot)
	login.GET(cameraLatestSnapshotPath, CameraLatestSnapShot)
	return r
}

var skipPaths = map[string]struct{}{"getSeaLevelList": {}, "seaHeightData": {}, "getGlossDataList": {}, "getSonelDataList": {}, "getPsmslDataList": {}}

func SlogLogger(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil {
			httpRequest, _ := httputil.DumpRequest(c.Request, false)
			username := c.GetString(contextKeyUsername)

			slog.Error("Request panic",
				"path", c.Request.URL.Path,
				"error", err,
				"request", string(httpRequest),
				"username", username,
			)
			c.AbortWithStatus(http.StatusInternalServerError)
		}
	}()
	start := time.Now()
	c.Next()

	if len(c.Errors) > 0 {
		username := c.GetString(contextKeyUsername)
		slog.Error("Request error",
			"path", c.Request.URL.Path,
			"query", c.Request.URL.RawQuery,
			"error", c.Errors.String(),
			"username", username,
		)
	}

	if global.Config.Debug {
		pathKey := strings.TrimPrefix(c.Request.URL.Path, "/")
		if _, ok := skipPaths[pathKey]; !ok {
			latency := time.Since(start)
			username := c.GetString(contextKeyUsername)
			slog.Debug("HTTP request",
				"path", c.Request.URL.Path,
				"status", c.Writer.Status(),
				"method", c.Request.Method,
				"latency", latency,
				"username", username,
			)
		}
	}
}

var wsUpgrade = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func validateWs(c *gin.Context) {
	ws := c.MustGet(contextKeyWsConn).(wsutil.WsWrap)
	// check token
	username, err := userManager.GetLoginUser(c.Request)
	if err != nil {
		_ = ws.WriteControl(websocket.CloseMessage, wsInternalServerError, time.Now().Add(writeWait))
		c.Abort()
	} else if username == "" {
		_ = ws.WriteControl(websocket.CloseMessage, wsUnauthorized, time.Now().Add(writeWait))
		c.Abort()
	} else {
		user, err := userManager.GetUser(username)
		if err != nil {
			slog.Error("Failed to get user info", "username", username, "error", err)
			_ = ws.WriteControl(websocket.CloseMessage, wsInternalServerError, time.Now().Add(writeWait))
			c.Abort()
			return
		}

		c.Set(contextKeyUsername, username)
		c.Set(contextKeyRole, user.Role)
		c.Set(contextKeyEmail, user.Email)
	}
}

func validate(c *gin.Context) {
	username, err := userManager.GetLoginUser(c.Request)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
	} else if username == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
	} else {
		user, err := userManager.GetUser(username)
		if err != nil {
			slog.Error("Failed to get user info in validation", "username", username, "error", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Set(contextKeyUserInfo, user)
		c.Set(contextKeyUsername, username)
		c.Set(contextKeyRole, user.Role)
		c.Set(contextKeyEmail, user.Email)
		if user.Role == auth.NormalUser {
			c.Set(contextKeyLiveCamera, user.LiveCamera)
		}
	}
}

func validateAdmin(c *gin.Context) {
	if c.GetInt(contextKeyRole) < auth.Admin {
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func validateLiveSnapshot(c *gin.Context) {
	if c.GetInt(contextKeyRole) == auth.NormalUser {
		if !c.GetBool(contextKeyLiveCamera) {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
	}
}

func upgradeWs(c *gin.Context) {
	ws, err := wsUpgrade.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer func() { _ = ws.Close() }()

	wsw := wsutil.WsWrap{Conn: ws}
	c.Set(contextKeyWsConn, wsw)
	c.Next()
}
