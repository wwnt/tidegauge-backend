package syncv2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const (
	ProtocolVersion = "v2"

	StationPath = "/sync_v2/station"
	RelayPath   = "/sync_v2/relay"

	DefaultMaxFrameBytes int64 = 32 << 20 // 32MiB
)

func StationURL(raw string) (string, error) {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return "", errors.New("empty url")
	}
	if !strings.Contains(addr, "://") {
		return "", errors.New("base url must include scheme (e.g. http://server:7100)")
	}
	// Caller must provide a valid base URL (including scheme). We only append the path.
	return strings.TrimRight(addr, "/") + StationPath, nil
}

func RelayURL(raw string) (string, error) {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return "", errors.New("empty url")
	}
	if !strings.Contains(addr, "://") {
		return "", errors.New("base url must include scheme (e.g. http://server:7100)")
	}
	// Caller must provide a valid base URL (including scheme). We only append the path.
	return strings.TrimRight(addr, "/") + RelayPath, nil
}

func DoUpgrade(ctx context.Context, c *http.Client, url string, h http.Header) (io.ReadWriteCloser, *http.Response, error) {
	if c == nil {
		c = &http.Client{}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, nil, err
	}

	if h != nil {
		for k, vs := range h {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}

	// net/http/response.go: func isProtocolSwitchResponse()
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, resp, fmt.Errorf("unexpected upgrade status: %d", resp.StatusCode)
	}

	conn, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		_ = resp.Body.Close()
		return nil, resp, errors.New("response body does not implement io.ReadWriteCloser")
	}

	return conn, resp, nil
}

func HijackUpgrade(w http.ResponseWriter) (net.Conn, error) {
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
		return nil, err
	}
	return conn, nil
}
