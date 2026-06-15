// Package filewatcher uses fsnotify to watch the active workspace and push
// change events to connected clients via Server-Sent Events (SSE).
//
// Events are debounced (250 ms quiet window) so rapid file system activity
// (e.g. a build writing many files) produces a single notification rather
// than a storm.
//
// Clients subscribe via GET /api/watch.  The response is a text/event-stream;
// each event is a JSON payload with at least a "type" field:
//
//	{"type":"change"}      — one or more files changed (explorer should refresh)
//	{"type":"ping"}        — keepalive sent every 15 s
package filewatcher

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WorkspaceProvider is satisfied by workspace.Manager.
type WorkspaceProvider interface {
	Current() string
	OnChange(func(string))
}

// Handler manages fsnotify watchers and SSE subscribers.
type Handler struct {
	ws      WorkspaceProvider
	mu      sync.Mutex
	subs    map[chan struct{}]struct{} // broadcast channels
	watcher *fsnotify.Watcher
	wsDir   string // currently watched directory
}

// New creates a Handler that begins watching the workspace immediately (if
// one is already set) and reacts to workspace changes.
func New(ws WorkspaceProvider) *Handler {
	h := &Handler{
		ws:   ws,
		subs: make(map[chan struct{}]struct{}),
	}

	// Start watching the current workspace.
	if dir := ws.Current(); dir != "" {
		h.startWatching(dir)
	}

	// Re-watch when the workspace changes.
	ws.OnChange(func(dir string) {
		h.startWatching(dir)
	})

	return h
}

// startWatching replaces the current fsnotify watcher with one rooted at dir.
func (h *Handler) startWatching(dir string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.watcher != nil {
		h.watcher.Close() // closes the goroutine started below
		h.watcher = nil
	}

	if dir == "" {
		h.wsDir = ""
		return
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[filewatcher] failed to create watcher: %v", err)
		return
	}

	if err := w.Add(dir); err != nil {
		log.Printf("[filewatcher] failed to watch %q: %v", dir, err)
		w.Close()
		return
	}

	h.watcher = w
	h.wsDir = dir

	go h.debounce(w)
}

// debounce collects fsnotify events for 250 ms then broadcasts a single
// change notification to all subscribers.
func (h *Handler) debounce(w *fsnotify.Watcher) {
	const quiet = 250 * time.Millisecond
	timer := time.NewTimer(24 * time.Hour) // starts dormant
	timer.Stop()

	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				timer.Stop()
				return
			}
			// Ignore chmod-only events — they're very chatty and don't affect content.
			if ev.Op == fsnotify.Chmod {
				continue
			}
			// Reset debounce window.
			timer.Stop()
			timer.Reset(quiet)

		case err, ok := <-w.Errors:
			if !ok {
				timer.Stop()
				return
			}
			log.Printf("[filewatcher] watcher error: %v", err)

		case <-timer.C:
			h.broadcast()
		}
	}
}

// broadcast notifies all active SSE subscribers.
func (h *Handler) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		// Non-blocking send so a slow subscriber doesn't block others.
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// subscribe registers a new subscriber and returns its channel.
func (h *Handler) subscribe() chan struct{} {
	ch := make(chan struct{}, 4)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// unsubscribe removes a subscriber.
func (h *Handler) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// HandleSSE serves the Server-Sent Events endpoint.
func (h *Handler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	// Helper: write an SSE event.
	send := func(payload any) {
		data, _ := json.Marshal(payload)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		flusher.Flush()
	}

	// Send an initial ping so the client knows the stream is open.
	send(map[string]string{"type": "ping"})

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			send(map[string]string{"type": "change"})
		case <-ping.C:
			send(map[string]string{"type": "ping"})
		}
	}
}
