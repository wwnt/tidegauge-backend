package controller

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"tide/tide_server/global"
)

var skipPaths = map[string]struct{}{
	"getSeaLevelList":  {},
	"seaHeightData":    {},
	"getGlossDataList": {},
	"getSonelDataList": {},
	"getPsmslDataList": {},
}

func requestLogUsername(r *http.Request) string {
	if username := requestUsername(r); username != "" {
		return username
	}
	if userManager == nil {
		return ""
	}
	username, err := userManager.GetLoginUser(r)
	if err != nil {
		return ""
	}
	return username
}

func requestRemoteAddr(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) > 0 {
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func slogLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := newStatusResponseWriter(w)

		defer func() {
			if err := recover(); err != nil {
				httpRequest, _ := httputil.DumpRequest(r, false)
				slog.Error("Request panic",
					"path", r.URL.Path,
					"error", err,
					"request", string(httpRequest),
					"username", requestLogUsername(r),
					"remote_addr", requestRemoteAddr(r),
				)
				if !sw.WroteHeader() && !sw.IsHijacked() {
					sw.WriteHeader(http.StatusInternalServerError)
				}
			}
		}()

		start := time.Now()
		next.ServeHTTP(sw, r)

		if global.Config.Debug {
			pathKey := strings.TrimPrefix(r.URL.Path, "/")
			if _, ok := skipPaths[pathKey]; !ok {
				slog.Debug("HTTP request",
					"path", r.URL.Path,
					"status", sw.Status(),
					"method", r.Method,
					"latency", time.Since(start),
					"username", requestLogUsername(r),
					"remote_addr", requestRemoteAddr(r),
				)
			}
		}
	})
}
