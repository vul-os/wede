package lsp

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestMessageBounds(t *testing.T) {
	for _, raw := range []string{
		fmt.Sprintf("Content-Length: %d\r\n\r\n", maxMessageBytes+1),
		"Content-Length: -1\r\n\r\n",
		"Content-Length: 99999999999999999999999999\r\n\r\n",
		"Content-Length: 1\r\nContent-Length: 1\r\n\r\nx",
		strings.Repeat("x", maxHeaderBytes+1),
		strings.Repeat("X: x\r\n", maxHeaderBytes/6+1) + "\r\n",
	} {
		if _, err := readLSPMessage(bufio.NewReader(strings.NewReader(raw))); err == nil {
			t.Fatal("accepted invalid/oversized frame")
		}
	}
	body := strings.Repeat("x", maxMessageBytes)
	prefix := fmt.Sprintf("Content-Length: %d\r\n", len(body))
	// Both limits inclusive, including the final blank line in the header budget.
	header := prefix + "X:" + strings.Repeat("x", maxHeaderBytes-len(prefix)-6) + "\r\n\r\n"
	if len(header) != maxHeaderBytes {
		t.Fatal("incorrect test header")
	}
	got, err := readLSPMessage(bufio.NewReader(strings.NewReader(header + body)))
	if err != nil || len(got) != len(body) {
		t.Fatalf("boundary frame rejected: %d, %v", len(got), err)
	}
}

func TestOversizedTrafficCleanup(t *testing.T) {
	for _, direction := range []string{"websocket", "large-output", "large-header"} {
		t.Run(direction, func(t *testing.T) {
			h, _, url := lifecycleHarness(t)
			c := dialLSP(t, url)
			p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
			if direction == "websocket" {
				// The server may close as soon as it sees the oversized frame header.
				c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", maxMessageBytes+1)))
			} else if err := c.WriteJSON(map[string]any{"method": direction}); err != nil {
				t.Fatal(err)
			}
			socketClosed(t, c)
			awaitCleanup(t, h, p)
		})
	}
}

func TestHealthyBurst(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	c := dialLSP(t, url)
	p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
	const count = 256
	// A short server pause forces many requests to queue before it drains them.
	for id := 2; id < count+2; id++ {
		method := "hover"
		if id == 2 {
			method = "pause"
		}
		if err := c.WriteJSON(map[string]any{"id": id, "method": method, "params": fmt.Sprint(id)}); err != nil {
			t.Fatal(err)
		}
	}
	for id := 2; id < count+2; id++ {
		var response struct {
			ID     int
			Result helperResult
		}
		if err := c.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if response.ID != id || response.Result.Echo != fmt.Sprint(id) || !response.Result.Valid {
			t.Fatalf("lost/reordered response: %+v", response)
		}
	}
	c.Close()
	awaitCleanup(t, h, p)
}

func TestQueueBudgetGraceAndCancellation(t *testing.T) {
	stop := make(chan struct{})
	q := newMessageQueue()
	message := make([]byte, maxMessageBytes-queueMessageOverhead)
	for range 2 {
		if err := q.push(message, stop, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	inFlight, ok := q.pop(stop)
	if !ok {
		t.Fatal("queue empty")
	}
	// Removing from the queue doesn't free the byte reservation until written.
	if err := q.push([]byte("small"), stop, 10*time.Millisecond); !errors.Is(err, errQueueOverloaded) {
		t.Fatalf("in-flight bytes not charged: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- q.push([]byte("small"), stop, time.Second) }()
	q.written(inFlight)
	if err := <-done; err != nil {
		t.Fatalf("grace did not allow drain: %v", err)
	}
	// Fill the remaining space, then cancellation must beat the long grace.
	if err := q.push(make([]byte, len(message)-len("small")-queueMessageOverhead), stop, time.Second); err != nil {
		t.Fatal(err)
	}
	go func() { done <- q.push(nil, stop, time.Hour) }()
	close(stop)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled push succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("queue ignored cancellation")
	}
}

func TestMaxSessionsOption(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{{"", 4}, {"0", 4}, {"-1", 4}, {"invalid", 4}, {"2", 2}, {" 6 ", 6}} {
		t.Setenv("WEDE_LSP_MAX_SESSIONS", tc.value)
		if got := maxSessions(); got != tc.want {
			t.Fatalf("%q: got %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestLimiterConcurrency(t *testing.T) {
	limiter := sessionLimiter{changed: make(chan struct{})}
	cancel := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := limiter.acquire(cancel, 2, time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			limiter.mu.Lock()
			active := limiter.active
			limiter.mu.Unlock()
			if active > 2 {
				t.Errorf("cap exceeded: %d", active)
			}
			time.Sleep(time.Millisecond)
			release()
			release() // Release is idempotent.
		}()
	}
	wg.Wait()
	if limiter.active != 0 {
		t.Fatalf("leaked reservations: %d", limiter.active)
	}
	release, err := limiter.acquire(cancel, 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limiter.acquire(cancel, 1, 10*time.Millisecond); err == nil {
		t.Fatal("over-cap admission succeeded")
	}
	done := make(chan error, 1)
	go func() { _, err := limiter.acquire(cancel, 1, time.Hour); done <- err }()
	close(cancel)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled admission succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("admission ignored cancellation")
	}
	release()
}

func TestGlobalCapAcrossHandlers(t *testing.T) {
	t.Setenv("WEDE_LSP_MAX_SESSIONS", "1")
	h1, ws, url1 := lifecycleHarness(t)
	// A separate handler, but the same process-wide admission budget.
	h2 := New(ws, "")
	server := httptest.NewServer(http.HandlerFunc(h2.HandleWS))
	t.Cleanup(server.Close)
	t.Cleanup(h2.Close)
	url2 := "ws" + strings.TrimPrefix(server.URL, "http") + "?lang=lifecycle-test"
	c1 := dialLSP(t, url1)
	p1 := processFor(t, h1, request(t, c1, 1, "initialize", "first").PID)
	rejected := dialLSP(t, url2)
	rejected.SetReadDeadline(time.Now().Add(5 * time.Second))
	var notice struct {
		Method string
		Params struct{ Message string }
	}
	if err := rejected.ReadJSON(&notice); err != nil {
		t.Fatal(err)
	}
	if notice.Method != "window/showMessage" || !strings.Contains(notice.Params.Message, "global session limit (1)") {
		t.Fatalf("missing overload message: %+v", notice)
	}
	socketClosed(t, rejected)
	request(t, c1, 2, "hover", "unaffected")
	// Reconnect while the old process still occupies the slot. It should wait,
	// rather than immediately reject or start a second process.
	reconnect := dialLSP(t, url2)
	type result struct {
		r   helperResult
		err error
	}
	response := make(chan result, 1)
	go func() { r, err := rpc(reconnect, 1, "initialize", "reconnected"); response <- result{r, err} }()
	select {
	case r := <-response:
		t.Fatalf("admission did not wait: %+v", r)
	case <-time.After(50 * time.Millisecond):
	}
	c1.Close()
	awaitCleanup(t, h1, p1)
	r := <-response
	if r.err != nil {
		t.Fatal(r.err)
	}
	p2 := processFor(t, h2, r.r.PID)
	h1.Close() // Closing an unrelated handler must not release h2's slot.
	globalSessions.mu.Lock()
	active := globalSessions.active
	globalSessions.mu.Unlock()
	if active != 1 {
		t.Fatalf("incorrect global accounting: %d", active)
	}
	h2.Close()
	awaitCleanup(t, h2, p2)
	globalSessions.mu.Lock()
	active = globalSessions.active
	globalSessions.mu.Unlock()
	if active != 0 {
		t.Fatalf("leaked global slot: %d", active)
	}
}

func TestHeartbeatLiveness(t *testing.T) {
	// Exercise the production transport helpers with shorter intervals, without
	// adding deployment knobs solely for tests.
	for _, respond := range []bool{true, false} {
		t.Run(fmt.Sprintf("pong=%v", respond), func(t *testing.T) {
			finished := make(chan struct{})
			ready := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					close(finished)
					return
				}
				defer close(finished)
				defer conn.Close()
				configureWebSocket(conn, 300*time.Millisecond)
				stop, joined := make(chan struct{}), make(chan struct{})
				go func() { defer close(joined); heartbeat(conn, stop, 40*time.Millisecond) }()
				defer func() { close(stop); <-joined }()
				close(ready)
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}))
			defer server.Close()
			c := dialLSP(t, "ws"+strings.TrimPrefix(server.URL, "http"))
			<-ready
			if !respond {
				c.SetPingHandler(func(string) error { return nil })
			}
			readerDone := make(chan struct{})
			go func() {
				defer close(readerDone)
				for {
					if _, _, err := c.ReadMessage(); err != nil {
						return
					}
				}
			}()
			if respond {
				select {
				case <-finished:
					t.Fatal("healthy pong client timed out")
				case <-time.After(700 * time.Millisecond):
				}
				c.Close()
			}
			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Fatal("missing pong did not close session")
			}
			<-readerDone
		})
	}
}

func TestSlowWebSocketReaderDeadline(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	c := dialLSP(t, url)
	p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
	if err := c.WriteJSON(map[string]any{"method": "flood"}); err != nil {
		t.Fatal(err)
	}
	// No more reads or Close: a stalled peer must be evicted by the data-write
	// deadline, before the 20-second heartbeat or 60-second pong timeout.
	select {
	case <-p.done:
	case <-time.After(12 * time.Second):
		t.Fatal("slow reader survived write deadline")
	}
	awaitCleanup(t, h, p)
}

func TestLargeHealthyMessage(t *testing.T) {
	h, _, url := lifecycleHarness(t)
	c := dialLSP(t, url)
	p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
	request(t, c, 2, "hover", strings.Repeat("x", maxMessageBytes-1024))
	c.Close()
	awaitCleanup(t, h, p)
}

func TestAdmissionFailureAndCancellationReleaseCapacity(t *testing.T) {
	t.Setenv("WEDE_LSP_MAX_SESSIONS", "1")
	h, ws, _ := lifecycleHarness(t)
	// Exercise actual start failure after reserving the only global slot.
	if _, err := h.startServer(nil, 0, "/nonexistent-wede-lsp-test-binary", nil); err == nil {
		t.Fatal("unexpected start success")
	}
	globalSessions.mu.Lock()
	active := globalSessions.active
	globalSessions.mu.Unlock()
	if active != 0 {
		t.Fatalf("failed start leaked %d slots", active)
	}
	release, err := globalSessions.acquire(make(chan struct{}), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() { _, err := h.startServer(nil, 0, "must-not-start", nil); done <- err }()
	ws.change(t.TempDir())
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stale admission succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("workspace change did not cancel admission")
	}
	globalSessions.mu.Lock()
	active = globalSessions.active
	globalSessions.mu.Unlock()
	if active != 1 {
		t.Fatalf("cancelled admission changed another owner's slot: %d", active)
	}
}
