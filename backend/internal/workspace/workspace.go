// Package workspace introduces the multi-project ("workspace") model that replaces wede's
// former single global workspace. Each Workspace owns an isolated project root via its
// own folder.Manager, which satisfies the WorkspaceProvider interface that the
// files/git/search/filewatcher/terminal/lsp handlers depend on. A RoomManager owns
// the set of live workspaces.
//
// This is the backbone of the collaborative rebuild: per-workspace state instead of one
// global singleton. Route-scoping of the per-workspace services (/api/workspaces/{id}/...) is
// layered on in subsequent slices; this file establishes the registry and lifecycle.
package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ywebsocket "github.com/reearth/ygo/provider/websocket"

	"wede/backend/internal/chat"
	"wede/backend/internal/collab"
	"wede/backend/internal/collabdoc"
	"wede/backend/internal/filewatcher"
	"wede/backend/internal/files"
	"wede/backend/internal/folder"
	"wede/backend/internal/git"
	"wede/backend/internal/lsp"
	"wede/backend/internal/presence"
	"wede/backend/internal/search"
	"wede/backend/internal/terminal"
)

// Workspace is one open project. It is rooted at an immutable path on disk, surfaced
// through its folder.Manager (which the per-workspace service handlers consume).
// The service handlers (files/git/search) are constructed lazily and bound to
// this workspace's workspace, so each workspace operates on its own isolated root.
type Workspace struct {
	ID   string
	Name string

	ws *folder.Manager

	// frameAncestors is threaded into the terminal/lsp WebSocket origin checks.
	frameAncestors string

	mu       sync.Mutex
	files    *files.Handler
	git      *git.Handler
	search   *search.Handler
	watcher  *filewatcher.Handler
	terminal *terminal.Handler
	lsp      *lsp.Handler
	presence  *presence.Hub
	collab    *collab.Handler
	chatPublic  *chat.Hub
	chatPrivate *chat.Hub
	docs       *collabdoc.DocStore
	docServer  *ywebsocket.Server          // ygo sync+awareness WS server (one doc per file)
	docPersist *collabdoc.DiskPersistence  // seeds from + writes back to disk
}

// Workspace returns the workspace's folder.Manager, satisfying the WorkspaceProvider
// interfaces (Current/OnChange) the service handlers require.
func (r *Workspace) Folder() *folder.Manager { return r.ws }

// Root is the absolute project path this workspace is pinned to.
func (r *Workspace) Root() string { return r.ws.Current() }

// Files returns this workspace's files handler, bound to the workspace's workspace.
func (r *Workspace) Files() *files.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.files == nil {
		r.files = files.New(r.ws)
	}
	return r.files
}

// Git returns this workspace's git handler, bound to the workspace's workspace.
func (r *Workspace) Git() *git.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.git == nil {
		r.git = git.New(r.ws)
	}
	return r.git
}

// Search returns this workspace's search handler, bound to the workspace's workspace.
func (r *Workspace) Search() *search.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.search == nil {
		r.search = search.New(r.ws)
	}
	return r.search
}

// Watcher returns this workspace's filewatcher, lazily starting an fsnotify watch on
// the workspace's root on first use.
func (r *Workspace) Watcher() *filewatcher.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.watcher == nil {
		r.watcher = filewatcher.New(r.ws)
	}
	return r.watcher
}

// Terminal returns this workspace's terminal handler (shared PTY sessions), lazily
// constructed and bound to the workspace's root.
func (r *Workspace) Terminal() *terminal.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminal == nil {
		r.terminal = terminal.New(r.ws, r.frameAncestors)
	}
	return r.terminal
}

// LSP returns this workspace's language-server proxy, lazily constructed and bound to
// the workspace's root.
func (r *Workspace) LSP() *lsp.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lsp == nil {
		r.lsp = lsp.New(r.ws, r.frameAncestors)
	}
	return r.lsp
}

// Presence returns this workspace's presence hub (who is connected and what they are
// viewing), lazily created on first use.
func (r *Workspace) Presence() *presence.Hub {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.presence == nil {
		r.presence = presence.NewHub()
	}
	return r.presence
}

// Collab returns this workspace's collaboration WebSocket handler, bound to the
// workspace's presence hub. Lazily created on first use.
func (r *Workspace) Collab() *collab.Handler {
	hub := r.Presence() // acquires/releases r.mu internally; must not hold it here
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.collab == nil {
		r.collab = collab.New(r.frameAncestors, hub)
	}
	return r.collab
}

// Chat returns this workspace's chat hub for the given channel ("public" or
// "private"; anything else falls back to public), lazily created on first use.
// Public persists to <root>/.wede/chat.md (committed, LLM-readable) and polls git
// for activity; private persists to <root>/.wede/private/chat.md, which wede
// gitignores by default.
//
//	GET /api/workspaces/{id}/chat?channel=public|private -> Chat(channel).HandleWS
func (r *Workspace) Chat(channel string) *chat.Hub {
	// .wede lives under the chosen host folder (root/host); git still resolves
	// upward from there, so the chat hub gets the correct .wede location.
	hostRoot := r.wedeHostRoot() // reads ws.Current() + persisted host; does not take r.mu
	r.mu.Lock()
	defer r.mu.Unlock()
	if channel == chat.ChannelPrivate {
		if r.chatPrivate == nil {
			r.chatPrivate = chat.NewHub(hostRoot, chat.ChannelPrivate)
		}
		return r.chatPrivate
	}
	if r.chatPublic == nil {
		r.chatPublic = chat.NewHub(hostRoot, chat.ChannelPublic)
	}
	return r.chatPublic
}

// Docs returns this workspace's collaborative document store (server-authoritative
// CRDT doc per open file), lazily created on first use.
func (r *Workspace) Docs() *collabdoc.DocStore {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.docs == nil {
		r.docs = collabdoc.NewDocStore()
	}
	return r.docs
}

// DocServer returns this workspace's ygo collaboration server (y-protocols sync +
// awareness), lazily created. The provider "workspace" name is a file's workspace-relative
// path; documents are seeded from disk via DiskPersistence rooted at this workspace.
func (r *Workspace) DocServer() *ywebsocket.Server {
	root := r.Root() // reads ws.Current(); does not take r.mu
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.docServer == nil {
		persist := collabdoc.NewDiskPersistence(root)
		srv := ywebsocket.NewServerWithPersistence(persist)
		persist.SetProvider(srv) // enable debounced write-back of edits to disk
		if r.frameAncestors != "" {
			srv.AllowedOrigins = strings.Fields(r.frameAncestors)
		}
		r.docServer = srv
		r.docPersist = persist
	}
	return r.docServer
}

// shutdown tears down the workspace's long-lived subsystems. Called by Manager.Close.
func (r *Workspace) shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.watcher != nil {
		r.watcher.Close()
		r.watcher = nil
	}
	if r.terminal != nil {
		r.terminal.Close()
		r.terminal = nil
	}
	if r.lsp != nil {
		r.lsp.Close()
		r.lsp = nil
	}
	if r.presence != nil {
		r.presence.Close()
		r.presence = nil
	}
	if r.chatPublic != nil {
		r.chatPublic.Close()
		r.chatPublic = nil
	}
	if r.chatPrivate != nil {
		r.chatPrivate.Close()
		r.chatPrivate = nil
	}
	if r.docs != nil {
		r.docs.CloseAll()
		r.docs = nil
	}
	if r.docPersist != nil {
		r.docPersist.Stop() // final flush of pending edits while docs are still alive
		r.docPersist = nil
	}
	if r.docServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		r.docServer.Shutdown(ctx) //nolint:errcheck
		cancel()
		r.docServer = nil
	}
}

// Manager owns the set of live workspaces. Safe for concurrent use.
type Manager struct {
	mu             sync.RWMutex
	workspaces          map[string]*Workspace
	order          []string // preserves creation order for stable listing
	frameAncestors string   // threaded into per-workspace terminal/lsp origin checks
}

// NewManager returns an empty RoomManager. frameAncestors mirrors the
// frame_ancestors config and is passed to each workspace's terminal/lsp handlers for
// WebSocket origin checking.
func NewManager(frameAncestors string) *Manager {
	return &Manager{workspaces: make(map[string]*Workspace), frameAncestors: frameAncestors}
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
func (m *Manager) register(r *Workspace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaces[r.ID] = r
	m.order = append(m.order, r.ID)
}

// Register adopts an existing folder.Manager as a workspace. Used to seed the
// default workspace from the boot workspace so the solo-user case works with no setup.
func (m *Manager) Register(name string, ws *folder.Manager) *Workspace {
	if name == "" {
		name = filepath.Base(ws.Current())
	}
	r := &Workspace{ID: newID(), Name: name, ws: ws, frameAncestors: m.frameAncestors}
	m.register(r)
	return r
}

// Create opens a new workspace rooted at the given path. The path is expanded (~),
// absolutised, and validated as an existing directory.
func (m *Manager) Create(name, root string) (*Workspace, error) {
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
	ws := folder.New(abs)
	if !ws.HasWorkspace() {
		return nil, fmt.Errorf("invalid workspace path: %s", abs)
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	r := &Workspace{ID: newID(), Name: name, ws: ws, frameAncestors: m.frameAncestors}
	m.register(r)
	return r, nil
}

// Get returns the workspace with the given id.
func (m *Manager) Get(id string) (*Workspace, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.workspaces[id]
	return r, ok
}

// List returns workspaces in creation order.
func (m *Manager) List() []*Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Workspace, 0, len(m.order))
	for _, id := range m.order {
		if r, ok := m.workspaces[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Close removes a workspace from the manager and tears down its long-lived
// subsystems (currently the filewatcher; terminal/lsp join as they are scoped).
func (m *Manager) Close(id string) bool {
	m.mu.Lock()
	r, ok := m.workspaces[id]
	if !ok {
		m.mu.Unlock()
		return false
	}
	delete(m.workspaces, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	// Tear down outside the manager lock so subsystem shutdown can't block other
	// workspace operations.
	r.shutdown()
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
