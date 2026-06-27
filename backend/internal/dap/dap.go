// Package dap proxies the Debug Adapter Protocol over a WebSocket, the same way
// internal/lsp proxies LSP: it spawns a debug adapter process per session and
// bridges Content-Length framed JSON messages between the browser and the
// adapter's stdin/stdout.
//
// Unlike LSP there is no process caching — each debug session is a fresh adapter
// that is killed when the socket closes.
//
// Security: the adapter binary is located via exec.LookPath (no injected paths);
// the working directory is always the validated workspace root. Built-in
// adapters cover the common languages; more can be added via ~/.wede/debug.json
// (global) or a trusted workspace's .wede/debug.json — a debug adapter runs host
// code, so project config is honoured only when the owner trusts the workspace.
package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"wede/backend/internal/trust"
)

// WorkspaceProvider is the subset of the workspace used here.
type WorkspaceProvider interface {
	Current() string
}

// adapter is a debug adapter command + the file extensions it serves.
type adapter struct {
	bin  string
	args []string
	exts []string
}

// builtins are the out-of-the-box debug adapters (located via PATH).
var builtins = map[string]adapter{
	"go":     {bin: "dlv", args: []string{"dap"}, exts: []string{"go"}},
	"python": {bin: "debugpy-adapter", args: []string{}, exts: []string{"py", "pyw"}},
}

// configAdapter mirrors one entry in debug.json.
type configAdapter struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Extensions []string `json:"extensions"`
}

type debugConfig struct {
	Adapters map[string]configAdapter `json:"adapters"`
}

// resolveAdapters builds the active adapter table: built-ins, then the owner's
// global ~/.wede/debug.json, then the workspace's committed .wede/debug.json
// (only if the workspace is trusted, since adapters run host code).
func resolveAdapters(root string) map[string]adapter {
	out := make(map[string]adapter, len(builtins))
	for k, v := range builtins {
		out[k] = v
	}
	if home, err := os.UserHomeDir(); err == nil {
		mergeAdapterFile(out, filepath.Join(home, ".wede", "debug.json"))
	}
	if root != "" && trust.IsTrusted(root) {
		mergeAdapterFile(out, filepath.Join(root, ".wede", "debug.json"))
	}
	return out
}

func mergeAdapterFile(out map[string]adapter, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg debugConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	for lang, a := range cfg.Adapters {
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang == "" || strings.TrimSpace(a.Command) == "" {
			continue
		}
		exts := make([]string, 0, len(a.Extensions))
		for _, e := range a.Extensions {
			e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
			if e != "" {
				exts = append(exts, e)
			}
		}
		out[lang] = adapter{bin: strings.TrimSpace(a.Command), args: a.Args, exts: exts}
	}
}

// Handler upgrades DAP WebSockets and bridges them to a debug adapter process.
type Handler struct {
	ws       WorkspaceProvider
	upgrader websocket.Upgrader
}

// New builds a DAP handler. allowedOrigins mirrors the lsp/terminal origin check.
func New(ws WorkspaceProvider, allowedOrigins string) *Handler {
	allowed := parseOrigins(allowedOrigins)
	return &Handler{
		ws: ws,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return checkOrigin(r, allowed) },
		},
	}
}

// HandleAvailable reports which debug adapters are installed plus the ext→language
// map, so the client can offer "Debug" only for supported, installed languages.
// GET /api/workspaces/{id}/dap/available
func (h *Handler) HandleAvailable(w http.ResponseWriter, _ *http.Request) {
	root := h.ws.Current()
	adapters := resolveAdapters(root)
	available := map[string]string{}
	exts := map[string]string{}
	for lang, a := range adapters {
		if path, err := exec.LookPath(a.bin); err == nil {
			available[lang] = path
		}
		for _, e := range a.exts {
			exts[e] = lang
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"available":  available,
		"extensions": exts,
	})
}

// HandleWS is the debug session socket: GET /api/workspaces/{id}/dap?lang=<lang>
func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
	if lang == "" {
		http.Error(w, "missing lang parameter", http.StatusBadRequest)
		return
	}

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
		log.Printf("[dap] upgrade error: %v", err)
		return
	}
	defer conn.Close()

	root := h.ws.Current()
	a, ok := resolveAdapters(root)[lang]
	if !ok {
		sendOutput(conn, fmt.Sprintf("wede: no debug adapter configured for %q", lang))
		return
	}
	binPath, err := exec.LookPath(a.bin)
	if err != nil {
		sendOutput(conn, fmt.Sprintf("wede: debug adapter for %q (%s) is not installed", lang, a.bin))
		return
	}

	cmd := exec.Command(binPath, a.args...)
	if root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		sendOutput(conn, "wede: failed to open adapter stdin")
		return
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		sendOutput(conn, "wede: failed to open adapter stdout")
		return
	}
	if err := cmd.Start(); err != nil {
		sendOutput(conn, fmt.Sprintf("wede: failed to start debug adapter: %v", err))
		return
	}
	log.Printf("[dap] session started: lang=%s adapter=%s root=%s", lang, a.bin, root)
	stdout := bufio.NewReader(stdoutPipe)

	// adapter stdout → WebSocket
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := readMessage(stdout)
			if err != nil {
				return
			}
			if conn.WriteMessage(websocket.TextMessage, msg) != nil {
				return
			}
		}
	}()

	// WebSocket → adapter stdin
	conn.SetReadLimit(1 << 20)
	for {
		select {
		case <-done:
			goto cleanup
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if _, err := stdin.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg)))); err != nil {
			break
		}
		if _, err := stdin.Write(msg); err != nil {
			break
		}
	}

cleanup:
	stdin.Close()
	if cmd.Process != nil {
		cmd.Process.Kill() //nolint:errcheck
	}
	cmd.Wait() //nolint:errcheck
	log.Printf("[dap] session ended: lang=%s", lang)
}

// readMessage reads one Content-Length framed JSON message from r.
func readMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("no Content-Length header")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// sendOutput sends a DAP "output" event so the client can surface a message
// before the socket closes (e.g. adapter not installed).
func sendOutput(conn *websocket.Conn, message string) {
	evt := map[string]any{
		"type":  "event",
		"event": "output",
		"body":  map[string]any{"category": "console", "output": message + "\n"},
	}
	data, _ := json.Marshal(evt)
	conn.WriteMessage(websocket.TextMessage, data) //nolint:errcheck
}

// ── origin helpers (mirror lsp/terminal) ─────────────────────────────────────

func parseOrigins(frameAncestors string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, o := range strings.Fields(frameAncestors) {
		set[o] = struct{}{}
	}
	return set
}

func checkOrigin(r *http.Request, allowed map[string]struct{}) bool {
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
	if origin == scheme+"://"+r.Host {
		return true
	}
	_, ok := allowed[origin]
	return ok
}
