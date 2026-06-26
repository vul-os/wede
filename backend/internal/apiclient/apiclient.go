// Package apiclient implements wede's built-in HTTP API client (a Postman-style
// "request runner"). Requests and folders are persisted as plain JSON files under
// <wede>/requests/ so they're committable and shareable like the rest of .wede;
// environments live under <wede>/environments/. The send proxy executes requests
// server-side, so there are no browser CORS limits.
//
// Variable ({{name}}) substitution happens client-side before /send, so the proxy
// stays a dumb forwarder.
package apiclient

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxResponseBytes caps how much of a response body we buffer (10 MiB).
const maxResponseBytes = 10 << 20

// Handler serves the API client for one workspace. wedeDir resolves the
// workspace's current .wede directory (which may be relocated at runtime).
type Handler struct {
	wedeDir func() string
}

func New(wedeDir func() string) *Handler { return &Handler{wedeDir: wedeDir} }

func (h *Handler) requestsDir() string    { return filepath.Join(h.wedeDir(), "requests") }
func (h *Handler) environmentsDir() string { return filepath.Join(h.wedeDir(), "environments") }

// safeJoin confines a caller-supplied relative path within base.
func safeJoin(base, rel string) (string, bool) {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	clean := filepath.Clean(filepath.Join(base, rel))
	baseClean := filepath.Clean(base)
	if clean != baseClean && !strings.HasPrefix(clean, baseClean+string(os.PathSeparator)) {
		return "", false
	}
	return clean, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ── Send proxy ────────────────────────────────────────────────────────────────

type sendRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type sendResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	TimeMs     int64             `json:"timeMs"`
	Size       int               `json:"size"`
}

// Send executes the given HTTP request server-side and returns the response.
// Editor/owner only (mounted behind RequireEditor) — it can reach any host the
// server can, which is the point of an API client on a self-hosted dev tool.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if !strings.Contains(req.URL, "://") {
		req.URL = "http://" + req.URL
	}

	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}
	outReq, err := http.NewRequestWithContext(r.Context(), strings.ToUpper(req.Method), req.URL, bodyReader)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid URL or method: "+err.Error())
		return
	}
	for k, v := range req.Headers {
		if k != "" {
			outReq.Header.Set(k, v)
		}
	}

	client := &http.Client{Timeout: 60 * time.Second}
	start := time.Now()
	resp, err := client.Do(outReq)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error(), "timeMs": elapsed})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	headers := map[string]string{}
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}
	writeJSON(w, http.StatusOK, sendResponse{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Headers:    headers,
		Body:       string(body),
		TimeMs:     elapsed,
		Size:       len(body),
	})
}

// ── Collection tree (folders + request files) ─────────────────────────────────

type node struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"` // relative to requests/
	Type     string          `json:"type"` // "folder" | "request"
	Children []*node         `json:"children,omitempty"`
	Request  json.RawMessage `json:"request,omitempty"` // request body for type=="request"
}

func (h *Handler) buildTree(absDir, relDir string) []*node {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return []*node{}
	}
	out := []*node{}
	for _, e := range entries {
		rel := filepath.ToSlash(filepath.Join(relDir, e.Name()))
		if e.IsDir() {
			out = append(out, &node{
				Name:     e.Name(),
				Path:     rel,
				Type:     "folder",
				Children: h.buildTree(filepath.Join(absDir, e.Name()), rel),
			})
		} else if strings.HasSuffix(e.Name(), ".json") {
			data, _ := os.ReadFile(filepath.Join(absDir, e.Name()))
			out = append(out, &node{
				Name:    strings.TrimSuffix(e.Name(), ".json"),
				Path:    rel,
				Type:    "request",
				Request: json.RawMessage(data),
			})
		}
	}
	// folders first, then alphabetical
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "folder"
		}
		return out[i].Name < out[j].Name
	})
	return out
}

type environment struct {
	Name      string            `json:"name"`
	Variables map[string]string `json:"variables"`
}

func (h *Handler) loadEnvironments() []environment {
	out := []environment{}
	entries, err := os.ReadDir(h.environmentsDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.environmentsDir(), e.Name()))
		if err != nil {
			continue
		}
		var env environment
		if json.Unmarshal(data, &env) == nil {
			if env.Name == "" {
				env.Name = strings.TrimSuffix(e.Name(), ".json")
			}
			out = append(out, env)
		}
	}
	return out
}

// Tree returns the full collection tree + environments (read-only; safe for viewers).
func (h *Handler) Tree(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"tree":         h.buildTree(h.requestsDir(), ""),
		"environments": h.loadEnvironments(),
	})
}

// ── Mutations (editor/owner) ──────────────────────────────────────────────────

// SaveItem creates a folder or writes a request file. Body: {type, path, request?}.
func (h *Handler) SaveItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type    string          `json:"type"`
		Path    string          `json:"path"`
		Request json.RawMessage `json:"request"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Type == "folder" {
		abs, ok := safeJoin(h.requestsDir(), body.Path)
		if !ok {
			writeErr(w, http.StatusBadRequest, "invalid path")
			return
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	// request: path is relative without extension; store as <path>.json
	abs, ok := safeJoin(h.requestsDir(), body.Path+".json")
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pretty, _ := json.MarshalIndent(json.RawMessage(body.Request), "", "  ")
	if err := os.WriteFile(abs, pretty, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteItem removes a request file or a folder (recursively). ?path=&type=.
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if r.URL.Query().Get("type") == "request" {
		rel += ".json"
	}
	abs, ok := safeJoin(h.requestsDir(), rel)
	if !ok || abs == filepath.Clean(h.requestsDir()) {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := os.RemoveAll(abs); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SaveEnvironment writes an environment file. Body: {name, variables}.
func (h *Handler) SaveEnvironment(w http.ResponseWriter, r *http.Request) {
	var env environment
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil || strings.TrimSpace(env.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	abs, ok := safeJoin(h.environmentsDir(), env.Name+".json")
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	if err := os.MkdirAll(h.environmentsDir(), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	data, _ := json.MarshalIndent(env, "", "  ")
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteEnvironment removes an environment file. ?name=.
func (h *Handler) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	abs, ok := safeJoin(h.environmentsDir(), name+".json")
	if !ok || name == "" {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	_ = os.Remove(abs)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
