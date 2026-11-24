// Copyright 2013 M-Lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package digest

import (
	"crypto/md5"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
)

var (
	ErrNilTransport      = errors.New("transport is nil")
	ErrBadChallenge      = errors.New("challenge is bad")
	ErrAlgNotImplemented = errors.New("algorithm not implemented")
)

// Transport is an implementation of http.RoundTripper that supports HTTP
// digest authentication.
type Transport struct {
	Username  string
	Password  string
	Transport http.RoundTripper
}

// NewTransport creates a new digest transport using http.DefaultTransport.
func NewTransport(username, password string) *Transport {
	return &Transport{
		Username:  username,
		Password:  password,
		Transport: http.DefaultTransport,
	}
}

// RoundTrip makes a request expecting a 401 response that will require digest
// authentication. It creates the credentials it needs and makes a follow-up
// request.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.Transport == nil {
		return nil, ErrNilTransport
	}

	// GetBody 只在请求带可重放的 body 时才有；普通 GET 为 nil，不能无条件调用。
	var body io.ReadCloser
	if r.GetBody != nil {
		var err error
		if body, err = r.GetBody(); err != nil {
			return nil, err
		}
	}
	// 重试请求沿用原请求的 context，调用方设置的超时/取消对第二次请求同样生效。
	req, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header = make(http.Header)
	maps.Copy(req.Header, r.Header)

	// Make a request to get the 401 that contains the challenge.
	res, err := t.Transport.RoundTrip(r)
	if err != nil || res.StatusCode != 401 {
		return res, err
	}
	defer func() { _ = res.Body.Close() }()

	chal := res.Header.Get("WWW-Authenticate")
	c, err := parseChallenge(chal)
	if err != nil {
		return res, err
	}

	// Generate credentials based on the challenge.
	cr := t.authenticate(req, c)
	auth, err := cr.authorize()
	if err != nil {
		return res, err
	}

	// Make authenticated request.
	req.Header.Set("Authorization", auth)
	return t.Transport.RoundTrip(req)
}

type challenge struct {
	realm     string
	domain    string
	nonce     string
	opaque    string
	stale     string
	algorithm string
	qop       string
}

// parseChallenge 解析 WWW-Authenticate 的 Digest 质询。
//
// 指令按"引号外的逗号"切分（qop="auth,auth-int" 这类带逗号的引号值不能被拆开）；
// 没有 '=' 的指令视为格式错误返回 ErrBadChallenge，而不是越界 panic；
// 不认识的指令（RFC 7616 的 charset/userhash 等扩展）直接忽略。
func parseChallenge(input string) (*challenge, error) {
	const ws = " \n\r\t"
	const scheme = "Digest "
	s := strings.Trim(input, ws)
	if len(s) < len(scheme) || !strings.EqualFold(s[:len(scheme)], scheme) {
		return nil, ErrBadChallenge
	}
	c := &challenge{
		algorithm: "MD5",
	}
	for _, directive := range splitDirectives(s[len(scheme):]) {
		key, value, ok := strings.Cut(directive, "=")
		if !ok {
			return nil, ErrBadChallenge
		}
		key = strings.ToLower(strings.Trim(key, ws))
		value = strings.Trim(strings.Trim(value, ws), `"`)
		switch key {
		case "realm":
			c.realm = value
		case "domain":
			c.domain = value
		case "nonce":
			c.nonce = value
		case "opaque":
			c.opaque = value
		case "stale":
			c.stale = value
		case "algorithm":
			c.algorithm = strings.ToUpper(value)
		case "qop":
			c.qop = selectQop(value)
		}
	}
	if c.nonce == "" {
		return nil, ErrBadChallenge
	}
	return c, nil
}

// splitDirectives 按引号外的逗号切分指令列表，丢弃空段。
func splitDirectives(s string) []string {
	var (
		parts    []string
		inQuotes bool
		start    int
	)
	appendPart := func(part string) {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuotes {
				i++ // 引号内的转义字符，跳过下一个字节
			}
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				appendPart(s[start:i])
				start = i + 1
			}
		}
	}
	appendPart(s[start:])
	return parts
}

// selectQop 从服务端给出的 qop 选项列表里选出本实现支持的 "auth"；
// 列表里没有 "auth" 时原样返回，由 authorize 报 ErrAlgNotImplemented。
func selectQop(options string) string {
	for option := range strings.SplitSeq(options, ",") {
		if strings.EqualFold(strings.TrimSpace(option), "auth") {
			return "auth"
		}
	}
	return strings.TrimSpace(options)
}

type credentials struct {
	userhash  bool
	username  string
	realm     string
	nonce     string
	uri       string
	algorithm string
	cnonce    string
	opaque    string
	qop       string
	nc        int
	method    string
	password  string
}

func h(data string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(data)))
}

func (c *credentials) ha1() string {
	return h(fmt.Sprintf("%s:%s:%s", c.username, c.realm, c.password))
}

func (c *credentials) ha2() string {
	return h(fmt.Sprintf("%s:%s", c.method, c.uri))
}

func (c *credentials) response(cnonce string) (string, error) {
	c.nc++
	if c.qop == "auth" {
		if cnonce != "" {
			c.cnonce = cnonce
		} else {
			b := make([]byte, 8)
			_, _ = io.ReadFull(rand.Reader, b)
			c.cnonce = fmt.Sprintf("%x", b)[:16]
		}
		return h(fmt.Sprintf("%s:%s:%08x:%s:%s:%s",
			c.ha1(), c.nonce, c.nc, c.cnonce, c.qop, c.ha2())), nil
	} else if c.qop == "" {
		return h(fmt.Sprintf("%s:%s:%s", c.ha1(), c.nonce, c.ha2())), nil
	}
	return "", ErrAlgNotImplemented
}

func (c *credentials) authorize() (string, error) {
	if c.algorithm != "MD5" {
		return "", ErrAlgNotImplemented
	}
	if c.qop != "auth" && c.qop != "" {
		return "", ErrAlgNotImplemented
	}
	response, err := c.response("")
	if err != nil {
		return "", err
	}
	var sl []string
	sl = append(sl, fmt.Sprintf(`username="%s"`, c.username))
	sl = append(sl, fmt.Sprintf(`realm="%s"`, c.realm))
	sl = append(sl, fmt.Sprintf(`nonce="%s"`, c.nonce))
	sl = append(sl, fmt.Sprintf(`uri="%s"`, c.uri))
	sl = append(sl, fmt.Sprintf(`response="%s"`, response))
	sl = append(sl, fmt.Sprintf(`algorithm="%s"`, c.algorithm))
	sl = append(sl, fmt.Sprintf(`cnonce="%s"`, c.cnonce))
	if c.opaque != "" {
		sl = append(sl, fmt.Sprintf(`opaque="%s"`, c.opaque))
	}
	if c.qop != "" {
		sl = append(sl, fmt.Sprintf(`qop=%s`, c.qop))
	}
	sl = append(sl, fmt.Sprintf("nc=%08x", c.nc))
	if c.userhash {
		sl = append(sl, `userhash="true"`)
	}
	return fmt.Sprintf("Digest %s", strings.Join(sl, ", ")), nil
}

func (t *Transport) authenticate(req *http.Request, c *challenge) *credentials {
	return &credentials{
		username:  t.Username,
		realm:     c.realm,
		nonce:     c.nonce,
		uri:       req.URL.RequestURI(),
		algorithm: c.algorithm,
		opaque:    c.opaque,
		qop:       c.qop,
		nc:        0,
		method:    req.Method,
		password:  t.Password,
	}
}
