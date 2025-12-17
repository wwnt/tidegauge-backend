package controller

import (
	"net/http"
	"time"

	"tide/common"
	"tide/pkg/pubsub"
	"tide/pkg/wsutil"
	"tide/tide_server/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write the file to the client.
	writeWait = 5 * time.Second
)

var (
	wsUnauthorized        = websocket.FormatCloseMessage(4001, http.StatusText(http.StatusUnauthorized))
	wsInternalServerError = websocket.FormatCloseMessage(4002, http.StatusText(http.StatusInternalServerError))
)

func DataWebsocket(c *gin.Context) {
	var (
		err      error
		wsw      = c.MustGet(contextKeyWsConn).(wsutil.WsWrap)
		username = c.GetString(contextKeyUsername)
	)

	subscriber := pubsub.NewSubscriber(c.Request.Context().Done(), wsw)

	addUserConn(username, subscriber, connTypeWebBrowser)
	defer func() {
		delUserConn(username, subscriber)
		dataPubSub.Evict(subscriber)
	}()
	go wsw.Ping(c.Request.Context().Done())

	// reader
	for {
		var msg map[uuid.UUID][]string
		if err = wsw.ReadJSON(&msg); err != nil {
			return
		}
		if len(msg) == 0 {
			dataPubSub.Evict(subscriber)
			continue
		}
		if c.GetInt(contextKeyRole) < auth.Admin {
			// user need to check the permissions
			for stationId, items := range msg {
				for _, itemName := range items {
					if !authorization.CheckPermission(username, stationId, itemName) {
						_ = wsw.WriteControl(websocket.CloseMessage, wsUnauthorized, time.Now().Add(writeWait))
						return
					}
				}
			}
		}
		topics := make(pubsub.TopicMap)
		for stationId, items := range msg {
			for _, item := range items {
				topics[common.StationItemStruct{StationId: stationId, ItemName: item}] = struct{}{}
			}
		}
		dataPubSub.SubscribeTopic(subscriber, topics)
	}
}

func GlobalWebsocket(c *gin.Context) {
	wsw := c.MustGet(contextKeyWsConn).(wsutil.WsWrap)

	subscriber := pubsub.NewSubscriber(c.Request.Context().Done(), wsw)

	defer statusPubSub.Evict(subscriber)
	statusPubSub.SubscribeTopic(subscriber, nil)

	go wsw.Ping(c.Request.Context().Done())
	// reader
	for {
		if _, _, err := wsw.ReadMessage(); err != nil {
			return
		}
	}
}
