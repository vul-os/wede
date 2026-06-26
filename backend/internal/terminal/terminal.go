package terminal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type WorkspaceProvider interface {
	Current() string
	OnChange(func(string))
}

// subscriber is one websocket attached to a shared terminal session. Each has
// its own write mutex so concurrent broadcasts to different subscribers don't
// serialize through a single lock, while writes to the *same* socket stay ordered.
type subscriber struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

// write serializes a single message to this subscriber's socket.
func (sub *subscriber) write(msgType int, data []byte) error {
	sub.wmu.Lock()
	defer sub.wmu.Unlock()
	return sub.conn.WriteMessage(msgType, data)
}

// session holds a persistent pty shared by all subscribers in a room. The pty
// survives websocket reconnects and is fanned out to every connected subscriber.
type session struct {
	id     string
	ptmx   *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex              // guards subs + closed
	subs   map[*subscriber]struct{} // all connected viewers (shared terminal)
	pmu    sync.Mutex              // serializes input writes to the pty
	buf    *ringBuffer             // scrollback buffer for replay on (re)connect
	done   chan struct{}
	closed bool
}

// addSub registers a websocket as a subscriber and returns its handle.
func (s *session) addSub(conn *websocket.Conn) *subscriber {
	sub := &subscriber{conn: conn}
	s.mu.Lock()
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	return sub
}

// removeSub detaches a subscriber (does not kill the pty — the session persists).
func (s *session) removeSub(sub *subscriber) {
	s.mu.Lock()
	delete(s.subs, sub)
	s.mu.Unlock()
}

// subCount reports how many viewers are currently attached.
func (s *session) subCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

// broadcast writes data to every subscriber, pruning any that error. Subscribers
// are snapshotted under the lock, then written to outside it so a slow/blocked
// socket can't stall the pty reader while holding s.mu.
func (s *session) broadcast(msgType int, data []byte) {
	s.mu.Lock()
	subs := make([]*subscriber, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()

	var dead []*subscriber
	for _, sub := range subs {
		if err := sub.write(msgType, data); err != nil {
			dead = append(dead, sub)
		}
	}
	if len(dead) > 0 {
		s.mu.Lock()
		for _, sub := range dead {
			delete(s.subs, sub)
			sub.conn.Close()
		}
		s.mu.Unlock()
	}
}

// closeConns closes and clears all subscriber sockets. Caller must hold s.mu.
func (s *session) closeConns() {
	for sub := range s.subs {
		sub.conn.Close()
	}
	s.subs = make(map[*subscriber]struct{})
}

// ringBuffer stores the last N bytes of terminal output for replay on reconnect.
type ringBuffer struct {
	mu   sync.Mutex
	data []byte
	max  int
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{data: make([]byte, 0, size), max: size}
}

func (rb *ringBuffer) Write(p []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.data = append(rb.data, p...)
	if len(rb.data) > rb.max {
		rb.data = rb.data[len(rb.data)-rb.max:]
	}
}

func (rb *ringBuffer) Bytes() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	out := make([]byte, len(rb.data))
	copy(out, rb.data)
	return out
}

// parseOrigins splits a space-separated frame_ancestors value into a set of
// allowed origin strings.  Each entry should be an origin like
// "https://vulos.org" (no trailing slash, no path).
func parseOrigins(frameAncestors string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, o := range strings.Fields(frameAncestors) {
		set[o] = struct{}{}
	}
	return set
}

// checkOrigin enforces WebSocket origin validation.
//
//   - No Origin header (e.g. a native tool or curl) → allowed (non-browser client).
//   - Origin matches the Host header → same-origin → allowed.
//   - Origin is in the allowedOrigins set (from frame_ancestors config) → allowed
//     so that the Vulos OS shell, which legitimately embeds wede in an iframe,
//     can also open the terminal WebSocket.
//   - Anything else → rejected.
func checkOrigin(r *http.Request, allowedOrigins map[string]struct{}) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header: non-browser client (e.g. curl, native app). Allow.
		return true
	}

	// Derive the expected same-origin value from the Host header.
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// X-Forwarded-Proto from a trusted reverse proxy takes precedence.
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	selfOrigin := scheme + "://" + r.Host
	if origin == selfOrigin {
		return true
	}

	// Allowed cross-origin embedding (e.g. Vulos shell).
	if _, ok := allowedOrigins[origin]; ok {
		return true
	}

	log.Printf("[terminal] rejected WebSocket upgrade from origin %q (host=%s)", origin, r.Host)
	return false
}

type Handler struct {
	ws       WorkspaceProvider
	mu       sync.Mutex
	sessions map[string]*session
	upgrader websocket.Upgrader
}

// New creates a terminal handler.  allowedOrigins is the space-separated list
// from the frame_ancestors config (e.g. "https://vulos.org").  When empty,
// only same-origin WebSocket upgrades are allowed.
func New(ws WorkspaceProvider, allowedOrigins string) *Handler {
	allowed := parseOrigins(allowedOrigins)
	h := &Handler{
		ws:       ws,
		sessions: make(map[string]*session),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return checkOrigin(r, allowed)
			},
		},
	}
	// Kill all terminal sessions when workspace changes so new ones open in the new directory
	ws.OnChange(func(string) {
		h.mu.Lock()
		for id, s := range h.sessions {
			s.mu.Lock()
			s.closeConns()
			s.ptmx.Close()
			s.cmd.Process.Kill()
			s.closed = true
			s.mu.Unlock()
			delete(h.sessions, id)
		}
		h.mu.Unlock()
	})
	return h
}

// Close terminates every terminal session owned by this handler — closing the
// active websocket, the PTY, and killing the shell process. Called when the
// owning room is closed.
func (h *Handler) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, s := range h.sessions {
		s.mu.Lock()
		s.closeConns()
		if s.ptmx != nil {
			s.ptmx.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Kill() //nolint:errcheck
		}
		s.closed = true
		s.mu.Unlock()
		delete(h.sessions, id)
	}
}

func (h *Handler) getOrCreateSession(id string) (*session, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if s, ok := h.sessions[id]; ok && !s.closed {
		log.Printf("[terminal] reattaching to existing session %q (buf=%d bytes)", id, len(s.buf.data))
		return s, true, nil
	}
	log.Printf("[terminal] creating new session %q (existing sessions: %d)", id, len(h.sessions))

	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
		} else {
			shell = "/bin/bash"
		}
	}

	dir := h.ws.Current()
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home
	}

	cmd := exec.Command(shell, "-l")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, false, err
	}

	s := &session{
		id:   id,
		ptmx: ptmx,
		cmd:  cmd,
		subs: make(map[*subscriber]struct{}),
		buf:  newRingBuffer(64 * 1024), // 64KB scrollback
		done: make(chan struct{}),
	}
	h.sessions[id] = s

	// pty reader goroutine — reads from the pty and fans output out to every
	// connected subscriber (shared terminal), buffering for late-joiner replay.
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				s.mu.Lock()
				s.closed = true
				s.closeConns()
				s.mu.Unlock()
				close(s.done)
				h.mu.Lock()
				delete(h.sessions, id)
				h.mu.Unlock()
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			s.buf.Write(data)
			s.broadcast(websocket.BinaryMessage, data)
		}
	}()

	return s, false, nil
}

// ListSessions returns active session IDs via HTTP.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	ids := make([]string, 0, len(h.sessions))
	for id, s := range h.sessions {
		if !s.closed {
			ids = append(ids, id)
		}
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"sessions": ids})
}

func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	// The auth token is conveyed via the "auth.<token>" WebSocket subprotocol so
	// it never appears in server access logs.  The Middleware already validated
	// the request before this handler is reached; we echo the chosen subprotocol
	// back to the client so the browser's WebSocket handshake succeeds.
	var chosenProto string
	for _, p := range websocket.Subprotocols(r) {
		if strings.HasPrefix(p, "auth.") {
			chosenProto = p
			break
		}
	}
	var upgradeHeader http.Header
	if chosenProto != "" {
		upgradeHeader = http.Header{"Sec-Websocket-Protocol": {chosenProto}}
	}

	conn, err := h.upgrader.Upgrade(w, r, upgradeHeader)
	if err != nil {
		log.Println("websocket upgrade error:", err)
		return
	}

	// Session ID from query param only (token no longer passed in URL).
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		// Fallback: derive session from the token subprotocol value so existing
		// single-tab sessions continue to work even without an explicit session param.
		if chosenProto != "" {
			sessionID = strings.TrimPrefix(chosenProto, "auth.")
		}
	}

	log.Printf("[terminal] ws connect: session=%q", sessionID)
	s, reconnected, err := h.getOrCreateSession(sessionID)
	if err != nil {
		log.Println("pty start error:", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Error starting terminal: "+err.Error()))
		conn.Close()
		return
	}

	// Attach this connection as a subscriber (shared terminal — existing viewers
	// stay connected). _ = reconnected: replay is now unconditional below.
	_ = reconnected
	sub := s.addSub(conn)

	// Replay scrollback so a (re)joining viewer immediately sees the current
	// screen state. Harmless for a brand-new session (empty buffer).
	if replay := s.buf.Bytes(); len(replay) > 0 {
		sub.write(websocket.BinaryMessage, replay) //nolint:errcheck
	}

	// Per-connection ping/pong keepalive.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
		return nil
	})
	connDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sub.write(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-connDone:
				return
			case <-s.done:
				return
			}
		}
	}()

	// websocket -> pty. Any subscriber may type (multi-writer); pty writes are
	// serialized via s.pmu. Resize uses a last-writer-wins policy: the most
	// recent client's dimensions apply to the shared pty.
	log.Printf("[terminal] session %q: subscriber attached (viewers=%d)", sessionID, s.subCount())
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			// This viewer disconnected — detach but keep the pty alive for others.
			s.removeSub(sub)
			conn.Close()
			close(connDone)
			return
		}
		if msgType == websocket.TextMessage {
			var resize struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(msg, &resize) == nil && resize.Type == "resize" {
				pty.Setsize(s.ptmx, &pty.Winsize{ //nolint:errcheck
					Rows: uint16(resize.Rows),
					Cols: uint16(resize.Cols),
				})
				continue
			}
		}
		s.pmu.Lock()
		_, werr := io.WriteString(s.ptmx, string(msg))
		s.pmu.Unlock()
		if werr != nil {
			s.removeSub(sub)
			conn.Close()
			close(connDone)
			return
		}
	}
}
