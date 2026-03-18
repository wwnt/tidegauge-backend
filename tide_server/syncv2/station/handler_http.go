package syncv2station

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	internalsyncv2 "tide/internal/syncv2"

	"github.com/hashicorp/yamux"
)

type Handler struct {
	Enabled       func() bool
	Server        *Server
	MaxFrameBytes int64
	Logger        *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Server == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if h.Enabled != nil && !h.Enabled() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	log := h.Logger
	if log == nil {
		log = slog.Default()
	}

	conn, err := internalsyncv2.HijackUpgrade(w)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	muxCfg := yamux.DefaultConfig()
	muxCfg.KeepAliveInterval = 100 * time.Second
	muxCfg.ConnectionWriteTimeout = 30 * time.Second
	muxCfg.LogOutput = io.Discard
	session, err := yamux.Server(conn, muxCfg)
	if err != nil {
		log.Debug("failed to create station yamux session", "remote", conn.RemoteAddr().String(), "error", err)
		return
	}
	defer func() { _ = session.Close() }()

	mainConn, err := session.Open()
	if err != nil {
		log.Debug("failed to open station main stream", "remote", conn.RemoteAddr().String(), "error", err)
		return
	}
	stream := internalsyncv2.NewStationMessageStream(mainConn, h.MaxFrameBytes)
	defer func() { _ = stream.Close() }()

	openCommandStream := func() (internalsyncv2.StationMessageStream, error) {
		cmdConn, openErr := session.Open()
		if openErr != nil {
			return nil, openErr
		}
		return internalsyncv2.NewStationMessageStream(cmdConn, h.MaxFrameBytes), nil
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-done:
		}
	}()

	if err = h.Server.StreamStation(ctx, stream, openCommandStream, conn.RemoteAddr().String()); err != nil {
		log.Debug("v2 station stream closed", "remote", conn.RemoteAddr().String(), "error", err)
	}
}
