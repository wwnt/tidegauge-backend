package wsutil

import (
	"github.com/gorilla/websocket"
	"time"
)

type WsWrap struct {
	*websocket.Conn
}

func (ws WsWrap) Write(b []byte) (int, error) {
	if err := ws.Conn.WriteMessage(websocket.TextMessage, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (ws WsWrap) Ping(done <-chan struct{}) {
	_ = ws.SetReadDeadline(time.Now().Add(40 * time.Second))
	ws.SetPongHandler(func(string) error { _ = ws.SetReadDeadline(time.Now().Add(40 * time.Second)); return nil })
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)) != nil {
				return
			}
		case <-done:
			return
		}
	}
}
