// Copyright 2018 The Fuchsia Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package digest

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var c = &credentials{
	username:  "admin",
	realm:     "Digest:4C1F0000000000000000000000000000",
	nonce:     "GZHoABAHAAAAAAAAtejSfCEQLbW+c/fM",
	uri:       "/index",
	algorithm: "MD5",
	qop:       "auth",
	method:    "POST",
	password:  "password",
}

var cnonce = "0a4f113b"

func TestHa1(t *testing.T) {
	r := c.ha1()
	if r != "e00fd2f74e4bb1ccd5c3f359e13822ce" {
		t.Fail()
	}
}

func TestHa2(t *testing.T) {
	r := c.ha2()
	if r != "f272ccec928f9de4e8e0bc6319ab2c66" {
		t.Fail()
	}
}

func TestResponse(t *testing.T) {
	r, err := c.response(cnonce)
	if err != nil {
		t.Fail()
	}
	if r != "ce25c065de2d1c900b21ed6d6fbe886b" {
		t.Fail()
	}
}

func TestParseChallenge(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    challenge
		wantErr bool
	}{
		{
			name:  "typical camera challenge",
			input: `Digest realm="Login to cam", qop="auth", nonce="abc123", opaque="xyz"`,
			want:  challenge{realm: "Login to cam", qop: "auth", nonce: "abc123", opaque: "xyz", algorithm: "MD5"},
		},
		{
			name:  "quoted qop list keeps its comma and selects auth",
			input: `Digest realm="r", nonce="n", qop="auth,auth-int", algorithm=md5`,
			want:  challenge{realm: "r", nonce: "n", qop: "auth", algorithm: "MD5"},
		},
		{
			name:  "comma inside a quoted realm is not a separator",
			input: `Digest realm="a, b", nonce="n"`,
			want:  challenge{realm: "a, b", nonce: "n", algorithm: "MD5"},
		},
		{
			name:  "unknown extension directives are ignored",
			input: `Digest realm="r", nonce="n", charset=UTF-8, userhash=false`,
			want:  challenge{realm: "r", nonce: "n", algorithm: "MD5"},
		},
		{name: "directive without a value", input: `Digest realm`, wantErr: true},
		{name: "bare flag after valid directives", input: `Digest realm="r", nonce="n", stale`, wantErr: true},
		{name: "missing nonce", input: `Digest realm="r"`, wantErr: true},
		{name: "not a digest challenge", input: `Basic realm="r"`, wantErr: true},
		{name: "empty header", input: ``, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChallenge(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrBadChallenge) {
					t.Fatalf("parseChallenge(%q) error = %v, want ErrBadChallenge", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseChallenge(%q) unexpected error: %v", tt.input, err)
			}
			if *got != tt.want {
				t.Fatalf("parseChallenge(%q) = %+v, want %+v", tt.input, *got, tt.want)
			}
		})
	}
}

// 走一遍真实的 401 -> 带 Authorization 重试流程；请求是不带 GetBody 的普通 GET。
func TestRoundTripAnswersChallenge(t *testing.T) {
	var authorized int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="cam", qop="auth,auth-int", nonce="n0nce"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(auth, "Digest ") && strings.Contains(auth, `username="admin"`) && strings.Contains(auth, "qop=auth") {
			authorized++
			_, _ = w.Write([]byte("jpg"))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/snapshot?channel=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := NewTransport("admin", "secret").RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "jpg" || authorized != 1 {
		t.Fatalf("status=%d body=%q authorized=%d, want 200 \"jpg\" 1", resp.StatusCode, body, authorized)
	}
}

// 畸形质询必须以错误返回，不能 panic 拖垮调用方进程。
func TestRoundTripMalformedChallengeReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewTransport("admin", "secret").RoundTrip(req)
	if !errors.Is(err, ErrBadChallenge) {
		t.Fatalf("RoundTrip error = %v, want ErrBadChallenge", err)
	}
}
