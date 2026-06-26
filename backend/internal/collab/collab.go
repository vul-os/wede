// Package collab provides the per-room collaboration WebSocket. For now it
// carries presence (who is connected, what file/line each person is viewing);
// the CRDT document channel is layered on the same socket in a later wave.
//
// The handler is a thin pump over the already-tested presence.Hub: a write loop
// drains the member's outbound roster channel to the socket, and a read loop
// turns inbound {"type":"cursor", "file", "line"} messages into hub.Update.
package collab

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"wede/backend/internal/presence"
)

const (
	pingInterval = 30 * time.Second
	readLimit    = 8 << 10 // 8 KiB — cursor messages are tiny
)

// Handler upgrades collaboration WebSockets for a single room's presence hub.
type Handler struct {
	hub      *presence.Hub
	upgrader websocket.Upgrader
}

// New builds a collab handler bound to a room's presence hub. frameAncestors
// mirrors the terminal/lsp origin-check behaviour (space-separated origins).
func New(frameAncestors string, hub *presence.Hub) *Handler {
	allowed := parseOrigins(frameAncestors)
	return &Handler{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return checkOrigin(r, allowed) },
		},
	}
}

// HandleWS serves GET /api/rooms/{id}/collab. Auth is validated by the protected
// mux before this point; the "auth.<token>" subprotocol is echoed so the browser
// handshake succeeds (token never appears in the URL).
func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	var chosen string
	for _, p := range websocket.Subprotocols(r) {
		if strings.HasPrefix(p, "auth.") {
			chosen = p
			break
		}
	}
	var hdr http.Header
	if chosen != "" {
		hdr = http.Header{"Sec-Websocket-Protocol": {chosen}}
	}

	conn, err := h.upgrader.Upgrade(w, r, hdr)
	if err != nil {
		log.Println("[collab] websocket upgrade error:", err)
		return
	}

	username := r.URL.Query().Get("username")
	id, out := h.hub.Join(username)
	if id == "" { // hub already closed (room shutting down)
		conn.Close()
		return
	}

	stop := make(chan struct{})
	go writePump(conn, out, stop)

	// Read loop: parse cursor updates until the socket closes.
	conn.SetReadLimit(readLimit)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if file, line, ok := parseCursor(msg); ok {
			h.hub.Update(id, file, line)
			continue
		}
		// Ephemeral peer signals (mouse / window) are relayed to everyone else,
		// tagged with the sender id so the client can attribute them via the roster.
		if relayed, ok := tagRelay(id, msg); ok {
			h.hub.RelayExcept(id, relayed)
		}
	}

	close(stop)      // stop the write pump promptly
	h.hub.Leave(id)  // remove from roster (closes out)
	conn.Close()
}

// writePump forwards roster events to the socket and keeps it alive with pings.
func writePump(conn *websocket.Conn, out <-chan []byte, stop <-chan struct{}) {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-stop:
			return
		case data, ok := <-out:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// inbound is the client→server message shape on the collab socket.
type inbound struct {
	Type string `json:"type"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// parseCursor extracts a cursor update from a raw message. Returns ok=false for
// malformed JSON or any non-cursor message type.
func parseCursor(msg []byte) (file string, line int, ok bool) {
	var in inbound
	if err := json.Unmarshal(msg, &in); err != nil {
		return "", 0, false
	}
	if in.Type != "cursor" {
		return "", 0, false
	}
	return in.File, in.Line, true
}

// tagRelay re-tags an ephemeral message (mouse/window) with the sender id so it
// can be fanned out to peers. Returns ok=false for any other message type.
func tagRelay(senderID string, msg []byte) ([]byte, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(msg, &m); err != nil {
		return nil, false
	}
	raw, ok := m["type"]
	if !ok {
		return nil, false
	}
	var t string
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, false
	}
	if t != "mouse" && t != "window" {
		return nil, false
	}
	idBytes, err := json.Marshal(senderID)
	if err != nil {
		return nil, false
	}
	m["id"] = idBytes
	out, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return out, true
}
