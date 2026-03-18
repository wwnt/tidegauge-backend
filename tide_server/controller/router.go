package controller

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"

	internalsyncv2 "tide/internal/syncv2"
	"tide/tide_server/auth"

	"github.com/coder/websocket"
)

const syncPath = "/sync"
const loginPath = "/login"
const cameraLatestSnapshotPath = "/cameraLatestSnapshot"

type Middleware func(http.Handler) http.Handler

func chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func setupRouter() http.Handler {
	mux := http.NewServeMux()
	handle := func(method string, path string, h http.HandlerFunc, middlewares ...Middleware) {
		pattern := path
		if method != "" {
			pattern = method + " " + path
		}
		mux.Handle(pattern, chain(h, middlewares...))
	}
	authMW := []Middleware{validateMiddleware}
	adminMW := []Middleware{validateMiddleware, validateAdminMiddleware}

	// Sea routes.
	handle(http.MethodGet, "/getSeaLevelList", GetSateAltimetry)
	handle(http.MethodGet, "/seaHeightData", GetSeaLevel)
	handle(http.MethodGet, "/getGlossDataList", GetGlossDataList)
	handle(http.MethodGet, "/getSonelDataList", GetSonelDataList)
	handle(http.MethodGet, "/getPsmslDataList", GetPsmslDataList)
	handle(http.MethodGet, "/IOCHistory", IOCHistory)

	// WebSocket routes.
	handle(http.MethodGet, "/ws/global", GlobalWebsocket, upgradeWsMiddleware, validateWsMiddleware)
	handle(http.MethodGet, "/ws/data", DataWebsocket, upgradeWsMiddleware, validateWsMiddleware)

	// Auth routes.
	handle(http.MethodPost, "/applyAccount", ApplyAccount)
	handle(http.MethodPost, loginPath, Login)
	handle(http.MethodPost, "/logout", Logout, authMW...)

	// Sync routes.
	handle(http.MethodPost, syncPath, Sync, validateMiddleware)
	handle(http.MethodPost, internalsyncv2.StationPath, v2StationHandler.ServeHTTP)
	handle(http.MethodPost, internalsyncv2.RelayPath, v2RelayHandler.ServeHTTP, validateMiddleware)

	// Upstream routes.
	handle(http.MethodGet, "/listUpstream", ListUpstream, adminMW...)
	handle(http.MethodPost, "/editUpstream", EditUpstream, adminMW...)
	handle(http.MethodPost, "/delUpstream", DelUpstream, adminMW...)

	// User routes.
	handle(http.MethodGet, "/listUser", ListUser, adminMW...)
	handle(http.MethodGet, "/userInfo", UserInfo, authMW...)
	handle(http.MethodPost, "/editUser", EditUser, authMW...)
	handle(http.MethodPost, "/passApplication", PassApplication, adminMW...)
	handle(http.MethodPost, "/delUser", DelUser, adminMW...)

	// Permission routes.
	handle(http.MethodGet, "/listPermission", ListPermission, authMW...)
	handle(http.MethodPost, "/editPermission", EditPermission, adminMW...)
	handle(http.MethodGet, "/listCameraStatusPermission", ListCameraStatusPermission, authMW...)
	handle(http.MethodPost, "/editCameraStatusPermission", EditCameraStatusPermission, adminMW...)

	// Data routes.
	handle(http.MethodGet, "/dataHistory", DataHistory, authMW...)
	handle(http.MethodGet, "/itemStatusLogs", ListItemStatusLogs, authMW...)

	// Station routes.
	handle(http.MethodGet, "/listStation", ListStation, authMW...)
	handle(http.MethodPost, "/editStation", EditStation, adminMW...)
	handle(http.MethodPost, "/delStation", DelStation, adminMW...)

	// Device routes.
	handle(http.MethodGet, "/listDevice", ListDevice, authMW...)
	handle(http.MethodPost, "/editDevice", EditDevice, adminMW...)
	handle(http.MethodGet, "/listDeviceRecord", ListDeviceRecord, authMW...)
	handle(http.MethodPost, "/editDeviceRecord", EditDeviceRecord, adminMW...)

	// Item and camera routes.
	handle(http.MethodGet, "/listItem", ListItem, authMW...)
	handle(http.MethodGet, "/cameraSnapshot", CameraLiveSnapshot, validateMiddleware, validateLiveSnapshotMiddleware)
	handle(http.MethodGet, cameraLatestSnapshotPath, CameraLatestSnapShot, authMW...)

	registerPprofRoutes(mux)

	// must be setup first
	return chain(mux, slogLoggerMiddleware)
}

func registerPprofRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
}

func upgradeWsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, wsAcceptOptions)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		ws.SetReadLimit(16 << 20)

		next.ServeHTTP(w, withWSConn(r, ws))
	})
}

var wsAcceptOptions = &websocket.AcceptOptions{
	InsecureSkipVerify: true,
}

type authResult struct {
	Request    *http.Request
	StatusCode int
}

func attachAuthenticatedUser(r *http.Request, username string) authResult {
	if username == "" {
		return authResult{StatusCode: http.StatusUnauthorized}
	}

	user, err := userManager.GetUser(username)
	if err != nil {
		slog.Error("Failed to get user info in request auth", "username", username, "error", err)
		return authResult{StatusCode: http.StatusInternalServerError}
	}
	return authResult{Request: withRequestUser(r, username, user)}
}

func authenticateRequest(r *http.Request) authResult {
	username, err := userManager.GetLoginUser(r)
	if err != nil {
		return authResult{StatusCode: http.StatusInternalServerError}
	}
	return attachAuthenticatedUser(r, username)
}

func closeWSByHTTPStatus(ws *websocket.Conn, statusCode int) {
	wsCode := wsStatusInternalServerError
	if statusCode == http.StatusUnauthorized {
		wsCode = wsStatusUnauthorized
	}
	_ = ws.Close(wsCode, http.StatusText(statusCode))
}

func validateWsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, ok := requestWSConn(r)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		authRes := authenticateRequest(r)
		if authRes.StatusCode != 0 {
			closeWSByHTTPStatus(ws, authRes.StatusCode)
			return
		}
		next.ServeHTTP(w, authRes.Request)
	})
}

func validateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authRes := authenticateRequest(r)
		if authRes.StatusCode != 0 {
			w.WriteHeader(authRes.StatusCode)
			return
		}
		next.ServeHTTP(w, authRes.Request)
	})
}

func validateAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestRole(r) < auth.Admin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validateLiveSnapshotMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestRole(r) == auth.NormalUser && !requestLiveCamera(r) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hijackUpgradeConn(w http.ResponseWriter) (net.Conn, error) {
	// make client's Response.Body implement io.ReadWriteCloser
	// net/http/response.go: func isProtocolSwitchResponse()
	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	w.WriteHeader(http.StatusSwitchingProtocols)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer does not support hijacking")
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		slog.Error("Failed to hijack sync connection", "error", err)
		return nil, err
	}
	return conn, nil
}
