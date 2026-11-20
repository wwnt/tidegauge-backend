package controller

import (
	"context"
	"net/http"
	"time"

	"tide/pkg/project"
	"tide/tide_server/global"
)

func runHTTPServer(r http.Handler) error {
	server := &http.Server{
		Addr:    global.Config.Listen,
		Handler: r,
		// 只限制读请求头的时间：不设的话，连接后迟迟不发完请求头的客户端可以无限期占着连接。
		// 不能设 ReadTimeout/WriteTimeout——/sync、/sync_v2/* 和 WebSocket 都是长连接。
		ReadHeaderTimeout: 15 * time.Second,
	}

	project.RegisterReleaseFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	return server.ListenAndServe()
}
