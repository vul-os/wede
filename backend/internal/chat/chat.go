// Package chat provides per-workspace live chat with markdown persistence and git
// activity notifications. The Hub is transport-agnostic (same pattern as presence):
// each connected peer drains an outbound buffered chan of JSON bytes; the WS handler
// pumps that chan to the socket and feeds inbound text messages back via Post.
//
// Route the integrator must wire:
//
//	GET /api/workspaces/{id}/chat -> workspace.Chat().HandleWS
//	(behind auth middleware, public-read OK)
package chat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval    = 30 * time.Second
	readLimit       = 4 << 10 // 4 KiB — chat messages are tiny
	gitPollInterval = 5 * time.Second
	peerBuf         = 64 // outbound channel buffer per peer
)

// Message is one chat event in the workspace room.
type Message struct {
	ID    string    `json:"id"`
	User  string    `json:"user,omitempty"`
	Color string    `json:"color,omitempty"`
	Text  string    `json:"text"`
	Kind  string    `json:"kind"` // "user" | "system" | "git"
	Time  time.Time `json:"time"`
}

// outEnvelope is the wire format sent to connected peers.
type outEnvelope struct {
	Type     string    `json:"type"`               // "history" | "msg"
	Messages []Message `json:"messages,omitempty"` // only for type=history
	Message  *Message  `json:"message,omitempty"`  // only for type=msg
}

// inEnvelope is the wire format received from connected peers.
type inEnvelope struct {
	Type string `json:"type"` // expected: "msg"
	Text string `json:"text"`
}

type peer struct {
	out chan []byte
}

// Channel names. Public chat is committed (.wede/chat.md) so collaborators and
// LLMs working on the repo can read it; private chat lives in .wede/private/,
// which wede gitignores by default so it never enters the repo.
const (
	ChannelPublic  = "public"
	ChannelPrivate = "private"
)

// Hub is one chat channel for a workspace. Safe for concurrent use.
type Hub struct {
	mu      sync.Mutex
	peers   map[string]*peer
	history []Message
	root    string
	relPath string // workspace-relative chat file path
	gitOn   bool   // post git-activity messages (public channel only)
	counter uint64
	closed  bool
	stop    chan struct{}
}

// NewHub creates a Hub for the given channel ("public" or "private") rooted at
// root, replaying any existing history file so reconnecting clients see prior
// messages. The public channel persists to .wede/chat.md and starts a
// git-activity poller; the private channel persists to .wede/private/chat.md and
// ensures .wede/.gitignore excludes it.
func NewHub(root, channel string) *Hub {
	relPath := filepath.Join(".wede", "chat.md")
	gitOn := true
	if channel == ChannelPrivate {
		relPath = filepath.Join(".wede", "private", "chat.md")
		gitOn = false
		ensurePrivateGitignore(root)
	}
	h := &Hub{
		peers:   make(map[string]*peer),
		root:    root,
		relPath: relPath,
		gitOn:   gitOn,
		stop:    make(chan struct{}),
	}
	h.loadHistory()
	if gitOn {
		go h.gitPoller()
	}
	return h
}

// ensurePrivateGitignore writes "private/" into <root>/.wede/.gitignore so the
// private chat folder is never committed. Best-effort + idempotent.
func ensurePrivateGitignore(root string) {
	dir := filepath.Join(root, ".wede")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	gi := filepath.Join(dir, ".gitignore")
	if data, err := os.ReadFile(gi); err == nil && strings.Contains(string(data), "private/") {
		return
	}
	f, err := os.OpenFile(gi, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, "private/") //nolint:errcheck
}

// chatFile returns the absolute path to this channel's history file.
func (h *Hub) chatFile() string {
	return filepath.Join(h.root, h.relPath)
}

// loadHistory reads .wede/chat.md (if present) and appends parsed messages to
// h.history. Malformed lines are silently skipped.
func (h *Hub) loadHistory() {
	f, err := os.Open(h.chatFile())
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if msg, ok := parseLine(sc.Text()); ok {
			h.history = append(h.history, msg)
		}
	}
}

// parseLine parses a single `- <ts> [kind] …` markdown line into a Message.
// Returns ok=false for any malformed input so history loading is best-effort.
func parseLine(line string) (Message, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "- ") {
		return Message{}, false
	}
	line = line[2:] // strip leading "- "

	// Expected tokens: <ISO8601> [kind] <rest>
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return Message{}, false
	}
	t, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return Message{}, false
	}
	kbracket := parts[1]
	if !strings.HasPrefix(kbracket, "[") || !strings.HasSuffix(kbracket, "]") {
		return Message{}, false
	}
	kind := kbracket[1 : len(kbracket)-1]
	rest := parts[2]

	msg := Message{
		ID:   fmt.Sprintf("h%d", t.UnixNano()),
		Kind: kind,
		Time: t,
	}
	switch kind {
	case "user":
		// format: "<username>: <text>"
		sub := strings.SplitN(rest, ": ", 2)
		if len(sub) == 2 {
			msg.User = sub[0]
			msg.Text = sub[1]
		} else {
			msg.Text = rest
		}
	default: // "git", "system"
		msg.Text = rest
	}
	return msg, true
}

// formatLine serialises a Message into a human/LLM-friendly markdown line.
// Examples:
//
//	- 2026-06-26T15:30:00Z [user] alice: hello world
//	- 2026-06-26T15:31:00Z [git] 📦 committed a1b2c3d: fix typo
//	- 2026-06-26T15:32:00Z [system] workspace opened
func formatLine(m Message) string {
	ts := m.Time.UTC().Format(time.RFC3339)
	switch m.Kind {
	case "user":
		return fmt.Sprintf("- %s [user] %s: %s\n", ts, m.User, m.Text)
	default:
		return fmt.Sprintf("- %s [%s] %s\n", ts, m.Kind, m.Text)
	}
}

// appendToDisk creates .wede/chat.md if needed and appends one formatted line.
// Errors are silently dropped; live chat is not gated on disk availability.
func (h *Hub) appendToDisk(m Message) {
	if err := os.MkdirAll(filepath.Dir(h.chatFile()), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(h.chatFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprint(f, formatLine(m)) //nolint:errcheck
}

// nextID returns a unique sequential message ID (mu must be held).
func (h *Hub) nextID() string {
	h.counter++
	return fmt.Sprintf("c%d", h.counter)
}

// post is the shared implementation behind Post/PostSystem/PostGit.
// It records the message in history, persists it, and fans it out to all peers.
func (h *Hub) post(user, color, text, kind string) {
	msg := Message{
		User:  user,
		Color: color,
		Text:  text,
		Kind:  kind,
		Time:  time.Now().UTC(),
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	msg.ID = h.nextID()
	h.history = append(h.history, msg)
	data, _ := json.Marshal(outEnvelope{Type: "msg", Message: &msg})
	peers := make([]*peer, 0, len(h.peers))
	for _, p := range h.peers {
		peers = append(peers, p)
	}
	h.mu.Unlock()

	// Disk and channel operations happen outside the lock.
	h.appendToDisk(msg)
	for _, p := range peers {
		select {
		case p.out <- data:
		default: // drop for a slow consumer rather than block the hub
		}
	}
}

// Post posts a user-authored message attributed to user with the given hex color.
func (h *Hub) Post(user, color, text string) {
	h.post(user, color, text, "user")
}

// PostSystem posts a system notification (kind="system").
func (h *Hub) PostSystem(text string) {
	h.post("", "", text, "system")
}

// PostGit posts a git-activity notification (kind="git").
func (h *Hub) PostGit(text string) {
	h.post("", "", text, "git")
}

// Join registers a new peer and returns (id, outbound chan). The chan receives a
// history dump (type="history") followed by live messages (type="msg"). The caller
// pumps the chan to the WebSocket. The channel is buffered; a slow consumer drops
// live events rather than blocking the hub.
func (h *Hub) Join(username, color string) (string, <-chan []byte) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		ch := make(chan []byte)
		close(ch)
		return "", ch
	}
	h.counter++
	id := fmt.Sprintf("m%d", h.counter)
	p := &peer{out: make(chan []byte, peerBuf)}
	h.peers[id] = p
	hist := make([]Message, len(h.history))
	copy(hist, h.history)
	h.mu.Unlock()

	// Send the full history as a single frame (best-effort; buffer is peerBuf).
	if len(hist) > 0 {
		if data, err := json.Marshal(outEnvelope{Type: "history", Messages: hist}); err == nil {
			select {
			case p.out <- data:
			default:
			}
		}
	}
	return id, p.out
}

// Leave removes a peer by id and closes its outbound channel.
func (h *Hub) Leave(id string) {
	h.mu.Lock()
	p, ok := h.peers[id]
	if ok {
		delete(h.peers, id)
		close(p.out)
	}
	h.mu.Unlock()
}

// Close shuts the hub down: stops git polling and closes all peer channels. Idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	for id, p := range h.peers {
		close(p.out)
		delete(h.peers, id)
	}
	h.mu.Unlock()
	close(h.stop)
}

// ── git activity poller ────────────────────────────────────────────────────────

// gitPoller runs in its own goroutine, polling git state every gitPollInterval.
// It posts events only when something actually changes (dedup via shouldPostCommit /
// shouldPostDirty) so it never spams the channel.
func (h *Hub) gitPoller() {
	ticker := time.NewTicker(gitPollInterval)
	defer ticker.Stop()
	var lastHEAD string
	var lastCount int = -1 // -1 = first sample, skip posting

	for {
		select {
		case <-h.stop:
			return
		case <-ticker.C:
			h.pollGit(&lastHEAD, &lastCount)
		}
	}
}

// pollGit checks the current git HEAD and dirty-file count, posting events when
// they change. lastHEAD and lastCount carry state between calls.
func (h *Hub) pollGit(lastHEAD *string, lastCount *int) {
	headOut, err := gitCmd(h.root, "rev-parse", "HEAD")
	if err != nil {
		return // not a git repo or no commits yet
	}
	head := strings.TrimSpace(headOut)

	statusOut, _ := gitCmd(h.root, "status", "--porcelain")
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(statusOut), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	if shouldPostCommit(*lastHEAD, head) {
		short, _ := gitCmd(h.root, "rev-parse", "--short", "HEAD")
		short = strings.TrimSpace(short)
		subject, _ := gitCmd(h.root, "log", "-1", "--pretty=%s")
		subject = strings.TrimSpace(subject)
		h.PostGit(fmt.Sprintf("📦 committed %s: %s", short, subject))
	}
	*lastHEAD = head

	if shouldPostDirty(*lastCount, count) {
		if count == 0 {
			h.PostGit("✅ working tree clean")
		} else {
			h.PostGit(fmt.Sprintf("✏️ %d uncommitted change(s)", count))
		}
	}
	*lastCount = count
}

// shouldPostCommit returns true when the HEAD has changed and we had a known previous
// HEAD (i.e. this is not the first poll).
func shouldPostCommit(prev, next string) bool {
	return prev != "" && prev != next
}

// shouldPostDirty returns true when the uncommitted-file count has changed and we
// had a prior sample (prev >= 0).
func shouldPostDirty(prev, next int) bool {
	return prev >= 0 && prev != next
}

// gitCmd runs a git sub-command under root and returns combined stdout.
func gitCmd(root string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", root}, args...)
	out, err := exec.Command("git", fullArgs...).Output()
	return string(out), err
}

// ── WebSocket handler ──────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	// Allow all origins; the auth middleware gates access; public-read is OK per spec.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWS upgrades a GET /api/workspaces/{id}/chat request to a chat WebSocket.
//
// Route the integrator must wire:
//
//	GET /api/workspaces/{id}/chat -> workspace.Chat().HandleWS
//	(behind auth middleware, public-read OK)
//
// Query params:
//
//	token=    session token (also accepted as "auth.<token>" WS subprotocol for
//	          browser clients that set subprotocols)
//	username= display name for this user's messages
//	color=    hex colour for this user's avatar/text
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	// Mirror collab.go: echo back "auth.<token>" subprotocol so browser handshakes succeed.
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

	conn, err := upgrader.Upgrade(w, r, hdr)
	if err != nil {
		log.Println("[chat] websocket upgrade error:", err)
		return
	}

	username := r.URL.Query().Get("username")
	color := r.URL.Query().Get("color")
	if username == "" {
		username = "anon"
	}
	if color == "" {
		color = "#888888"
	}

	id, out := h.Join(username, color)
	if id == "" { // hub is already closed (workspace shutting down)
		conn.Close()
		return
	}

	stop := make(chan struct{})
	go chatWritePump(conn, out, stop)

	conn.SetReadLimit(readLimit)
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var in inEnvelope
		if err := json.Unmarshal(raw, &in); err != nil {
			continue
		}
		if in.Type == "msg" && strings.TrimSpace(in.Text) != "" {
			h.Post(username, color, strings.TrimSpace(in.Text))
		}
	}

	close(stop)
	h.Leave(id)
	conn.Close()
}

// chatWritePump drains the peer's outbound channel to the socket and keeps it alive
// with periodic pings.
func chatWritePump(conn *websocket.Conn, out <-chan []byte, stop <-chan struct{}) {
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
