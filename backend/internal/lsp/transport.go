package lsp

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait    = 5 * time.Second
	controlWait  = time.Second
	pingInterval = 20 * time.Second
	pongWait     = 60 * time.Second
)

func configureWebSocket(conn *websocket.Conn, liveness time.Duration) {
	conn.SetReadLimit(maxMessageBytes)
	conn.SetReadDeadline(time.Now().Add(liveness))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(liveness))
	})
}

// Control writes may run concurrently with the data writer. Only the reader
// updates read deadlines (via its pong handler), as required by gorilla.
func heartbeat(conn *websocket.Conn, stop <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(controlWait)); err != nil {
				return
			}
		}
	}
}
