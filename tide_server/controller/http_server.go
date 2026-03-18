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
	}

	project.RegisterReleaseFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	return server.ListenAndServe()
}
