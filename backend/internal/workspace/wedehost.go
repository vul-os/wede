package workspace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The .wede directory (chat history, saved API requests, etc.) lives at the
// workspace root by default. When a workspace contains several projects, the
// owner can choose which one hosts .wede so it's committed into that project's
// repo. The choice is a workspace-root-relative folder ("" = root) persisted in
// ~/.wede/wede-hosts.json keyed by absolute root path (workspaces are otherwise
// in-memory), and changing it MOVES the existing .wede contents.

var wedeHostMu sync.Mutex

func wedeHostsFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wede", "wede-hosts.json")
}

func loadWedeHostMap() map[string]string {
	m := map[string]string{}
	if data, err := os.ReadFile(wedeHostsFile()); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

// wedeHostFor returns the persisted relative host folder for a workspace root.
func wedeHostFor(root string) string {
	wedeHostMu.Lock()
	defer wedeHostMu.Unlock()
	return loadWedeHostMap()[root]
}

func saveWedeHostFor(root, rel string) {
	wedeHostMu.Lock()
	defer wedeHostMu.Unlock()
	m := loadWedeHostMap()
	if rel == "" {
		delete(m, root)
	} else {
		m[root] = rel
	}
	home, _ := os.UserHomeDir()
	_ = os.MkdirAll(filepath.Join(home, ".wede"), 0o700)
	data, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(wedeHostsFile(), data, 0o600)
}

// WedeHost returns the workspace-root-relative folder hosting .wede ("" = root).
func (r *Workspace) WedeHost() string { return wedeHostFor(r.Root()) }

// wedeHostRoot is the absolute folder that physically contains .wede (root/host).
// git commands run from here still find the enclosing repo by walking up.
func (r *Workspace) wedeHostRoot() string { return filepath.Join(r.Root(), r.WedeHost()) }

// WedeDir returns the absolute path to this workspace's .wede directory.
func (r *Workspace) WedeDir() string { return filepath.Join(r.wedeHostRoot(), ".wede") }

// cleanRel normalises a user-supplied relative path and rejects escapes.
func cleanRel(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if rel == "" || rel == "." {
		return "", nil
	}
	c := filepath.Clean(rel)
	if c == ".." || strings.HasPrefix(c, ".."+string(os.PathSeparator)) || filepath.IsAbs(c) {
		return "", fmt.Errorf("invalid folder")
	}
	return c, nil
}

// SetWedeHost relocates .wede into the given workspace-relative folder, moving any
// existing contents. Owner-only at the route layer.
func (r *Workspace) SetWedeHost(rel string) error {
	root := r.Root()
	rel, err := cleanRel(rel)
	if err != nil {
		return err
	}
	target := filepath.Join(root, rel)
	if rel != "" {
		info, err := os.Stat(target)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("folder does not exist: %s", rel)
		}
	}
	oldDir := r.WedeDir()
	newDir := filepath.Join(target, ".wede")
	if oldDir == newDir {
		return nil
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf(".wede already exists in %q — remove it first", rel)
	}

	// Pause chat hubs so their files aren't being written during the move; they
	// lazily reopen at the new location on next access.
	r.mu.Lock()
	if r.chatPublic != nil {
		r.chatPublic.Close()
		r.chatPublic = nil
	}
	if r.chatPrivate != nil {
		r.chatPrivate.Close()
		r.chatPrivate = nil
	}
	r.mu.Unlock()

	if _, err := os.Stat(oldDir); err == nil {
		if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
			return fmt.Errorf("move .wede: %w", err)
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			// Cross-device fallback: copy then remove.
			if err2 := os.CopyFS(newDir, os.DirFS(oldDir)); err2 != nil {
				return fmt.Errorf("move .wede: %w", err)
			}
			_ = os.RemoveAll(oldDir)
		}
	}
	saveWedeHostFor(root, rel)
	return nil
}

// topLevelFolders lists immediate sub-directories of the workspace root that are
// candidate .wede hosts (skips hidden dirs and common noise).
func (r *Workspace) topLevelFolders() []string {
	out := []string{}
	entries, err := os.ReadDir(r.Root())
	if err != nil {
		return out
	}
	skip := map[string]bool{"node_modules": true, "dist": true, "build": true, "vendor": true}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && !strings.HasPrefix(name, ".") && !skip[name] {
			out = append(out, name)
		}
	}
	return out
}

// ── HTTP handlers (owner-only) ────────────────────────────────────────────────
//   GET /api/workspaces/{id}/wede-location -> { host, dir, folders }
//   PUT /api/workspaces/{id}/wede-location  { host } -> moves + returns new state

func (r *Workspace) wedeLocationJSON() map[string]any {
	return map[string]any{
		"host":    r.WedeHost(),
		"dir":     r.WedeDir(),
		"folders": r.topLevelFolders(),
	}
}

// HandleWedeLocationGet returns the current .wede host + candidate folders.
func (r *Workspace) HandleWedeLocationGet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(r.wedeLocationJSON())
}

// HandleWedeLocationSet relocates .wede to the requested host folder.
func (r *Workspace) HandleWedeLocationSet(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Host string `json:"host"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}
	if err := r.SetWedeHost(body.Host); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(r.wedeLocationJSON())
}
