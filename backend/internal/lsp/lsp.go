// Package lsp provides a WebSocket-to-stdio proxy for Language Server Protocol
// servers. It spawns one language server process per (workspace, language) pair
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

// serverKey identifies a unique language server instance.
type serverKey struct {
	workspace string
	lang      string
}

// serverProc is a running language server process with its I/O pipes.
type serverProc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex // guards concurrent writes to stdin
	done   chan struct{}
}

// kill tears down the process and signals the done channel.
func (p *serverProc) kill() {
	if p.cmd.Process != nil {
		p.cmd.Process.Kill() //nolint:errcheck
	}
}

// Handler manages LSP server processes and WebSocket proxy connections.
type Handler struct {
	ws       WorkspaceProvider
	mu       sync.Mutex
	servers  map[serverKey]*serverProc
	upgrader websocket.Upgrader
}

// New creates an LSP handler. allowedOrigins mirrors the terminal handler's
// origin-check behaviour (space-separated list from frame_ancestors config).
func New(ws WorkspaceProvider, allowedOrigins string) *Handler {
	allowed := parseOrigins(allowedOrigins)
	h := &Handler{
		ws:      ws,
		servers: make(map[serverKey]*serverProc),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return checkOrigin(r, allowed)
			},
		},
	}
	// When the workspace changes, kill all language server processes; the new
	// workspace needs fresh servers initialised with the correct root URI.
	ws.OnChange(func(string) {
		h.mu.Lock()
		defer h.mu.Unlock()
		for k, p := range h.servers {
			p.kill()
			delete(h.servers, k)
		}
	})
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

// Close kills every language server process owned by this handler. Called when
// the owning room is closed.
func (h *Handler) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, p := range h.servers {
		p.kill()
		delete(h.servers, k)
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

	workspaceDir := h.ws.Current()
	key := serverKey{workspace: workspaceDir, lang: lang}

	proc, err := h.getOrStartServer(key, binPath, ls.args, workspaceDir)
	if err != nil {
		log.Printf("[lsp] failed to start %s: %v", ls.bin, err)
		sendNotification(conn, "window/showMessage", map[string]any{
			"type":    1, // Error
			"message": fmt.Sprintf("wede: failed to start language server: %v", err),
		})
		return
	}

	log.Printf("[lsp] ws connected: lang=%s workspace=%s", lang, workspaceDir)

	// Forward server stdout → WebSocket in a goroutine.
	connClosed := make(chan struct{})
	go func() {
		defer close(connClosed)
		if err := proxyServerToWS(proc, conn); err != nil {
			log.Printf("[lsp] server→ws error: %v", err)
		}
	}()

	// Forward WebSocket messages → server stdin (this goroutine).
	proxyWSToServer(conn, proc, connClosed)
	log.Printf("[lsp] ws disconnected: lang=%s", lang)
}

// getOrStartServer returns an existing running server or starts a new one.
func (h *Handler) getOrStartServer(key serverKey, binPath string, args []string, workspaceDir string) (*serverProc, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if p, ok := h.servers[key]; ok {
		// Verify the process is still alive by checking if done is closed.
		select {
		case <-p.done:
			// Process exited; fall through to restart.
			delete(h.servers, key)
		default:
			return p, nil
		}
	}

	cmdArgs := append([]string{}, args...)
	cmd := exec.Command(binPath, cmdArgs...)
	if workspaceDir != "" {
		cmd.Dir = workspaceDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	p := &serverProc{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
		done:   make(chan struct{}),
	}
	h.servers[key] = p

	// Reap the process when it exits.
	go func() {
		cmd.Wait() //nolint:errcheck
		close(p.done)
		h.mu.Lock()
		if h.servers[key] == p {
			delete(h.servers, key)
		}
		h.mu.Unlock()
		log.Printf("[lsp] server exited: lang=%s", key.lang)
	}()

	log.Printf("[lsp] started: %s %v (workspace=%s)", binPath, args, workspaceDir)
	return p, nil
}

// proxyWSToServer reads JSON-RPC messages from the WebSocket and writes them
// to the language server's stdin using the LSP wire format:
//
//	Content-Length: <n>\r\n\r\n<json>
func proxyWSToServer(conn *websocket.Conn, proc *serverProc, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// Write Content-Length framed message to language server stdin.
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
		proc.mu.Lock()
		_, err = proc.stdin.Write([]byte(header))
		if err == nil {
			_, err = proc.stdin.Write(msg)
		}
		proc.mu.Unlock()
		if err != nil {
			log.Printf("[lsp] stdin write error: %v", err)
			return
		}
	}
}

// proxyServerToWS reads LSP Content-Length framed messages from the language
// server's stdout and forwards them as WebSocket text frames.
func proxyServerToWS(proc *serverProc, conn *websocket.Conn) error {
	for {
		select {
		case <-proc.done:
			return nil
		default:
		}

		msg, err := readLSPMessage(proc.stdout)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return err
		}
	}
}

// readLSPMessage reads one Content-Length framed JSON-RPC message from r.
func readLSPMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	// Read headers until blank line.
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
		// Other headers (e.g. Content-Type) are ignored.
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
