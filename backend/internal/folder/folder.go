package folder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type Manager struct {
	mu        sync.RWMutex
	current   string // empty = no folder open yet
	dataDir   string
	recents   []string
	listeners []func(string) // called when workspace changes
}

func New(defaultPath string) *Manager {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".wede")
	os.MkdirAll(dataDir, 0755)

	m := &Manager{
		dataDir: dataDir,
	}
	m.loadRecents()

	if defaultPath != "" {
		abs, err := filepath.Abs(defaultPath)
		if err == nil {
			defaultPath = abs
		}
		if info, err := os.Stat(defaultPath); err == nil && info.IsDir() {
			m.current = defaultPath
			m.addRecent(defaultPath)
		}
	}

	return m
}

func (m *Manager) Current() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *Manager) HasWorkspace() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current != ""
}

func (m *Manager) OnChange(fn func(string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

func (m *Manager) SetWorkspace(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", abs)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", abs)
	}

	m.mu.Lock()
	m.current = abs
	m.addRecent(abs)
	listeners := make([]func(string), len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.Unlock()

	m.saveRecents()
	for _, fn := range listeners {
		fn(abs)
	}
	return nil
}

func (m *Manager) Recents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.recents))
	copy(out, m.recents)
	return out
}

func (m *Manager) addRecent(path string) {
	filtered := []string{path}
	for _, r := range m.recents {
		if r != path {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) > 20 {
		filtered = filtered[:20]
	}
	m.recents = filtered
}

func (m *Manager) recentsFile() string {
	return filepath.Join(m.dataDir, "recent.json")
}

func (m *Manager) loadRecents() {
	data, err := os.ReadFile(m.recentsFile())
	if err != nil {
		return
	}
	json.Unmarshal(data, &m.recents)
}

func (m *Manager) saveRecents() {
	m.mu.RLock()
	data, _ := json.MarshalIndent(m.recents, "", "  ")
	m.mu.RUnlock()
	os.WriteFile(m.recentsFile(), data, 0644)
}

// HTTP Handlers

func (m *Manager) HandleGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"current": m.Current(),
		"recents": m.Recents(),
		"hasWorkspace": m.HasWorkspace(),
	})
}

func (m *Manager) HandleOpen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "path required"})
		return
	}

	// Expand ~ to home directory
	if strings.HasPrefix(body.Path, "~/") || body.Path == "~" {
		home, _ := os.UserHomeDir()
		body.Path = filepath.Join(home, body.Path[1:])
	}

	if err := m.SetWorkspace(body.Path); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"current": m.Current(),
	})
}

// Browse lists directories at a given path for the folder picker.
// The response is restricted to the user's home directory tree; paths outside
// the home directory are rejected so an authenticated user cannot enumerate
// the whole filesystem (e.g. ?path=/etc).
func (m *Manager) HandleBrowse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	home, _ := os.UserHomeDir()

	reqPath := r.URL.Query().Get("path")
	if reqPath == "" || reqPath == "~" {
		reqPath = home
	}
	if strings.HasPrefix(reqPath, "~/") {
		reqPath = filepath.Join(home, reqPath[2:])
	}

	abs, err := filepath.Abs(reqPath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid path"})
		return
	}

	// Confine browsing to the home directory tree.
	if !strings.HasPrefix(abs, home+string(filepath.Separator)) && abs != home {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside home directory"})
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	type DirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	dirs := []DirEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		dirs = append(dirs, DirEntry{
			Name: name,
			Path: filepath.Join(abs, name),
		})
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}

	// Detect common root locations
	roots := []DirEntry{
		{Name: "Home", Path: home},
	}
	if runtime.GOOS == "darwin" {
		roots = append(roots, DirEntry{Name: "Documents", Path: filepath.Join(home, "Documents")})
		roots = append(roots, DirEntry{Name: "Desktop", Path: filepath.Join(home, "Desktop")})
		roots = append(roots, DirEntry{Name: "Projects", Path: filepath.Join(home, "Projects")})
	} else {
		roots = append(roots, DirEntry{Name: "Documents", Path: filepath.Join(home, "Documents")})
		roots = append(roots, DirEntry{Name: "Projects", Path: filepath.Join(home, "projects")})
	}
	roots = append(roots, DirEntry{Name: "Root", Path: "/"})

	// Filter roots to only existing directories
	validRoots := []DirEntry{}
	for _, root := range roots {
		if info, err := os.Stat(root.Path); err == nil && info.IsDir() {
			validRoots = append(validRoots, root)
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"path":    abs,
		"parent":  parent,
		"dirs":    dirs,
		"roots":   validRoots,
	})
}
