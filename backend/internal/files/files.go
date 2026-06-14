package files

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type WorkspaceProvider interface {
	Current() string
}

type Handler struct {
	ws WorkspaceProvider
}

func New(ws WorkspaceProvider) *Handler {
	return &Handler{ws: ws}
}

type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size,omitempty"`
}

func (h *Handler) workspace() string {
	return h.ws.Current()
}

func (h *Handler) checkWorkspace(w http.ResponseWriter) bool {
	if h.workspace() == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no workspace open"})
		return false
	}
	return true
}

func (h *Handler) safePath(reqPath string) (string, bool) {
	ws := h.workspace()
	if ws == "" {
		return "", false
	}
	if reqPath == "" || reqPath == "/" || reqPath == "." {
		return ws, true
	}
	// filepath.Join calls filepath.Clean internally, which resolves all ".."
	// sequences. We must NOT call Clean separately before Join because
	// Clean("../../etc/passwd") == "../../etc/passwd" (still has ".."), and
	// Join correctly collapses it: Join("/ws", "../../etc/passwd") == "/etc/passwd".
	full := filepath.Join(ws, reqPath)
	// Guard against prefix-collision attacks: if the workspace is "/tmp/ws",
	// a plain HasPrefix check would permit "/tmp/ws2/evil" because "/tmp/ws2"
	// starts with "/tmp/ws". Enforce the separator so we only allow exact
	// workspace root or files strictly inside it.
	wsWithSep := ws + string(filepath.Separator)
	if full != ws && !strings.HasPrefix(full, wsWithSep) {
		return "", false
	}
	return full, true
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	reqPath := r.URL.Query().Get("path")
	full, ok := h.safePath(reqPath)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	result := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == ".git" || name == "node_modules" || name == ".DS_Store" {
			continue
		}
		info, _ := e.Info()
		relPath := filepath.Join(reqPath, name)
		fe := FileEntry{
			Name:  name,
			Path:  relPath,
			IsDir: e.IsDir(),
		}
		if info != nil && !e.IsDir() {
			fe.Size = info.Size()
		}
		result = append(result, fe)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	json.NewEncoder(w).Encode(result)
}

func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	reqPath := r.URL.Query().Get("path")
	full, ok := h.safePath(reqPath)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	info, err := os.Stat(full)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "file not found"})
		return
	}

	if info.Size() > 10*1024*1024 {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large (>10MB)"})
		return
	}

	data, err := os.ReadFile(full)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"path":    reqPath,
		"content": string(data),
	})
}

func (h *Handler) Write(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 10*1024*1024)).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	full, ok := h.safePath(body.Path)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	dir := filepath.Dir(full)
	os.MkdirAll(dir, 0755)

	if err := os.WriteFile(full, []byte(body.Content), 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	full, ok := h.safePath(body.Path)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	var err error
	if body.IsDir {
		err = os.MkdirAll(full, 0755)
	} else {
		dir := filepath.Dir(full)
		os.MkdirAll(dir, 0755)
		err = os.WriteFile(full, []byte{}, 0644)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	reqPath := r.URL.Query().Get("path")
	full, ok := h.safePath(reqPath)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	if full == h.workspace() {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "cannot delete workspace root"})
		return
	}

	if err := os.RemoveAll(full); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Rename(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	oldFull, ok1 := h.safePath(body.OldPath)
	newFull, ok2 := h.safePath(body.NewPath)
	if !ok1 || !ok2 {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	dir := filepath.Dir(newFull)
	os.MkdirAll(dir, 0755)

	if err := os.Rename(oldFull, newFull); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
