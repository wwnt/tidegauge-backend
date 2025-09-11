package controller

import (
	"log/slog"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"tide/pkg/project"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/auth/permission"
	"tide/tide_server/auth/usermanager"
	"tide/tide_server/db"
	"tide/tide_server/global"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	editMu sync.Mutex

	dataPubSub      = pubsub.NewPubSub()
	dataPubSubDelay = pubsub.NewDelayPublish(dataPubSub, global.Config.Tide.DataDelaySec*time.Second, nil)
	missDataPubSub  = pubsub.NewPubSub()
	statusPubSub    = pubsub.NewPubSub()
	configPubSub    = pubsub.NewPubSub()

	userManager   auth.UserManager
	authorization auth.Permission
)

const (
	contextKeyWsConn     = "wsConn"
	contextKeyUserInfo   = "user_info"
	contextKeyEmail      = "email"
	contextKeyUsername   = "username"
	contextKeyRole       = "role"
	contextKeyLiveCamera = "live_camera"
)

func Init() {
	db.Init()
	project.RegisterReleaseFunc(db.CloseDB)

	go func() {
		for {
			seaHeight()
			stationInfoGlossAll()
			time.Sleep(120 * time.Second)
		}
	}()

	userManager = usermanager.NewKeycloak(
		db.TideDB,
		global.Config.Keycloak.BasePath,
		global.Config.Keycloak.MasterUsername,
		global.Config.Keycloak.MasterPassword,
		global.Config.Keycloak.Realm,
		global.Config.Keycloak.ClientId,
		global.Config.Keycloak.ClientSecret,
	)

	authorization = permission.NewPostgres(db.TideDB)

	upstreams, err := db.GetUpstreams()
	if err != nil {
		slog.Error("Failed to get upstreams", "error", err)
		os.Exit(1)
	}
	for _, upstream := range upstreams {
		go startSync(upstream)
	}
	go tideDataReceiver()
	//go cameraStorage()

	r := setupRouter()

	r.Any("/debug/pprof/*name", func(c *gin.Context) {
		name := strings.TrimPrefix(c.Param("name"), "/")
		switch name {
		case "":
			pprof.Index(c.Writer, c.Request)
		case "cmdline":
			pprof.Cmdline(c.Writer, c.Request)
		case "profile":
			pprof.Profile(c.Writer, c.Request)
		case "symbol":
			pprof.Symbol(c.Writer, c.Request)
		case "trace":
			pprof.Trace(c.Writer, c.Request)
		default:
			pprof.Handler(name).ServeHTTP(c.Writer, c.Request)
		}
	})

	go func() {
		if err := r.Run(global.Config.Listen); err != nil {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()
}
