package room

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"wede/backend/internal/workspace"
)

func TestCreateAndIsolation(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	m := NewManager()
	a, err := m.Create("alpha", dirA)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	b, err := m.Create("", dirB) // empty name -> base of path
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	if a.ID == b.ID {
		t.Fatal("expected distinct room ids")
	}
	if a.Root() == b.Root() {
		t.Fatalf("expected distinct roots, both %q", a.Root())
	}
	if a.Name != "alpha" {
		t.Errorf("name = %q, want alpha", a.Name)
	}
	if b.Name == "" {
		t.Error("empty name should default to path base")
	}

	// Mutating one room's workspace must not affect the other.
	if a.Root() == b.Root() {
		t.Fatal("rooms share a root — not isolated")
	}
}

func TestCreateRejectsBadPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Create("x", "/no/such/path/wede-test"); err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestGetListClose(t *testing.T) {
	m := NewManager()
	r1, _ := m.Create("one", t.TempDir())
	r2, _ := m.Create("two", t.TempDir())

	if got, ok := m.Get(r1.ID); !ok || got.ID != r1.ID {
		t.Fatalf("Get(%s) failed", r1.ID)
	}
	if _, ok := m.Get("missing"); ok {
		t.Fatal("Get(missing) should fail")
	}

	list := m.List()
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	if list[0].ID != r1.ID || list[1].ID != r2.ID {
		t.Error("List did not preserve creation order")
	}

	if !m.Close(r1.ID) {
		t.Fatal("Close(r1) returned false")
	}
	if m.Close(r1.ID) {
		t.Fatal("double Close should return false")
	}
	if len(m.List()) != 1 {
		t.Fatalf("after close, List len = %d, want 1", len(m.List()))
	}
}

func TestLazyHandlersAreStable(t *testing.T) {
	m := NewManager()
	r, _ := m.Create("x", t.TempDir())

	if r.Files() != r.Files() {
		t.Error("Files() should return a stable instance")
	}
	if r.Git() != r.Git() {
		t.Error("Git() should return a stable instance")
	}
	if r.Search() != r.Search() {
		t.Error("Search() should return a stable instance")
	}
}

func TestWatcherLifecycle(t *testing.T) {
	m := NewManager()
	r, _ := m.Create("x", t.TempDir())

	w := r.Watcher()
	if w == nil {
		t.Fatal("Watcher() returned nil")
	}
	if r.Watcher() != w {
		t.Error("Watcher() should return a stable instance")
	}

	// Close tears the room down (incl. its watcher) without panicking, and the
	// room is gone from the manager afterward.
	if !m.Close(r.ID) {
		t.Fatal("Close returned false")
	}
	if _, ok := m.Get(r.ID); ok {
		t.Error("room still present after Close")
	}
}

func TestScopedDispatch(t *testing.T) {
	m := NewManager()
	r, _ := m.Create("x", t.TempDir())

	called := false
	h := m.Scoped(func(rm *Room) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			if rm.ID != r.ID {
				t.Errorf("dispatched to wrong room: %s", rm.ID)
			}
			called = true
			w.WriteHeader(http.StatusOK)
		}
	})

	// Found: dispatches to the picked handler.
	req := httptest.NewRequest(http.MethodGet, "/api/rooms/"+r.ID+"/files", nil)
	req.SetPathValue("id", r.ID)
	rec := httptest.NewRecorder()
	h(rec, req)
	if !called {
		t.Fatal("scoped handler was not invoked for existing room")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	// Missing: 404, picked handler never runs.
	called = false
	req2 := httptest.NewRequest(http.MethodGet, "/api/rooms/missing/files", nil)
	req2.SetPathValue("id", "missing")
	rec2 := httptest.NewRecorder()
	h(rec2, req2)
	if called {
		t.Error("scoped handler ran for a missing room")
	}
	if rec2.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec2.Code)
	}
}

func TestRegisterAdoptsWorkspace(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir)
	m := NewManager()
	r := m.Register("seeded", ws)

	if r.Root() != ws.Current() {
		t.Errorf("adopted root = %q, want %q", r.Root(), ws.Current())
	}
	if r.Workspace() != ws {
		t.Error("Register should adopt the exact workspace instance")
	}
}
