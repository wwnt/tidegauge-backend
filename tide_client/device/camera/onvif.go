package camera

import (
	"go.fuchsia.dev/fuchsia/tools/net/digest"
	"io"
	"net/http"
)

func OnvifSnapshot(url string, username, password string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, nil
	}
	var transport http.RoundTripper
	if username == "" {
		transport = http.DefaultTransport
	} else {
		transport = digest.NewTransport(username, password)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}
