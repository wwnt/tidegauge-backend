package syncv2relay

import (
	"context"
	"log/slog"
	"net/http"

	internalsyncv2 "tide/internal/syncv2"
)

func (h *UpstreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	if h.UsernameFromRequest == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	authenticatedUsername := h.UsernameFromRequest(r)
	if authenticatedUsername == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, err := internalsyncv2.HijackUpgrade(w)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	stream := internalsyncv2.NewRelayMessageStream(conn, h.MaxFrameBytes)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-done:
		}
	}()

	if err = h.Server.StreamRelay(ctx, stream, authenticatedUsername); err != nil {
		log.Debug("v2 relay stream closed", "username", authenticatedUsername, "remote", conn.RemoteAddr().String(), "error", err)
	}
}
