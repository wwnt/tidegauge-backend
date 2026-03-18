package controller

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
	"tide/tide_server/auth"
)

type requestContextKey int

const (
	reqCtxWSConn requestContextKey = iota + 1
	reqCtxUserInfo
	reqCtxEmail
	reqCtxUsername
	reqCtxRole
	reqCtxLiveCamera
)

func withWSConn(r *http.Request, ws *websocket.Conn) *http.Request {
	ctx := context.WithValue(r.Context(), reqCtxWSConn, ws)
	return r.WithContext(ctx)
}

func requestWSConn(r *http.Request) (*websocket.Conn, bool) {
	v := r.Context().Value(reqCtxWSConn)
	ws, ok := v.(*websocket.Conn)
	return ws, ok
}

func withRequestUser(r *http.Request, username string, user auth.User) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, reqCtxUserInfo, user)
	ctx = context.WithValue(ctx, reqCtxUsername, username)
	ctx = context.WithValue(ctx, reqCtxRole, user.Role)
	ctx = context.WithValue(ctx, reqCtxEmail, user.Email)
	if user.Role == auth.NormalUser {
		ctx = context.WithValue(ctx, reqCtxLiveCamera, user.LiveCamera)
	}
	return r.WithContext(ctx)
}

func requestUserInfo(r *http.Request) (auth.User, bool) {
	v := r.Context().Value(reqCtxUserInfo)
	user, ok := v.(auth.User)
	return user, ok
}

func requestUsername(r *http.Request) string {
	v := r.Context().Value(reqCtxUsername)
	s, _ := v.(string)
	return s
}

func requestRole(r *http.Request) int {
	v := r.Context().Value(reqCtxRole)
	n, _ := v.(int)
	return n
}

func requestEmail(r *http.Request) string {
	v := r.Context().Value(reqCtxEmail)
	s, _ := v.(string)
	return s
}

func requestLiveCamera(r *http.Request) bool {
	v := r.Context().Value(reqCtxLiveCamera)
	b, _ := v.(bool)
	return b
}
