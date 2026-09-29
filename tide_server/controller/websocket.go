package controller

import (
	"context"
	"net/http"
	"time"

	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

const (
	wsStatusUnauthorized        websocket.StatusCode = 4001
	wsStatusInternalServerError websocket.StatusCode = 4002
)

const (
	wsWriteTimeout = 5 * time.Second
	wsPingInterval = 30 * time.Second
	wsPingTimeout  = 5 * time.Second
)

// 消息读限制保持 coder/websocket 的默认值（32 KiB，见 read.go 的 defaultReadLimit）：
// 浏览器端唯一会发来的消息是 DataWebsocket 的订阅表，实测单站全部数据项的
// 订阅 JSON 不足 1 KiB，几十倍余量足够；不额外调用 SetReadLimit。
//
// 两个 handler 的连接 context 都用 context.WithCancel(context.Background()) 派生，刻意
// 不用 r.Context()：websocket.Accept 内部会 Hijack 连接（accept.go:159），net/http 在
// handler 返回后立刻 cancel 该请求的 ctx（server.go:2075），且 coder/websocket 明确警告
// Accept 之后再使用 request ctx 行为不可预期（accept.go:100）。连接生命周期因此自行管理。

// wsHubJSONWriter 用订阅者的 context 给每次写加超时，避免慢客户端把写 goroutine 拖住。
func wsHubJSONWriter(ctx context.Context, ws *websocket.Conn) func(any) error {
	return func(val any) error {
		writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
		defer cancel()
		return wsjson.Write(writeCtx, ws, val)
	}
}

// runWebsocketPing 周期性向对端发送 ping，有两个作用：
//
//  1. 复位反向代理的空闲超时（本部署里的主要作用）。站点部署在 nginx 后面
//     （tide_server/tidegauge.nginx.conf 代理到 :7100，且未覆盖 proxy_read_timeout），
//     nginx 的 proxy_read_timeout 默认 60s；其官方 WebSocket 文档明确写着"被代理服务器
//     60 秒内没有发送任何数据就会关闭连接"，并建议由后端定期发送 ping 帧来复位该超时。
//     这里 30s < 60s，留了一倍余量。注意 TCP keepalive 不能替代：保活探测由内核消费、
//     不会作为数据交给 nginx，因此不会复位 proxy_read_timeout。删掉本函数后，长时间
//     没有数据推送的连接（尤其 /ws/global）会被 nginx 约每 60 秒掐断一次，前端只能反复重连。
//  2. 探测半开连接：ping 失败即 cancel 连接 context，让服务端及时回收订阅者。
//
// 与读循环是配套关系，不能单独使用：coder/websocket 的 Ping 自己并不读连接，
// 它等待某次 Reader 调用把 pong 读回来（conn.go:208）。两个 handler 都有并发的
// 读循环（DataWebsocket 的 wsjson.Read、GlobalWebsocket 的 wsw.Read），pong 才有
// 人去消费。反过来说，ctx 一旦被 cancel，库内部的 setupReadTimeout 会直接关闭底层
// 连接（read.go:232），阻塞中的读循环随之返回——这正是写失败后无需再额外
// CloseNow() 的原因。
//
// 周期与超时由调用方传入（生产用 wsPingInterval/wsPingTimeout），测试可传毫秒级值，
// 无需等待 30 秒即可覆盖半开检测。
func runWebsocketPing(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, interval, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
			err := ws.Ping(pingCtx)
			pingCancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

func DataWebsocket(w http.ResponseWriter, r *http.Request) {
	wsw, ok := requestWSConn(r)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	username := requestUsername(r)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subscriber := hub.NewSubscriber(ctx, cancel, wsHubJSONWriter(ctx, wsw))
	hub.TrackSubscriber(username, subscriber, connTypeWebBrowser)
	defer func() {
		hub.UntrackSubscriber(username, subscriber)
		hub.Unsubscribe(BrokerData, subscriber)
	}()
	go runWebsocketPing(ctx, cancel, wsw, wsPingInterval, wsPingTimeout)

	// reader
	for {
		var msg map[uuid.UUID][]string
		if err := wsjson.Read(ctx, wsw, &msg); err != nil {
			return
		}
		if len(msg) == 0 {
			hub.Unsubscribe(BrokerData, subscriber)
			continue
		}
		if requestRole(r) < auth.Admin {
			// user need to check the permissions
			for stationId, items := range msg {
				for _, itemName := range items {
					if !authorization.CheckPermission(username, stationId, itemName) {
						_ = wsw.Close(wsStatusUnauthorized, http.StatusText(http.StatusUnauthorized))
						return
					}
				}
			}
		}
		subscribedTopics := make(pubsub.TopicSet)
		for stationId, items := range msg {
			for _, item := range items {
				subscribedTopics[common.StationItemStruct{StationId: stationId, ItemName: item}] = struct{}{}
			}
		}
		hub.Subscribe(BrokerData, subscriber, subscribedTopics)
	}
}

func GlobalWebsocket(w http.ResponseWriter, r *http.Request) {
	wsw, ok := requestWSConn(r)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	username := requestUsername(r)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subscriber := hub.NewSubscriber(ctx, cancel, wsHubJSONWriter(ctx, wsw))
	hub.TrackSubscriber(username, subscriber, connTypeWebBrowser)
	defer func() {
		hub.UntrackSubscriber(username, subscriber)
		hub.Unsubscribe(BrokerStatus, subscriber)
	}()

	hub.Subscribe(BrokerStatus, subscriber, nil)
	go runWebsocketPing(ctx, cancel, wsw, wsPingInterval, wsPingTimeout)

	// 读循环必须常驻：它既消费控制帧（pong/close，见 runWebsocketPing 的说明），
	// 也让协议对端发来的数据帧不会堆积。本端点不接收客户端消息，读到即丢弃。
	for {
		if _, _, err := wsw.Read(ctx); err != nil {
			return
		}
	}
}
