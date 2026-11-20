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

const (
	// defaultJwtExpire 是配置里没有 jwt.expire 时的令牌有效期。
	defaultJwtExpire = 6 * time.Hour

	// schema.sql 里预置的超级管理员账号及其初始密码（与 schema.sql 的 insert 保持一致）。
	schemaDefaultAdminUsername = "tgm-admin"
	schemaDefaultAdminPassword = "123456"
)

func Init() {
	db.Init()
	project.RegisterReleaseFunc(db.CloseDB)

	go refreshExternalSeaData()

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
		if _, err := rand.Read(key); err != nil {
			slog.Error("Failed to generate JWT signing key", "error", err)
			os.Exit(1)
		}
		// jwt.expire 缺省为 0 时签出的令牌立刻过期，表现为"能登录但所有请求 401"。
		expire := global.Config.Jwt.Expire * time.Second
		if expire <= 0 {
			expire = defaultJwtExpire
			slog.Warn("jwt.expire is not configured, using default", "expire", expire)
		}
		jwtManager := usermanager.NewJwt(
			db.TideDB,
			key,
			"navi-tech.net",
			expire,
		)
		// schema.sql 预置的超级管理员带着公开的初始密码；仍在使用时每次启动都高声提醒。
		if jwtManager.CheckUserPwd(schemaDefaultAdminUsername, schemaDefaultAdminPassword) {
			slog.Error("SECURITY: the built-in super admin still uses the default password from schema.sql; change it now",
				"username", schemaDefaultAdminUsername)
		}
		userManager = jwtManager
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
