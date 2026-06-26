// Package presence tracks who is connected to a room and what each person is
// looking at (file + cursor line), for multiplayer awareness.
//
// The Hub is transport-agnostic: each member has an outbound channel of JSON
// event bytes. The collab WebSocket handler pumps that channel to the socket and
// feeds incoming cursor/file updates back via Update. Keeping the hub free of any
// net/http or websocket dependency makes it directly unit-testable.
//
// On any membership or cursor change the hub broadcasts the full roster. For a
// small team sharing one host this is simpler and plenty efficient versus deltas.
package presence

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Member is one connected participant in a room.
type Member struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Color    string `json:"color"`
	File     string `json:"file,omitempty"` // path currently viewed (relative to room root)
	Line     int    `json:"line,omitempty"` // cursor line in that file
}

// rosterEvent is the message broadcast to every member's outbound channel.
type rosterEvent struct {
	Type    string   `json:"type"` // always "presence"
	Members []Member `json:"members"`
}

// palette gives each member a stable, distinct color by join order.
var palette = []string{
	"#f87171", "#fb923c", "#fbbf24", "#34d399",
	"#22d3ee", "#60a5fa", "#a78bfa", "#f472b6",
}

type client struct {
	member Member
	out    chan []byte
}

// Hub is a room's presence registry. Safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*client
	counter uint64
	closed  bool
}

// NewHub returns an empty presence hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[string]*client)}
}

// Join registers a participant and returns its assigned id plus an outbound
// channel that will receive roster events (including the one triggered by this
// join). The channel is buffered; if a consumer falls behind, events are dropped
// rather than blocking the hub.
func (h *Hub) Join(username string) (string, <-chan []byte) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		ch := make(chan []byte)
		close(ch)
		return "", ch
	}
	h.counter++
	id := fmt.Sprintf("m%d", h.counter)
	if username == "" {
		username = "anon"
	}
	c := &client{
		member: Member{
			ID:       id,
			Username: username,
			Color:    palette[(h.counter-1)%uint64(len(palette))],
		},
		out: make(chan []byte, 16),
	}
	h.clients[id] = c
	h.mu.Unlock()

	h.broadcast()
	return id, c.out
}

// Leave removes a participant and notifies the rest.
func (h *Hub) Leave(id string) {
	h.mu.Lock()
	c, ok := h.clients[id]
	if ok {
		delete(h.clients, id)
		close(c.out)
	}
	h.mu.Unlock()
	if ok {
		h.broadcast()
	}
}

// Update sets the file + cursor line a member is viewing and rebroadcasts.
func (h *Hub) Update(id, file string, line int) {
	h.mu.Lock()
	c, ok := h.clients[id]
	if ok {
		c.member.File = file
		c.member.Line = line
	}
	h.mu.Unlock()
	if ok {
		h.broadcast()
	}
}

// Roster returns a snapshot of current members (unordered).
func (h *Hub) Roster() []Member {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.snapshot()
}

// snapshot builds a member slice; caller must hold at least a read lock.
func (h *Hub) snapshot() []Member {
	out := make([]Member, 0, len(h.clients))
	for _, c := range h.clients {
		out = append(out, c.member)
	}
	return out
}

// broadcast sends the current roster to every member's channel (non-blocking).
func (h *Hub) broadcast() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	data, err := json.Marshal(rosterEvent{Type: "presence", Members: h.snapshot()})
	if err != nil {
		return
	}
	for _, c := range h.clients {
		select {
		case c.out <- data:
		default: // drop for a slow consumer rather than block the hub
		}
	}
}

// Close shuts the hub down, closing all outbound channels. Idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for id, c := range h.clients {
		close(c.out)
		delete(h.clients, id)
	}
}
