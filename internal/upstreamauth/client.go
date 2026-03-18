package upstreamauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultLoginPath    = "/login"
	defaultLoginTimeout = 10 * time.Second
)

type Config struct {
	BaseURL      string
	Username     string
	Password     string
	LoginPath    string
	LoginTimeout time.Duration
	HTTPClient   *http.Client
}

type RequestFactory func(token string) (*http.Request, error)

type Client struct {
	baseURL      string
	username     string
	password     string
	loginPath    string
	loginTimeout time.Duration
	httpClient   *http.Client

	tokenMu     sync.Mutex
	bearerToken string
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, errors.New("empty base url")
	}
	if !strings.Contains(baseURL, "://") {
		return nil, errors.New("base url must include scheme (e.g. http://server:7100)")
	}

	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		return nil, errors.New("empty username")
	}
	if cfg.Password == "" {
		return nil, errors.New("empty password")
	}

	loginPath := strings.TrimSpace(cfg.LoginPath)
	if loginPath == "" {
		loginPath = defaultLoginPath
	}
	if !strings.HasPrefix(loginPath, "/") {
		loginPath = "/" + loginPath
	}

	loginTimeout := cfg.LoginTimeout
	if loginTimeout <= 0 {
		loginTimeout = defaultLoginTimeout
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		username:     username,
		password:     cfg.Password,
		loginPath:    loginPath,
		loginTimeout: loginTimeout,
		httpClient:   httpClient,
	}, nil
}

// DoWithAuth runs one upstream request with bearer auth.
// It retries once when upstream returns 401: clear cached token, re-login, and resend.
func (c *Client) DoWithAuth(ctx context.Context, build RequestFactory) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("nil auth client")
	}
	if build == nil {
		return nil, errors.New("nil request factory")
	}

	token, err := c.getToken(ctx, false)
	if err != nil {
		return nil, err
	}

	req, err := build(token)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	// Close the first response body before retrying to avoid leaks.
	_ = resp.Body.Close()

	token, err = c.getToken(ctx, true)
	if err != nil {
		return nil, err
	}
	req, err = build(token)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *Client) getToken(ctx context.Context, forceRefresh bool) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if forceRefresh {
		c.bearerToken = ""
	}
	if token := strings.TrimSpace(c.bearerToken); token != "" {
		return token, nil
	}

	token, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	c.bearerToken = token
	return token, nil
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
}

func (c *Client) login(ctx context.Context) (string, error) {
	loginCtx, cancel := context.WithTimeout(ctx, c.loginTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)

	loginURL := c.baseURL + c.loginPath
	req, err := http.NewRequestWithContext(loginCtx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upstream login returned %s", resp.Status)
	}

	var loginResp loginResponse
	if err = json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", err
	}
	token := strings.TrimSpace(loginResp.AccessToken)
	if token == "" {
		return "", errors.New("upstream login returned empty access_token")
	}
	return token, nil
}
