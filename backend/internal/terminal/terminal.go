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

// session holds a persistent pty that survives websocket reconnects.
type session struct {
	id     string
	ptmx   *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	wmu    sync.Mutex      // serializes writes to conn
	conn   *websocket.Conn // current active connection
	buf    *ringBuffer     // scrollback buffer for reconnect replay
	done   chan struct{}
	closed bool
}

// writeMessage safely writes to the current connection with write serialization.
func (s *session) writeMessage(msgType int, data []byte) error {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	s.wmu.Lock()
	err := c.WriteMessage(msgType, data)
	s.wmu.Unlock()
	if err != nil {
		s.mu.Lock()
		if s.conn == c {
			s.conn = nil
		}
		s.mu.Unlock()
	}
	return err
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
			if s.conn != nil {
				s.conn.Close()
			}
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
		if s.conn != nil {
			s.conn.Close()
		}
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
		buf:  newRingBuffer(64 * 1024), // 64KB scrollback
		done: make(chan struct{}),
	}
	h.sessions[id] = s

	// pty reader goroutine — reads from pty and sends to current ws connection
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				s.mu.Lock()
				s.closed = true
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
			s.writeMessage(websocket.BinaryMessage, data)
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

	// Detach old connection if any
	s.mu.Lock()
	oldConn := s.conn
	s.conn = conn
	s.mu.Unlock()

	if oldConn != nil {
		oldConn.Close()
	}

	// Replay scrollback buffer on reconnect
	if reconnected {
		replay := s.buf.Bytes()
		if len(replay) > 0 {
			s.writeMessage(websocket.BinaryMessage, replay)
		}
	}

	// Set up a ping/pong keepalive
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				c := s.conn
				s.mu.Unlock()
				if c != conn {
					return
				}
				s.wmu.Lock()
				err := conn.WriteMessage(websocket.PingMessage, nil)
				s.wmu.Unlock()
				if err != nil {
					return
				}
			case <-s.done:
				return
			}
		}
	}()

	// websocket -> pty
	log.Printf("[terminal] session %q: entering read loop", sessionID)
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[terminal] session %q: read error: %v", sessionID, err)
			// Client disconnected — detach but keep pty alive
			s.mu.Lock()
			if s.conn == conn {
				s.conn = nil
			}
			s.mu.Unlock()
			conn.Close()
			return
		}
		if msgType == websocket.TextMessage {
			var resize struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(msg, &resize) == nil && resize.Type == "resize" {
				pty.Setsize(s.ptmx, &pty.Winsize{
					Rows: uint16(resize.Rows),
					Cols: uint16(resize.Cols),
				})
				continue
			}
		}
		if _, err := io.WriteString(s.ptmx, string(msg)); err != nil {
			return
		}
	}
}
