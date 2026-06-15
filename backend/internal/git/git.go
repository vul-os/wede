package git

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
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

func (h *Handler) run(args ...string) (string, error) {
	dir := h.ws.Current()
	if dir == "" {
		return "", nil
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (h *Handler) checkWorkspace(w http.ResponseWriter) bool {
	if h.ws.Current() == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no workspace open"})
		return false
	}
	return true
}

type StatusFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Staged bool   `json:"staged"`
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	out, err := h.run("status", "--porcelain")
	if err != nil {
		// Not a git repo - that's fine, just return empty
		json.NewEncoder(w).Encode(map[string]any{"branch": "", "files": []any{}, "isRepo": false})
		return
	}

	branch, _ := h.run("branch", "--show-current")

	files := []StatusFile{}
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			if len(line) < 4 {
				continue
			}
			x := line[0] // index (staged) status
			y := line[1] // working tree (unstaged) status
			path := strings.TrimSpace(line[3:])

			// Handle renames: "R  old -> new"
			if idx := strings.Index(path, " -> "); idx >= 0 {
				path = path[idx+4:]
			}

			// Untracked files
			if x == '?' && y == '?' {
				files = append(files, StatusFile{Path: path, Status: "untracked", Staged: false})
				continue
			}

			// Staged change (index column)
			if x != ' ' && x != '?' {
				status := "modified"
				switch x {
				case 'A':
					status = "added"
				case 'D':
					status = "deleted"
				case 'R':
					status = "renamed"
				case 'M':
					status = "modified"
				case 'C':
					status = "copied"
				}
				files = append(files, StatusFile{Path: path, Status: status, Staged: true})
			}

			// Unstaged change (working tree column)
			if y != ' ' && y != '?' {
				status := "modified"
				switch y {
				case 'D':
					status = "deleted"
				case 'M':
					status = "modified"
				}
				files = append(files, StatusFile{Path: path, Status: status, Staged: false})
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"branch": branch,
		"files":  files,
		"isRepo": true,
	})
}

type LogEntry struct {
	Hash    string   `json:"hash"`
	Short   string   `json:"short"`
	Message string   `json:"message"`
	Author  string   `json:"author"`
	Date    string   `json:"date"`
	Refs    string   `json:"refs,omitempty"`
	Parents []string `json:"parents"`
}

func (h *Handler) Log(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	count := r.URL.Query().Get("count")
	if count == "" {
		count = "50"
	}
	// Validate count is a positive integer to prevent flag injection via -n.
	// exec.Command is not a shell so "50; rm -rf /" won't execute, but a value
	// like "--all" would be misinterpreted by git as a flag argument.
	if n, err := strconv.Atoi(count); err != nil || n < 1 || n > 10000 {
		count = "50"
	}

	out, err := h.run("log", "--format=%H|%h|%s|%an|%ar|%D|%P", "-n", count, "--all")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"entries": []any{}})
		return
	}

	entries := []LogEntry{}
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "|", 7)
			if len(parts) < 7 {
				continue
			}
			parents := []string{}
			if parts[6] != "" {
				parents = strings.Split(parts[6], " ")
			}
			entries = append(entries, LogEntry{
				Hash:    parts[0],
				Short:   parts[1],
				Message: parts[2],
				Author:  parts[3],
				Date:    parts[4],
				Refs:    parts[5],
				Parents: parents,
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]any{"entries": entries})
}

func (h *Handler) Diff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	file := r.URL.Query().Get("file")
	staged := r.URL.Query().Get("staged") == "true"

	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}
	if file != "" {
		args = append(args, "--", file)
	}

	out, _ := h.run(args...)
	json.NewEncoder(w).Encode(map[string]string{"diff": out})
}

func (h *Handler) Stage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	path := body.Path
	if path == "" {
		path = "."
	}

	// Use "--" to prevent a path starting with "-" from being treated as a flag.
	out, err := h.run("add", "--", path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Unstage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	path := body.Path
	if path == "" {
		path = "."
	}

	out, err := h.run("reset", "HEAD", "--", path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Commit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "message required"})
		return
	}

	out, err := h.run("commit", "-m", body.Message)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

func (h *Handler) Branches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	out, _ := h.run("branch", "-a", "--format=%(refname:short)|%(HEAD)")
	current, _ := h.run("branch", "--show-current")

	branches := []map[string]any{}
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "|", 2)
			name := strings.TrimSpace(parts[0])
			if name == "" {
				continue
			}
			branches = append(branches, map[string]any{
				"name":    name,
				"current": name == current,
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]any{"branches": branches, "current": current})
}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Branch string `json:"branch"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Reject branch names that look like flags (start with "-").  This prevents
	// injecting options like "--detach", "--orphan=evil", or "-b" into git checkout.
	// Legitimate branch/ref names never begin with a hyphen (git itself enforces this).
	if body.Branch == "" || strings.HasPrefix(body.Branch, "-") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid branch name"})
		return
	}

	out, err := h.run("checkout", body.Branch)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// validBranchName returns true when name is safe to pass as a git branch
// argument — non-empty and not starting with "-" (which would look like a flag).
func validBranchName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "-")
}

// validRemoteName returns true when name is safe to pass as a git remote name.
func validRemoteName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "-")
}

// CreateBranch creates a new local branch (and optionally checks it out).
// POST /api/git/branch  {"name":"feat/foo","checkout":true}
func (h *Handler) CreateBranch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Name     string `json:"name"`
		Checkout bool   `json:"checkout"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if !validBranchName(body.Name) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid branch name"})
		return
	}

	var out string
	var err error
	if body.Checkout {
		// "git checkout -b <name>" — no "--" separator needed here since -b
		// takes the next argument as the branch name (not a pathspec).
		out, err = h.run("checkout", "-b", body.Name)
	} else {
		out, err = h.run("branch", "--", body.Name)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// Fetch runs git fetch [remote].
// POST /api/git/fetch  {"remote":"origin"}  (remote is optional)
func (h *Handler) Fetch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Remote string `json:"remote"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	args := []string{"fetch", "--prune"}
	if body.Remote != "" {
		if !validRemoteName(body.Remote) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid remote name"})
			return
		}
		args = append(args, "--", body.Remote)
	}

	out, err := h.run(args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// Pull runs git pull [remote [branch]].
// POST /api/git/pull  {"remote":"origin","branch":"main"}
func (h *Handler) Pull(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Remote string `json:"remote"`
		Branch string `json:"branch"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	args := []string{"pull"}
	if body.Remote != "" {
		if !validRemoteName(body.Remote) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid remote name"})
			return
		}
		args = append(args, "--", body.Remote)
		if body.Branch != "" {
			if !validBranchName(body.Branch) {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid branch name"})
				return
			}
			args = append(args, body.Branch)
		}
	}

	out, err := h.run(args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// Push runs git push [remote [branch]].
// POST /api/git/push  {"remote":"origin","branch":"main","setUpstream":true}
func (h *Handler) Push(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Remote      string `json:"remote"`
		Branch      string `json:"branch"`
		SetUpstream bool   `json:"setUpstream"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	args := []string{"push"}
	if body.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if body.Remote != "" {
		if !validRemoteName(body.Remote) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid remote name"})
			return
		}
		args = append(args, "--", body.Remote)
		if body.Branch != "" {
			if !validBranchName(body.Branch) {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid branch name"})
				return
			}
			args = append(args, body.Branch)
		}
	}

	out, err := h.run(args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// Remotes returns the list of configured git remotes.
// GET /api/git/remotes
func (h *Handler) Remotes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	out, _ := h.run("remote", "-v")
	// Parse "origin\thttps://... (fetch)" lines into deduplicated names.
	seen := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		url := parts[1]
		if _, ok := seen[name]; !ok {
			seen[name] = url
		}
	}
	remotes := []map[string]string{}
	for name, url := range seen {
		remotes = append(remotes, map[string]string{"name": name, "url": url})
	}
	json.NewEncoder(w).Encode(map[string]any{"remotes": remotes})
}
