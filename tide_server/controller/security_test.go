package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"
	"tide/tide_server/test"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

func doJSON(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, path, reader)
	require.NoError(t, err)
	addJsonContentHeader(req)
	if token != "" {
		addAuthorization(req, token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func login(t *testing.T, username, password string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, loginPath, strings.NewReader("username="+username+"&password="+password))
	require.NoError(t, err)
	addPostFormContentHeader(req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, ""
	}
	var resp loginTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.AccessToken
}

func userRole(t *testing.T, username string) (int, bool) {
	t.Helper()
	user, err := userManager.GetUser(username)
	if err != nil {
		return 0, false
	}
	return user.Role, true
}

// 待审批/被禁用的账号（role -1）既不能登录，也不能凭旧令牌访问接口，更不能走进
// EditUser 的管理员分支给自己造一个管理员。
func TestDisabledAccountCannotAuthenticateOrEscalate(t *testing.T) {
	const (
		pending  = "sec_pending"
		password = "pending-pass"
		minted   = "sec_minted_admin"
	)
	t.Cleanup(func() {
		_ = userManager.DelUser(pending)
		_ = userManager.DelUser(minted)
	})
	_ = userManager.DelUser(pending)
	_ = userManager.DelUser(minted)

	w := doJSON(t, http.MethodPost, "/applyAccount", "", auth.UserBaseInfo{Username: pending, Password: password})
	require.Equal(t, http.StatusOK, w.Code)

	// 同名再申请一次必须是明确的冲突，而不是 200 空响应。
	w = doJSON(t, http.MethodPost, "/applyAccount", "", auth.UserBaseInfo{Username: pending, Password: "other"})
	require.Equal(t, http.StatusConflict, w.Code)

	code, _ := login(t, pending, password)
	require.Equal(t, http.StatusUnauthorized, code, "a pending account must not be able to log in")

	// 审批通过后可以登录，拿到一个有效令牌。
	code, adminToken := login(t, test.AdminUsername, test.AdminPassword)
	require.Equal(t, http.StatusOK, code)
	w = doJSON(t, http.MethodPost, "/passApplication", adminToken, []string{pending})
	require.Equal(t, http.StatusOK, w.Code)
	code, userToken := login(t, pending, password)
	require.Equal(t, http.StatusOK, code)

	// 普通用户试图通过 /editUser 新建管理员：只会改到自己的基本信息。
	w = doJSON(t, http.MethodPost, "/editUser", userToken, auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: minted, Password: "x"},
		UserAuthority: auth.UserAuthority{Role: auth.Admin},
	})
	require.Equal(t, http.StatusOK, w.Code)
	_, exists := userRole(t, minted)
	require.False(t, exists, "a normal user must not be able to create accounts")
	// 上一步把自己的密码改成了 "x"（普通用户分支只认自己的用户名）。
	code, userToken = login(t, pending, "x")
	require.Equal(t, http.StatusOK, code)

	// 管理员把该用户禁用：旧令牌立即失效，重新登录也不行。
	w = doJSON(t, http.MethodPost, "/editUser", adminToken, auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: pending},
		UserAuthority: auth.UserAuthority{Role: auth.DisabledUser},
	})
	require.Equal(t, http.StatusOK, w.Code)
	role, exists := userRole(t, pending)
	require.True(t, exists)
	require.Equal(t, auth.DisabledUser, role)

	w = doJSON(t, http.MethodGet, "/userInfo", userToken, nil)
	require.Equal(t, http.StatusForbidden, w.Code, "a token issued before the account was disabled must stop working")

	// 关键：禁用账号凭旧令牌调用 /editUser 不得进入管理员分支。
	w = doJSON(t, http.MethodPost, "/editUser", userToken, auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: minted, Password: "x"},
		UserAuthority: auth.UserAuthority{Role: auth.Admin},
	})
	require.Equal(t, http.StatusForbidden, w.Code)
	_, exists = userRole(t, minted)
	require.False(t, exists, "a disabled account must not be able to mint an admin")

	code, _ = login(t, pending, "x")
	require.Equal(t, http.StatusUnauthorized, code, "a disabled account must not be able to log in again")
}

func TestPprofRequiresAdmin(t *testing.T) {
	w := doJSON(t, http.MethodGet, "/debug/pprof/cmdline", "", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	code, adminToken := login(t, test.AdminUsername, test.AdminPassword)
	require.Equal(t, http.StatusOK, code)
	w = doJSON(t, http.MethodGet, "/debug/pprof/cmdline", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequestBodyLimit(t *testing.T) {
	huge := `{"username":"sec_big","password":"` + strings.Repeat("a", maxRequestBodyBytes+1) + `"}`
	req, err := http.NewRequest(http.MethodPost, "/applyAccount", strings.NewReader(huge))
	require.NoError(t, err)
	addJsonContentHeader(req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	_, exists := userRole(t, "sec_big")
	require.False(t, exists)
}

func TestListUpstreamHidesPassword(t *testing.T) {
	truncateDB(t)
	upstream := db.Upstream{Username: "relay-user", Password: "relay-secret", Url: "http://127.0.0.1:1"}
	require.NoError(t, db.EditUpstream(&upstream))

	code, adminToken := login(t, test.AdminUsername, test.AdminPassword)
	require.Equal(t, http.StatusOK, code)
	w := doJSON(t, http.MethodGet, "/listUpstream", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "relay-secret")

	var got []db.Upstream
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 1)
	require.Equal(t, "relay-user", got[0].Username)
	require.Empty(t, got[0].Password)
}

// /editPermission 不带 scopes（或为 null）表示"清空权限"，不能被 hub 当成"不限制"。
func TestEditPermissionNullScopesRevokesLiveSubscriptions(t *testing.T) {
	const username = "sec_scopes"
	t.Cleanup(func() { _ = userManager.DelUser(username) })
	_ = userManager.DelUser(username)
	require.NoError(t, userManager.AddUser(auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: username, Password: "p"},
		UserAuthority: auth.UserAuthority{Role: auth.NormalUser},
	}))

	stationItem := common.StationItemStruct{StationId: uuid.New(), ItemName: "sec_item"}
	// 直接用 pubsub.NewSubscriber（没有后台转发 goroutine），消息留在 Ch 里便于断言。
	subscriber := pubsub.NewSubscriber(10, nil)
	hub.TrackSubscriber(username, subscriber, connTypeWebBrowser)
	t.Cleanup(func() { hub.UntrackSubscriber(username, subscriber) })
	hub.Subscribe(BrokerData, subscriber, uuidStringsMapToTopics(common.UUIDStringsMap{stationItem.StationId: {stationItem.ItemName}}))
	t.Cleanup(func() { hub.Unsubscribe(BrokerData, subscriber) })

	hub.Publish(BrokerData, "before", stationItem)
	require.Equal(t, "before", <-subscriber.Ch, "sanity check: the subscription is live before the edit")

	code, adminToken := login(t, test.AdminUsername, test.AdminPassword)
	require.Equal(t, http.StatusOK, code)
	w := doJSON(t, http.MethodPost, "/editPermission", adminToken, map[string]any{"username": username})
	require.Equal(t, http.StatusOK, w.Code)

	hub.Publish(BrokerData, "after", stationItem)
	require.Empty(t, subscriber.Ch, "clearing a user's permissions must drop their live data subscription")
}

// 删除/修改上游会取消 upstream.ctx；已经建立的 v1 中继会话必须随之结束，
// 不能继续从被删除的上游接收数据。
func TestHandleSyncClientConnStopsWhenUpstreamCanceled(t *testing.T) {
	truncateDB(t)
	upstream := db.Upstream{Username: "cancel-user", Password: "p", Url: "http://127.0.0.1:3"}
	require.NoError(t, db.EditUpstream(&upstream))

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
	t.Cleanup(func() { _ = serverConn.Close() })

	// 对端只维持 yamux 会话、接受流，但什么都不发：客户端会一直停在全量同步的第一次读上。
	serverSession, err := yamux.Server(serverConn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	go func() {
		for {
			if _, err := serverSession.Accept(); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleSyncClientConn(clientConn, &upstreamSyncState{config: upstream, ctx: ctx, cancel: cancel})
	}()

	select {
	case <-done:
		t.Fatal("the sync session ended before the upstream was canceled")
	case <-time.After(300 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceling the upstream did not end its established sync session")
	}
}

// v1 中继客户端连续收到的事件必须各自保有自己的内容：消息体按引用发布给订阅者，
// 解码缓冲区不能在两条消息之间复用。
func TestIncrementSyncConfigClientDoesNotAliasMessageBodies(t *testing.T) {
	truncateDB(t)
	upstream := db.Upstream{Username: "alias-user", Password: "p", Url: "http://127.0.0.1:2"}
	require.NoError(t, db.EditUpstream(&upstream))
	stationID := uuid.New()
	_, err := db.SyncStation(upstream.Id, db.Station{Id: stationID, Identifier: "alias-station", Name: "alias"})
	require.NoError(t, err)

	subscriber := pubsub.NewSubscriber(8, nil)
	received := subscriber.Ch
	hub.Subscribe(BrokerStatus, subscriber, nil)
	t.Cleanup(func() { hub.Unsubscribe(BrokerStatus, subscriber) })

	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		incrementSyncConfigClient(client, &upstreamSyncState{config: upstream})
	}()

	const events = 3
	encoder := json.NewEncoder(server)
	for i := 1; i <= events; i++ {
		// 每条事件的状态文本长度不同，复用缓冲区时旧内容会被覆盖或残留。
		require.NoError(t, encoder.Encode(SendMsgStruct{Type: kMsgUpdateStationStatus, Body: common.StationStatusStruct{
			StationId:  stationID,
			Identifier: "alias-station",
			StatusChangeStruct: common.StatusChangeStruct{
				Status:    strings.Repeat("s", i*7),
				ChangedAt: 1000,
			},
		}}))
	}
	_ = server.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("incrementSyncConfigClient did not return after the stream closed")
	}

	// 全部事件处理完之后再逐条检查：此时如果缓冲区被复用，前面的消息体已经被改写。
	for i := 1; i <= events; i++ {
		var raw any
		select {
		case raw = <-received:
		case <-time.After(2 * time.Second):
			t.Fatalf("event %d was not published", i)
		}
		msg, ok := raw.(SendMsgStruct)
		require.True(t, ok)
		payload, err := json.Marshal(msg.Body)
		require.NoErrorf(t, err, "event %d body is no longer valid JSON", i)
		var body common.StationStatusStruct
		require.NoError(t, json.Unmarshal(payload, &body))
		require.Equalf(t, strings.Repeat("s", i*7), body.Status, "event %d carries another event's body", i)
	}
}
