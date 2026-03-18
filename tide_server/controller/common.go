package controller

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"tide/pkg/project"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/auth/permission"
	"tide/tide_server/auth/usermanager"
	"tide/tide_server/db"
	"tide/tide_server/global"
	"time"
)

var (
	editMu sync.Mutex

	hub *SyncHub

	userManager   auth.UserManager
	authorization auth.Permission
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

	if global.Config.Keycloak.BasePath != "" {
		userManager = usermanager.NewKeycloak(
			db.TideDB,
			global.Config.Keycloak.BasePath,
			global.Config.Keycloak.MasterUsername,
			global.Config.Keycloak.MasterPassword,
			global.Config.Keycloak.Realm,
			global.Config.Keycloak.ClientId,
			global.Config.Keycloak.ClientSecret,
		)
	} else {
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		userManager = usermanager.NewJwt(
			db.TideDB,
			key,
			"navi-tech.net",
			global.Config.Jwt.Expire*time.Second,
		)
	}

	authorization = permission.NewPostgres(db.TideDB)

	// Initialize pubsub instances and hub
	dataBroker := pubsub.NewBroker()
	delayedDataBroker := pubsub.NewDelayedBroker(dataBroker, global.Config.Tide.DataDelaySec*time.Second)
	missingDataBroker := pubsub.NewBroker()
	statusBroker := pubsub.NewBroker()
	configBroker := pubsub.NewBroker()
	hub = NewSyncHub(dataBroker, delayedDataBroker, missingDataBroker, statusBroker, configBroker, userManager, authorization)

	// Sync V2 (station + relay) depends on hub/userManager/authorization.
	initSyncV2()

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

	go func() {
		if err := runHTTPServer(r); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()
}
