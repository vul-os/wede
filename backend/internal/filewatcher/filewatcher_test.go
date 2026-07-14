package filewatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockWS satisfies WorkspaceProvider for tests.
type mockWS struct {
	dir       string
	listeners []func(string)
}

func (m *mockWS) Current() string { return m.dir }
func (m *mockWS) OnChange(fn func(string)) {
	m.listeners = append(m.listeners, fn)
}
func (m *mockWS) changeDir(dir string) {
	m.dir = dir
	for _, fn := range m.listeners {
		fn(dir)
	}
}

func TestHandlerBroadcast(t *testing.T) {
	tmp := t.TempDir()
	ws := &mockWS{dir: tmp}
	h := New(ws)
	// Give the watcher goroutine time to start.
	time.Sleep(50 * time.Millisecond)

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	// Write a file — the watcher should pick it up and debounce.
	if err := os.WriteFile(filepath.Join(tmp, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-ch:
		// success
	case <-time.After(2 * time.Second):
		t.Error("timeout: no broadcast received after file write")
	}
}

func TestHandlerWorkspaceChange(t *testing.T) {
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()

	ws := &mockWS{dir: tmp1}
	h := New(ws)
	time.Sleep(50 * time.Millisecond)

	// Switch workspace.
	ws.changeDir(tmp2)
	time.Sleep(50 * time.Millisecond)

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	// Write to the NEW workspace — should trigger broadcast.
	os.WriteFile(filepath.Join(tmp2, "new.txt"), []byte("x"), 0644)

	select {
	case <-ch:
		// success
	case <-time.After(2 * time.Second):
		t.Error("timeout: no broadcast after workspace change + file write")
	}
}

func TestHandlerSSE_Ping(t *testing.T) {
	ws := &mockWS{dir: ""}
	h := New(ws)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/watch", nil).WithContext(ctx)
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.HandleSSE(rec, req)
		close(done)
	}()

	// Wait for the initial ping event.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			cancel()
			<-done
			t.Error("timeout waiting for ping event")
			return
		case <-time.After(30 * time.Millisecond):
			if strings.Contains(rec.body(), "ping") {
				cancel()
				<-done
				return
			}
		}
	}
}

func TestHandlerSSE_ReceivesChangeEvent(t *testing.T) {
	tmp := t.TempDir()
	ws := &mockWS{dir: tmp}
	h := New(ws)
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/watch", nil).WithContext(ctx)
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	go h.HandleSSE(rec, req)

	// Trigger a file change.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(tmp, "trigger.go"), []byte("package x"), 0644)

	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			cancel()
			t.Error("timeout waiting for change event")
			return
		case <-time.After(50 * time.Millisecond):
			if strings.Contains(rec.body(), `"change"`) {
				cancel()
				return
			}
		}
	}
}

// flushRecorder wraps httptest.ResponseRecorder and implements http.Flusher.
// The SSE handler writes from its own goroutine while the test polls body(), so
// both the write and the read are guarded by mu — httptest.ResponseRecorder's
// underlying bytes.Buffer is not safe for concurrent access.
type flushRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func (f *flushRecorder) Flush() {} // ResponseRecorder buffers; no-op for test

func (f *flushRecorder) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(p)
}

func (f *flushRecorder) body() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Body.String()
}
