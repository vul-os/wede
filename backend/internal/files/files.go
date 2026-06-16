package files

import (
	"encoding/base64"
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

	// Determine file type by extension.
	ext := strings.ToLower(filepath.Ext(info.Name()))
	imageMimes := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".webp": "image/webp",
	}

	if mime, isImage := imageMimes[ext]; isImage {
		data, err := os.ReadFile(full)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		dataURL := "data:" + mime + ";base64," + encoded
		json.NewEncoder(w).Encode(map[string]any{
			"path":     reqPath,
			"content":  "",
			"fileType": "image",
			"dataUrl":  dataURL,
		})
		return
	}

	data, err := os.ReadFile(full)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Binary probe: if > 0.5% of first 512 bytes are null, treat as binary.
	probe := data
	if len(probe) > 512 {
		probe = probe[:512]
	}
	nullCount := 0
	for _, b := range probe {
		if b == 0 {
			nullCount++
		}
	}
	if len(probe) > 0 && nullCount*200 > len(probe) {
		json.NewEncoder(w).Encode(map[string]any{
			"path":     reqPath,
			"content":  "",
			"fileType": "binary",
			"size":     info.Size(),
		})
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

// Copy recursively copies src to dst (both are workspace-relative paths).
// For files, it copies the file contents. For directories it walks the tree
// and recreates the structure under dst.
func (h *Handler) Copy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	srcFull, ok1 := h.safePath(body.Src)
	dstFull, ok2 := h.safePath(body.Dst)
	if !ok1 || !ok2 {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	srcInfo, err := os.Stat(srcFull)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "source not found"})
		return
	}

	if err := copyRecursive(srcFull, dstFull, srcInfo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// copyRecursive copies src to dst. srcInfo is the result of os.Stat(src).
func copyRecursive(src, dst string, srcInfo os.FileInfo) error {
	if srcInfo.IsDir() {
		if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childSrc := filepath.Join(src, e.Name())
			childDst := filepath.Join(dst, e.Name())
			info, err := e.Info()
			if err != nil {
				return err
			}
			if err := copyRecursive(childSrc, childDst, info); err != nil {
				return err
			}
		}
		return nil
	}
	// Regular file
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, srcInfo.Mode())
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
