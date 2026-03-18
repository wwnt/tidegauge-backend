package upstreamauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeTransport struct {
	mu sync.Mutex

	loginCount    int
	requestCount  int
	latestToken   string
	force401First bool
	emptyToken    bool
}

func (t *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch req.URL.Path {
	case "/login":
		t.loginCount++
		if t.emptyToken {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		t.latestToken = fmt.Sprintf("token-%d", t.loginCount)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"` + t.latestToken + `"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	case "/resource":
		t.requestCount++
		if t.force401First && t.requestCount == 1 {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		if req.Header.Get("Authorization") != "Bearer "+t.latestToken {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
}

func TestDoWithAuth_CachesToken(t *testing.T) {
	rt := &fakeTransport{}
	httpClient := &http.Client{Transport: rt}
	client, err := NewClient(Config{
		BaseURL:    "http://example.invalid",
		Username:   "user",
		Password:   "pass",
		HTTPClient: httpClient,
	})
	require.NoError(t, err)

	build := func(token string) (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/resource", nil)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	}

	resp, err := client.DoWithAuth(context.Background(), build)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = client.DoWithAuth(context.Background(), build)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	rt.mu.Lock()
	require.Equal(t, 1, rt.loginCount)
	require.Equal(t, 2, rt.requestCount)
	rt.mu.Unlock()
}

func TestDoWithAuth_RetryOnUnauthorized(t *testing.T) {
	rt := &fakeTransport{force401First: true}
	httpClient := &http.Client{Transport: rt}
	client, err := NewClient(Config{
		BaseURL:    "http://example.invalid",
		Username:   "user",
		Password:   "pass",
		HTTPClient: httpClient,
	})
	require.NoError(t, err)

	resp, err := client.DoWithAuth(context.Background(), func(token string) (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/resource", nil)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	rt.mu.Lock()
	require.Equal(t, 2, rt.loginCount)
	require.Equal(t, 2, rt.requestCount)
	rt.mu.Unlock()
}

func TestDoWithAuth_LoginReturnsEmptyToken(t *testing.T) {
	rt := &fakeTransport{emptyToken: true}
	httpClient := &http.Client{Transport: rt}
	client, err := NewClient(Config{
		BaseURL:    "http://example.invalid",
		Username:   "user",
		Password:   "pass",
		HTTPClient: httpClient,
	})
	require.NoError(t, err)

	_, err = client.DoWithAuth(context.Background(), func(token string) (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/resource", nil)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty access_token")
}
