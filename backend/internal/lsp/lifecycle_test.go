package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Run the test executable as a real stdio LSP, with process-local protocol state.
func TestLSPHelperProcess(t *testing.T) {
	if os.Getenv("WEDE_LSP_HELPER") != "1" {
		return
	}
	initialized, shutdown := false, false
	reader := bufio.NewReader(os.Stdin)
	for {
		msg, err := readLSPMessage(reader)
		if err != nil {
			os.Exit(0)
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params string          `json:"params"`
		}
		if json.Unmarshal(msg, &req) != nil {
			os.Exit(2)
		}
		switch req.Method {
		case "spawn":
			child := exec.Command(os.Args[0], "-test.run=^TestLSPDescendant$")
			child.Env = append(os.Environ(), "WEDE_LSP_DESCENDANT="+req.Params)
			child.Stdout = os.Stdout // Descendant intentionally keeps the LSP pipe open.
			if err := child.Start(); err != nil {
				os.Exit(3)
			}
			go child.Wait()
		case "pause":
			time.Sleep(150 * time.Millisecond)
		case "large-output":
			fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n", maxMessageBytes+1)
			for {
				time.Sleep(time.Hour)
			}
		case "large-header":
			fmt.Fprint(os.Stdout, strings.Repeat("x", maxHeaderBytes+1))
			for {
				time.Sleep(time.Hour)
			}
		case "exit":
			os.Exit(0)
		case "block":
			// Acknowledge before deliberately ceasing to read stdin.
			writeLSPMessage(os.Stdout, []byte(`{"jsonrpc":"2.0","method":"blocked"}`))
			for {
				time.Sleep(time.Hour)
			}
		case "flood":
			payload := []byte(strings.Repeat("x", 1<<20))
			for {
				if writeLSPMessage(os.Stdout, payload) != nil {
					os.Exit(0)
				}
			}
		case "close-stdout":
			os.Stdout.Close()
			for {
				time.Sleep(time.Hour)
			}
		case "bad-output":
			fmt.Fprint(os.Stdout, "Content-Length: invalid\r\n\r\n")
			for {
				time.Sleep(time.Hour)
			}
		}
		result := map[string]any{"pid": os.Getpid(), "echo": req.Params}
		cwd, _ := os.Getwd()
		result["cwd"] = cwd
		valid := true
		switch req.Method {
		case "initialize":
			valid = !initialized
			initialized = true
		case "shutdown":
			valid = initialized && !shutdown
			shutdown = true
		default:
			valid = initialized && !shutdown
		}
		result["valid"] = valid
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		if writeLSPMessage(os.Stdout, data) != nil {
			os.Exit(0)
		}
	}
}

type lifecycleWorkspace struct {
	mu        sync.Mutex
	root      string
	listeners []func(string)
}

func (w *lifecycleWorkspace) Current() string { w.mu.Lock(); defer w.mu.Unlock(); return w.root }
func (w *lifecycleWorkspace) OnChange(f func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.listeners = append(w.listeners, f)
}
func (w *lifecycleWorkspace) change(root string) {
	w.mu.Lock()
	w.root = root
	listeners := append([]func(string){}, w.listeners...)
	w.mu.Unlock()
	for _, f := range listeners {
		f(root)
	}
}

func lifecycleHarness(t *testing.T) (*Handler, *lifecycleWorkspace, string) {
	t.Helper()
	t.Setenv("WEDE_LSP_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const lang = "lifecycle-test"
	knownServers[lang] = langServer{bin: exe, args: []string{"-test.run=^TestLSPHelperProcess$"}}
	t.Cleanup(func() { delete(knownServers, lang) })
	ws := &lifecycleWorkspace{root: t.TempDir()}
	h := New(ws, "")
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	t.Cleanup(server.Close)
	t.Cleanup(h.Close)
	return h, ws, "ws" + strings.TrimPrefix(server.URL, "http") + "?lang=" + lang
}
func dialLSP(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

type helperResult struct {
	PID   int    `json:"pid"`
	Echo  string `json:"echo"`
	CWD   string `json:"cwd"`
	Valid bool   `json:"valid"`
}

func rpc(c *websocket.Conn, id int, method, token string) (helperResult, error) {
	// A near-limit JSON round trip crosses two race-instrumented processes.
	// Allow slow CI CPUs time to encode/decode; production I/O deadlines are
	// tested separately and remain unchanged.
	c.SetWriteDeadline(time.Now().Add(30 * time.Second))
	c.SetReadDeadline(time.Now().Add(30 * time.Second))
	err := c.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": token})
	if err != nil {
		return helperResult{}, err
	}
	var reply struct {
		ID     int          `json:"id"`
		Result helperResult `json:"result"`
	}
	if err := c.ReadJSON(&reply); err != nil {
		return helperResult{}, err
	}
	if reply.ID != id || reply.Result.Echo != token || !reply.Result.Valid {
		return reply.Result, fmt.Errorf("unexpected response: %+v (id=%d token=%q)", reply, id, token)
	}
	return reply.Result, nil
}
func request(t *testing.T, c *websocket.Conn, id int, method, token string) helperResult {
	t.Helper()
	r, err := rpc(c, id, method, token)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func processFor(t *testing.T, h *Handler, pid int) *serverProc {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for p := range h.servers {
		if p.cmd.Process.Pid == pid {
			return p
		}
	}
	t.Fatalf("process %d not registered", pid)
	return nil
}
func awaitCleanup(t *testing.T, h *Handler, p *serverProc) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("session did not finish cleanup")
	}
	// done synchronizes access to ProcessState with the sole cmd.Wait caller.
	if p.cmd.ProcessState == nil {
		t.Fatal("child was not reaped")
	}
	h.mu.Lock()
	_, exists := h.servers[p]
	h.mu.Unlock()
	if exists {
		t.Fatal("completed process remains registered")
	}
}
func socketClosed(t *testing.T, c *websocket.Conn) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err := c.ReadMessage()
	if err == nil {
		t.Fatal("socket still open")
	}
	if e, ok := err.(interface{ Timeout() bool }); ok && e.Timeout() {
		t.Fatalf("socket closure timed out: %v", err)
	}
}

func TestConcurrentClientsIndependent(t *testing.T) {
	t.Setenv("WEDE_LSP_MAX_SESSIONS", "6")
	h, _, url := lifecycleHarness(t)
	const clients = 6
	conns := make([]*websocket.Conn, clients)
	procs := make([]*serverProc, clients)
	type outcome struct {
		index  int
		result helperResult
		err    error
	}
	results := make(chan outcome, clients)
	for i := range conns {
		conns[i] = dialLSP(t, url)
		go func(i int) {
			r, err := rpc(conns[i], 1, "initialize", fmt.Sprint(i))
			results <- outcome{i, r, err}
		}(i)
	}
	pids := map[int]bool{}
	for range conns {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if pids[r.result.PID] {
			t.Fatal("clients share a process")
		}
		pids[r.result.PID] = true
		procs[r.index] = processFor(t, h, r.result.PID)
	}
	for i := range conns {
		go func(i int) {
			for id := 2; id < 30; id++ {
				r, err := rpc(conns[i], id, "hover", fmt.Sprintf("%d/%d", i, id))
				if err != nil {
					results <- outcome{i, r, err}
					return
				}
			}
			results <- outcome{index: i}
		}(i)
	}
	for range conns {
		if r := <-results; r.err != nil {
			t.Fatal(r.err)
		}
	}
	request(t, conns[0], 30, "shutdown", "first")
	conns[0].WriteJSON(map[string]any{"method": "exit"})
	socketClosed(t, conns[0])
	awaitCleanup(t, h, procs[0])
	// Shutdown and disconnect of one client cannot affect any other client.
	conns[1].Close()
	awaitCleanup(t, h, procs[1])
	for i := 2; i < clients; i++ {
		request(t, conns[i], 30, "hover", "still alive")
		request(t, conns[i], 31, "shutdown", "own shutdown")
	}
	h.Close()
	for _, p := range procs {
		awaitCleanup(t, h, p)
	}
}

func TestDisconnectReconnect(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	for i := 0; i < 8; i++ {
		c := dialLSP(t, url)
		r := request(t, c, 1, "initialize", "fresh")
		p := processFor(t, h, r.PID)
		c.Close()
		awaitCleanup(t, h, p)
	}
}

func TestServerExitAndBrokenOutputCloseSocket(t *testing.T) {
	for _, method := range []string{"exit", "bad-output", "close-stdout"} {
		t.Run(method, func(t *testing.T) {
			h, _, url := lifecycleHarness(t)
			c := dialLSP(t, url)
			p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
			if err := c.WriteJSON(map[string]any{"method": method}); err != nil {
				t.Fatal(err)
			}
			socketClosed(t, c)
			awaitCleanup(t, h, p)
		})
	}
}

func TestBlockedStdinDisconnect(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	c := dialLSP(t, url)
	p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
	c.WriteJSON(map[string]any{"method": "block"})
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	// Exceeds pipe capacity, leaving the stdin writer blocked in Write.
	if err := c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 2<<20))); err != nil {
		t.Fatal(err)
	}
	c.Close()
	awaitCleanup(t, h, p)
}

func TestWorkspaceChangeAndClose(t *testing.T) {
	h, ws, url := lifecycleHarness(t)
	c1, c2 := dialLSP(t, url), dialLSP(t, url)
	p1 := processFor(t, h, request(t, c1, 1, "initialize", "a").PID)
	p2 := processFor(t, h, request(t, c2, 1, "initialize", "b").PID)
	root := t.TempDir()
	ws.change(root)
	for _, p := range []*serverProc{p1, p2} {
		awaitCleanup(t, h, p)
	}
	socketClosed(t, c1)
	socketClosed(t, c2)
	c3 := dialLSP(t, url)
	r := request(t, c3, 1, "initialize", "new root")
	// macOS may canonicalize /var to /private/var in Getwd.
	want, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(r.CWD)
	if err != nil || !os.SameFile(want, got) {
		t.Fatalf("wrong child cwd: %q", r.CWD)
	}
	p3 := processFor(t, h, r.PID)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); h.Close() }()
	}
	wg.Wait()
	awaitCleanup(t, h, p3)
	socketClosed(t, c3)
	ws.change(ws.Current())
	c, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if c != nil {
		c.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("closed handler accepted connection: %v, %v", resp, err)
	}
}

func TestStaleSessionAdmission(t *testing.T) {
	h, ws, _ := lifecycleHarness(t)
	h.mu.Lock()
	generation := h.generation
	h.mu.Unlock()
	ws.change(t.TempDir())
	if _, err := h.startServer(nil, generation, "must-not-start", nil); err == nil {
		t.Fatal("accepted stale generation")
	}
	h.Close()
	h.mu.Lock()
	generation = h.generation
	h.mu.Unlock()
	if _, err := h.startServer(nil, generation, "must-not-start", nil); err == nil {
		t.Fatal("accepted session after Close")
	}
}

func TestCloseWithUnreadOutput(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	c := dialLSP(t, url)
	p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
	if err := c.WriteJSON(map[string]any{"method": "flood"}); err != nil {
		t.Fatal(err)
	}
	// Read one frame to prove the flood started, then stop consuming output.
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { h.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked on unread output")
	}
	awaitCleanup(t, h, p)
}

func TestLifecycleDuringUpgrade(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%v", permanent), func(t *testing.T) {
			h, ws, url := lifecycleHarness(t)
			entered, release := make(chan struct{}), make(chan struct{})
			// Stop the handshake after generation capture, before server admission.
			h.upgrader.CheckOrigin = func(*http.Request) bool { close(entered); <-release; return true }
			type dialResult struct {
				conn *websocket.Conn
				err  error
			}
			result := make(chan dialResult, 1)
			go func() { c, _, err := websocket.DefaultDialer.Dial(url, nil); result <- dialResult{c, err} }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("upgrade not reached")
			}
			if permanent {
				h.Close()
			} else {
				ws.change(t.TempDir())
			}
			close(release)
			var dial dialResult
			select {
			case dial = <-result:
			case <-time.After(5 * time.Second):
				t.Fatal("upgrade did not finish")
			}
			if dial.err != nil {
				t.Fatal(dial.err)
			}
			defer dial.conn.Close()
			dial.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			var notification struct {
				Method string `json:"method"`
			}
			if err := dial.conn.ReadJSON(&notification); err != nil {
				t.Fatal(err)
			}
			if notification.Method != "window/showMessage" {
				t.Fatalf("unexpected message: %+v", notification)
			}
			socketClosed(t, dial.conn)
			h.mu.Lock()
			count := len(h.servers)
			h.mu.Unlock()
			if count != 0 {
				t.Fatalf("stale handshake started %d processes", count)
			}
		})
	}
}

func TestStartFailureLeavesNoSession(t *testing.T) {
	h, ws, url := lifecycleHarness(t)
	ws.change(ws.Current() + "/missing-directory")
	c := dialLSP(t, url)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var notification struct {
		Method string `json:"method"`
	}
	if err := c.ReadJSON(&notification); err != nil {
		t.Fatal(err)
	}
	if notification.Method != "window/showMessage" {
		t.Fatalf("unexpected message: %+v", notification)
	}
	socketClosed(t, c)
	h.mu.Lock()
	count := len(h.servers)
	h.mu.Unlock()
	if count != 0 {
		t.Fatalf("failed start left %d sessions", count)
	}
}

func TestStalledServerQueueOverflow(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	c := dialLSP(t, url)
	p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
	if err := c.WriteJSON(map[string]any{"method": "block"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	// The first message exceeds pipe capacity; subsequent messages fill the
	// bounded queue. Overflow must terminate the session, not silently drop RPCs.
	if err := c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 2<<20))); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if err := c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 2<<20))); err != nil {
			break
		}
	}
	socketClosed(t, c)
	awaitCleanup(t, h, p)
}
