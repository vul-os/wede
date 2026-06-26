// Package room introduces the multi-project ("room") model that replaces wede's
// former single global workspace. Each Room owns an isolated project root via its
// own workspace.Manager, which satisfies the WorkspaceProvider interface that the
// files/git/search/filewatcher/terminal/lsp handlers depend on. A RoomManager owns
// the set of live rooms.
//
// This is the backbone of the collaborative rebuild: per-room state instead of one
// global singleton. Route-scoping of the per-room services (/api/rooms/{id}/...) is
// layered on in subsequent slices; this file establishes the registry and lifecycle.
package room

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"wede/backend/internal/workspace"
)

// Room is one open project. It is rooted at an immutable path on disk, surfaced
// through its workspace.Manager (which the per-room service handlers consume).
type Room struct {
	ID   string
	Name string

	ws *workspace.Manager
}

// Workspace returns the room's workspace.Manager, satisfying the WorkspaceProvider
// interfaces (Current/OnChange) the service handlers require.
func (r *Room) Workspace() *workspace.Manager { return r.ws }

// Root is the absolute project path this room is pinned to.
func (r *Room) Root() string { return r.ws.Current() }

// Manager owns the set of live rooms. Safe for concurrent use.
type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room
	order []string // preserves creation order for stable listing
}

// NewManager returns an empty RoomManager.
func NewManager() *Manager {
	return &Manager{rooms: make(map[string]*Room)}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail; fall back to a fixed-length marker so
		// callers still get a non-empty id rather than a panic.
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

// register inserts r under the manager's lock.
func (m *Manager) register(r *Room) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rooms[r.ID] = r
	m.order = append(m.order, r.ID)
}

// Register adopts an existing workspace.Manager as a room. Used to seed the
// default room from the boot workspace so the solo-user case works with no setup.
func (m *Manager) Register(name string, ws *workspace.Manager) *Room {
	if name == "" {
		name = filepath.Base(ws.Current())
	}
	r := &Room{ID: newID(), Name: name, ws: ws}
	m.register(r)
	return r
}

// Create opens a new room rooted at the given path. The path is expanded (~),
// absolutised, and validated as an existing directory.
func (m *Manager) Create(name, root string) (*Room, error) {
	root = expandHome(root)
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %s", abs)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", abs)
	}
	ws := workspace.New(abs)
	if !ws.HasWorkspace() {
		return nil, fmt.Errorf("invalid workspace path: %s", abs)
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	r := &Room{ID: newID(), Name: name, ws: ws}
	m.register(r)
	return r, nil
}

// Get returns the room with the given id.
func (m *Manager) Get(id string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rooms[id]
	return r, ok
}

// List returns rooms in creation order.
func (m *Manager) List() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Room, 0, len(m.order))
	for _, id := range m.order {
		if r, ok := m.rooms[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Close removes a room from the manager. (Tearing down per-room subsystems —
// watcher/lsp/pty — is added with the lazy lifecycle slice.)
func (m *Manager) Close(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rooms[id]; !ok {
		return false
	}
	delete(m.rooms, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return true
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
