package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"tide/common"
	"tide/pkg/pubsub"
	"tide/pkg/wsutil"
	"tide/tide_server/auth"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"go.uber.org/zap"
)

const (
	// Time allowed to write the file to the client.
	writeWait = 5 * time.Second
)

var (
	wsUnauthorized        = websocket.FormatCloseMessage(4001, http.StatusText(http.StatusUnauthorized))
	wsInternalServerError = websocket.FormatCloseMessage(4002, http.StatusText(http.StatusInternalServerError))
	wsStationDisconnected = websocket.FormatCloseMessage(4003, "Station Disconnected")
)

func PortTerminalWebsocket(c *gin.Context) {
	wsw := c.MustGet(contextKeyWsConn).(wsutil.WsWrap)

	s, ok := c.GetQuery("station_id")
	if !ok {
		return
	}
	stationId, err := uuid.Parse(s)
	if err != nil {
		return
	}
	value, ok := recvConnections.Load(stationId)
	if !ok {
		_ = wsw.WriteControl(websocket.CloseMessage, wsStationDisconnected, time.Now().Add(writeWait))
		return
	}
	stationConn, err := value.(*yamux.Session).Open()
	if err != nil {
		logger.Error(err.Error())
		return
	}
	defer func() {
		_ = stationConn.Close()
	}()

	if _, err := stationConn.Write([]byte{common.MsgPortTerminal}); err != nil {
		logger.Error(err.Error())
		return
	}

	go wsw.Ping(c.Request.Context().Done())
	// writer
	go func() {
		var err error
		stationDecoder := json.NewDecoder(stationConn)
		for {
			var msg json.RawMessage
			if err = stationDecoder.Decode(&msg); err != nil {
				if err != io.EOF {
					logger.Error("portTerminal conn", zap.Error(err))
				}
				_ = wsw.WriteControl(websocket.CloseMessage, wsStationDisconnected, time.Now().Add(writeWait))
				return
			}
			if err = wsw.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()
	// reader
	for {
		_, p, err := wsw.ReadMessage()
		if err != nil {
			return
		}
		if _, err = stationConn.Write(p); err != nil {
			return
		}
	}
}

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
