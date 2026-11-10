// Package lsp provides a WebSocket-to-stdio proxy for Language Server Protocol
// servers. It spawns one language server process per WebSocket connection
// and bridges JSON-RPC messages between the browser WebSocket client and the
// child process's stdin/stdout.
//
// Security model:
//   - All WebSocket upgrades are protected by the auth middleware in main.go before
//     this handler is reached.
//   - The auth token is echoed back via the "auth.<token>" subprotocol, matching
//     the pattern established by the terminal handler.
//   - The language server binary is located via exec.LookPath so an attacker
//     cannot inject a path; only binaries on the system PATH are eligible.
//   - The working directory is always the validated workspace root obtained from
//     the WorkspaceProvider; it is never taken from user input.
//
// Graceful degradation: if the requested language server binary is not installed,
// the WebSocket connection is accepted and a single JSON notification is sent to
// inform the client, then the connection is closed cleanly. No error page or
// HTTP 500 is returned.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WorkspaceProvider is the subset of workspace.Manager used by this package.
type WorkspaceProvider interface {
	Current() string
	OnChange(func(string))
}

// langServer maps a canonical language name to the binary, arguments, and the
// file extensions that belong to it.
type langServer struct {
	bin  string
	args []string
	exts []string
}

// knownServers is the built-in table of supported languages, extensible at
// startup via LoadConfig (see ~/.wede/lsp.json). All binaries are located via
// exec.LookPath — none are hard-coded paths. LoadConfig is called once before
// serving, so reads during request handling need no lock.
var knownServers = map[string]langServer{
	"go":         {bin: "gopls", args: []string{"serve"}, exts: []string{"go"}},
	"javascript": {bin: "typescript-language-server", args: []string{"--stdio"}, exts: []string{"js", "jsx", "cjs", "mjs"}},
	"typescript": {bin: "typescript-language-server", args: []string{"--stdio"}, exts: []string{"ts", "tsx"}},
	"python":     {bin: "pylsp", args: []string{}, exts: []string{"py", "pyw"}},
	"rust":       {bin: "rust-analyzer", args: []string{}, exts: []string{"rs"}},
}

// ConfigServer is one user-defined language server in ~/.wede/lsp.json.
type ConfigServer struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Extensions []string `json:"extensions"`
}

// Config is the on-disk LSP configuration: a map of language name → server.
type Config struct {
	Servers map[string]ConfigServer `json:"servers"`
}

// LoadConfig merges language-server definitions from a JSON file into the
// registry, letting users add support for any LSP server without recompiling.
// A missing file is not an error; config entries override built-ins by language.
// Must be called before the handler starts serving (it mutates the registry).
func LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for lang, s := range cfg.Servers {
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang == "" || strings.TrimSpace(s.Command) == "" {
			continue
		}
		exts := make([]string, 0, len(s.Extensions))
		for _, e := range s.Extensions {
			e = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(e, ".")))
			if e != "" {
				exts = append(exts, e)
			}
		}
		knownServers[lang] = langServer{bin: strings.TrimSpace(s.Command), args: s.Args, exts: exts}
	}
	return nil
}

// LanguageExtensions returns a map of file extension → language name across all
// registered servers, so the client can route a file to the right LSP.
func LanguageExtensions() map[string]string {
	out := make(map[string]string)
	for lang, ls := range knownServers {
		for _, e := range ls.exts {
			out[e] = lang
		}
	}
	return out
}

// serverProc belongs exclusively to one WebSocket. done closes only after the
// child has been reaped and all proxy goroutines have returned.
type serverProc struct {
	cmd        *exec.Cmd
	conn       *websocket.Conn
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	stdoutPipe io.Closer
	stopOnce   sync.Once
	done       chan struct{}
	stopped    chan struct{}
	release    func()
}

// stop interrupts both network and pipe I/O, including blocked writes. It must
// not wait: it is also called by the goroutines that teardown needs to join.
func (p *serverProc) stop() {
	p.stopOnce.Do(func() {
		close(p.stopped)
		p.conn.Close()
		killProcessTree(p.cmd)
		p.stdin.Close()
		p.stdoutPipe.Close()
	})
}

// Handler manages LSP server processes and WebSocket proxy connections.
type Handler struct {
	ws         WorkspaceProvider
	mu         sync.Mutex
	servers    map[*serverProc]struct{}
	closed     bool
	generation uint64
	changed    chan struct{}
	upgrader   websocket.Upgrader
}

// New creates an LSP handler. allowedOrigins mirrors the terminal handler's
// origin-check behaviour (space-separated list from frame_ancestors config).
func New(ws WorkspaceProvider, allowedOrigins string) *Handler {
	allowed := parseOrigins(allowedOrigins)
	h := &Handler{
		ws:      ws,
		servers: make(map[*serverProc]struct{}),
		changed: make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return checkOrigin(r, allowed)
			},
		},
	}
	// When the workspace changes, kill all language server processes; the new
	// workspace needs fresh servers initialised with the correct root URI.
	ws.OnChange(func(string) { h.stopServers(false) })
	return h
}

// AvailableServers returns a map of language → binary path for every known
// language server that is currently found on PATH.
func AvailableServers() map[string]string {
	out := make(map[string]string)
	for lang, ls := range knownServers {
		if path, err := exec.LookPath(ls.bin); err == nil {
			out[lang] = path
		}
	}
	return out
}

// Close permanently closes the handler and waits for its children and proxy
// goroutines to finish. It is safe to call concurrently or more than once.
func (h *Handler) Close() { h.stopServers(true) }

func (h *Handler) stopServers(permanent bool) {
	h.mu.Lock()
	h.closed = h.closed || permanent
	h.generation++
	close(h.changed)
	h.changed = make(chan struct{})
	procs := make([]*serverProc, 0, len(h.servers))
	for p := range h.servers {
		procs = append(procs, p)
	}
	h.mu.Unlock()
	for _, p := range procs {
		p.stop()
	}
	for _, p := range procs {
		<-p.done
	}
}

// HandleAvailable returns a JSON object listing installed language servers.
// GET /api/lsp/available
func (h *Handler) HandleAvailable(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"available":  AvailableServers(),
		"extensions": LanguageExtensions(),
	})
}

// HandleWS is the WebSocket endpoint: GET /api/lsp?lang=<language>
//
// The client sends LSP JSON-RPC messages as WebSocket text frames; the handler
// proxies them to the language server's stdin (with Content-Length headers) and
// forwards stdout messages back as text frames.
//
// If the language server binary is not installed, one JSON notification is sent
// and the connection is closed (no error HTTP status — the upgrade already
// succeeded with 101).
func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
	if lang == "" {
		http.Error(w, "missing lang parameter", http.StatusBadRequest)
		return
	}

	// Capture the generation before upgrading so an in-flight handshake cannot
	// start a stale session after a workspace change or Close.
	h.mu.Lock()
	generation, closed := h.generation, h.closed
	h.mu.Unlock()
	if closed {
		http.Error(w, "LSP handler closed", http.StatusServiceUnavailable)
		return
	}

	// Echo auth subprotocol (same pattern as terminal handler).
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
		log.Printf("[lsp] upgrade error: %v", err)
		return
	}
	defer conn.Close()
	configureWebSocket(conn, pongWait)

	// Validate language is in the known set.
	ls, ok := knownServers[lang]
	if !ok {
		sendNotification(conn, "window/showMessage", map[string]any{
			"type":    3, // Info
			"message": fmt.Sprintf("wede: language %q is not supported for LSP", lang),
		})
		return
	}

	// Check binary availability.
	binPath, err := exec.LookPath(ls.bin)
	if err != nil {
		log.Printf("[lsp] language server %q (%s) not found on PATH", lang, ls.bin)
		sendNotification(conn, "window/showMessage", map[string]any{
			"type":    3, // Info
			"message": fmt.Sprintf("wede: language server for %q (%s) is not installed — LSP features inactive", lang, ls.bin),
		})
		return
	}

	proc, err := h.startServer(conn, generation, binPath, ls.args)
	if err != nil {
		log.Printf("[lsp] failed to start %s: %v", ls.bin, err)
		sendNotification(conn, "window/showMessage", map[string]any{
			"type": 1, "message": fmt.Sprintf("wede: failed to start language server: %v", err),
		})
		return
	}

	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		defer proc.stop()
		heartbeat(conn, proc.stopped, pingInterval)
	}()
	exited := make(chan struct{})
	go func() {
		proc.cmd.Wait() // Exactly one waiter, independent of stdout EOF.
		proc.stop()
		close(exited)
	}()
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		defer proc.stop()
		if err := proxyServerToWS(proc, conn); err != nil {
			log.Printf("[lsp] server→ws error: %v", err)
		}
	}()
	// A byte budget tolerates bursts of small messages. A saturated queue
	// waits briefly for backpressure to clear, but Close/server exit can cancel
	// it immediately. Disconnect detection behind backpressure is delayed by at
	// most queueGrace; the best-effort overload close frame has controlWait.
	messages := newMessageQueue()
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		defer proc.stop()
		for {
			msg, ok := messages.pop(proc.stopped)
			if !ok {
				return
			}
			if err := writeLSPMessage(proc.stdin, msg); err != nil {
				return
			}
			messages.written(msg)
		}
	}()
	defer func() {
		proc.stop()
		<-inputDone
		<-outputDone
		<-exited
		<-heartbeatDone
		proc.release()
		h.mu.Lock()
		delete(h.servers, proc)
		close(proc.done)
		h.mu.Unlock()
	}()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := messages.push(msg, proc.stopped, queueGrace); err != nil {
			log.Printf("[lsp] stdin queue: %v", err)
			// WriteControl is safe alongside the sole stdout/data writer.
			conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseTryAgainLater, err.Error()),
				time.Now().Add(controlWait))
			return
		}
	}
}

// startServer serializes admission with workspace changes and Close. No process
// can be started without being registered before either takes its snapshot.
func (h *Handler) startServer(conn *websocket.Conn, generation uint64, binPath string, args []string) (*serverProc, error) {
	h.mu.Lock()
	if h.closed || generation != h.generation {
		h.mu.Unlock()
		return nil, fmt.Errorf("LSP session closed or workspace changed")
	}
	cancel := h.changed
	h.mu.Unlock()
	release, err := globalSessions.acquire(cancel, maxSessions(), admissionWait)
	if err != nil {
		return nil, err
	}
	admitted := false
	defer func() {
		if !admitted {
			release()
		}
	}()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || generation != h.generation {
		return nil, fmt.Errorf("LSP session closed or workspace changed")
	}
	workspaceDir := h.ws.Current()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = workspaceDir
	configureProcess(cmd)
	childStdin, stdin, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	cmd.Stdin = childStdin
	// Own the stdout pipe ourselves: Cmd.Wait must not close a StdoutPipe
	// concurrently with its reader. Explicit closure also unblocks reads if a
	// descendant inherits stdout after the server exits.
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		childStdin.Close()
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stdout = childStdout
	if err := cmd.Start(); err != nil {
		childStdin.Close()
		stdin.Close()
		stdout.Close()
		childStdout.Close()
		return nil, fmt.Errorf("start: %w", err)
	}
	childStdin.Close()
	childStdout.Close()
	p := &serverProc{cmd: cmd, conn: conn, stdin: stdin,
		stdout: bufio.NewReader(stdout), stdoutPipe: stdout, done: make(chan struct{}),
		stopped: make(chan struct{}), release: release}
	h.servers[p] = struct{}{}
	admitted = true
	log.Printf("[lsp] started: %s %v (workspace=%s)", binPath, args, workspaceDir)
	return p, nil
}

func writeLSPMessage(w io.Writer, msg []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(msg)); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}

// proxyServerToWS is the sole stdout reader and WebSocket data writer.
func proxyServerToWS(proc *serverProc, conn *websocket.Conn) error {
	for {
		msg, err := readLSPMessage(proc.stdout)
		if err != nil {
			return err
		}
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return err
		}
	}
}

// readLSPMessage reads one Content-Length framed JSON-RPC message from r.
func readLSPMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	// Count the entire header section, including ignored headers and line
	// endings. ReadByte avoids ReadString allocating an unbounded header line.
	total := 0
	for {
		line := make([]byte, 0, 128)
		for {
			if total >= maxHeaderBytes {
				return nil, fmt.Errorf("LSP headers exceed %d bytes", maxHeaderBytes)
			}
			ch, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			total++
			if ch == '\n' {
				break
			}
			line = append(line, ch)
		}
		header := strings.TrimRight(string(line), "\r")
		if header == "" {
			break
		}
		name, value, ok := strings.Cut(header, ":")
		if ok && strings.EqualFold(name, "Content-Length") {
			if contentLength != -1 {
				return nil, fmt.Errorf("duplicate Content-Length")
			}
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 || n > maxMessageBytes {
				return nil, fmt.Errorf("invalid Content-Length: must be 0..%d", maxMessageBytes)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("no Content-Length header found")
	}

	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// sendNotification sends a single JSON-RPC notification to the WebSocket client
// and is used for best-effort out-of-band messages (e.g. "server not installed").
func sendNotification(conn *websocket.Conn, method string, params any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, _ := json.Marshal(msg)
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	conn.WriteMessage(websocket.TextMessage, data) //nolint:errcheck
}

// ── Origin helpers (mirrors terminal package) ────────────────────────────────

func parseOrigins(frameAncestors string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, o := range strings.Fields(frameAncestors) {
		set[o] = struct{}{}
	}
	return set
}

func checkOrigin(r *http.Request, allowedOrigins map[string]struct{}) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	selfOrigin := scheme + "://" + r.Host
	if origin == selfOrigin {
		return true
	}
	if _, ok := allowedOrigins[origin]; ok {
		return true
	}
	log.Printf("[lsp] rejected WebSocket upgrade from origin %q (host=%s)", origin, r.Host)
	return false
}
