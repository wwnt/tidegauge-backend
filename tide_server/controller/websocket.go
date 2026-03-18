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
	// Time allowed to write the file to the client.
	wsStatusUnauthorized        websocket.StatusCode = 4001
	wsStatusInternalServerError websocket.StatusCode = 4002
)

const wsWriteTimeout = 5 * time.Second

func wsHubJSONWriter(ctx context.Context, ws *websocket.Conn) func(any) error {
	return func(val any) error {
		writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
		defer cancel()
		return wsjson.Write(writeCtx, ws, val)
	}
}

func DataWebsocket(w http.ResponseWriter, r *http.Request) {
	wsw, ok := requestWSConn(r)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	username := requestUsername(r)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	subscriber := hub.NewSubscriber(ctx, cancel, wsHubJSONWriter(ctx, wsw))
	go func() { <-ctx.Done(); _ = wsw.CloseNow() }()
	hub.TrackSubscriber(username, subscriber, connTypeWebBrowser)
	defer func() {
		hub.UntrackSubscriber(username, subscriber)
		hub.Unsubscribe(BrokerData, subscriber)
	}()

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

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		defer cancel()
		for {
			if _, _, err := wsw.Read(ctx); err != nil {
				return
			}
		}
	}()

	subscriber := hub.NewSubscriber(ctx, cancel, wsHubJSONWriter(ctx, wsw))
	hub.TrackSubscriber(username, subscriber, connTypeWebBrowser)
	defer func() {
		hub.UntrackSubscriber(username, subscriber)
		hub.Unsubscribe(BrokerStatus, subscriber)
	}()

	hub.Subscribe(BrokerStatus, subscriber, nil)

	<-ctx.Done()
}
