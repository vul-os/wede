package git

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	Path       string `json:"path"`
	Status     string `json:"status"`
	Staged     bool   `json:"staged"`
	Conflicted bool   `json:"conflicted,omitempty"`
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

			// Conflict check: any line where either column is 'U', or both are the same non-space (AA, DD)
			isConflict := x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D')
			if isConflict {
				files = append(files, StatusFile{Path: path, Status: "conflict", Staged: false, Conflicted: true})
				continue
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

// validRemoteNameStrict matches only safe characters for git remote names.
// Must start with an alphanumeric character to prevent flag injection via "-" prefix.
var validRemoteNameStrict = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

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

// DeleteBranch deletes a local branch (git branch -d/-D).
// POST /api/git/branch/delete  {"name":"feature","force":false}
func (h *Handler) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Force bool   `json:"force"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if !validBranchName(body.Name) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid branch name"})
		return
	}

	flag := "-d" // safe delete (refuses if unmerged)
	if body.Force {
		flag = "-D" // force delete
	}
	out, err := h.run("branch", flag, "--", body.Name)
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

// Discard restores a working-tree file to its HEAD state using git restore.
// For untracked files, the command will fail and an appropriate error is returned.
// POST /api/git/discard  {"path": "src/foo.go"}
func (h *Handler) Discard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Reject flag-like paths to prevent injection (e.g. "--source=evil").
	if body.Path == "" || strings.HasPrefix(body.Path, "-") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid path"})
		return
	}

	out, err := h.run("restore", "--", body.Path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// StashEntry represents a single stash entry.
type StashEntry struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

// StashList lists all stash entries.
// GET /api/git/stash
func (h *Handler) StashList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	out, _ := h.run("stash", "list", "--format=%gd|%s|%cr")
	entries := []StashEntry{}
	if out != "" {
		for i, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "|", 3)
			if len(parts) < 3 {
				continue
			}
			entries = append(entries, StashEntry{
				Index:   i,
				Message: parts[1],
				Date:    parts[2],
			})
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"stashes": entries})
}

// StashPush creates a new stash entry.
// POST /api/git/stash  {"message": "optional message"}
func (h *Handler) StashPush(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	args := []string{"stash", "push"}
	if body.Message != "" {
		args = append(args, "-m", body.Message)
	}

	out, err := h.run(args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// validStashIndex parses and validates a stash index: must be a non-negative integer.
func validStashIndex(v int) bool {
	return v >= 0
}

// StashPop applies and removes a stash entry by index.
// POST /api/git/stash/pop  {"index": 0}
func (h *Handler) StashPop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Index *int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Index == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "index required"})
		return
	}
	if !validStashIndex(*body.Index) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid stash index"})
		return
	}

	ref := "stash@{" + strconv.Itoa(*body.Index) + "}"
	out, err := h.run("stash", "pop", ref)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// StashDrop removes a stash entry by index without applying it.
// POST /api/git/stash/drop  {"index": 0}
func (h *Handler) StashDrop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}
	var body struct {
		Index *int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Index == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "index required"})
		return
	}
	if !validStashIndex(*body.Index) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid stash index"})
		return
	}

	ref := "stash@{" + strconv.Itoa(*body.Index) + "}"
	out, err := h.run("stash", "drop", ref)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
}

// validCommitHash checks that a commit hash is safe to pass to git:
// 4–64 lowercase hex characters only.
var validCommitHash = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

// CommitDiff returns the stat and full diff for a specific commit.
// GET /api/git/commit-diff?hash=<hash>
func (h *Handler) CommitDiff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	hash := r.URL.Query().Get("hash")
	if !validCommitHash.MatchString(hash) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid commit hash"})
		return
	}

	stat, _ := h.run("show", "--stat", "--format=", hash)
	diff, _ := h.run("show", hash)

	// Parse file names from the stat output.
	// Lines look like: " src/foo.go | 3 +++"
	// The last summary line ("N files changed…") is skipped (no "|").
	files := []string{}
	for _, line := range strings.Split(stat, "\n") {
		if idx := strings.Index(line, " | "); idx >= 0 {
			name := strings.TrimSpace(line[:idx])
			if name != "" {
				files = append(files, name)
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"stat":  stat,
		"diff":  diff,
		"files": files,
	})
}

// ConflictRegion represents a merge conflict region in a file.
type ConflictRegion struct {
	Index         int      `json:"index"`
	CurrentLines  []string `json:"currentLines"`
	IncomingLines []string `json:"incomingLines"`
	StartLine     int      `json:"startLine"`
	EndLine       int      `json:"endLine"`
}

// ConflictRegions parses merge conflict markers from a file.
// GET /api/git/conflict?file=<path>
func (h *Handler) ConflictRegions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	reqPath := r.URL.Query().Get("file")
	if reqPath == "" || strings.HasPrefix(reqPath, "-") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid path"})
		return
	}

	full := filepath.Join(h.ws.Current(), reqPath)
	wsWithSep := h.ws.Current() + string(filepath.Separator)
	if full != h.ws.Current() && !strings.HasPrefix(full, wsWithSep) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	data, err := os.ReadFile(full)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "file not found"})
		return
	}

	regions := parseConflictRegions(string(data))
	json.NewEncoder(w).Encode(map[string]any{"regions": regions})
}

// parseConflictRegions finds all <<<<<<< ... ======= ... >>>>>>> blocks in text.
func parseConflictRegions(text string) []ConflictRegion {
	lines := strings.Split(text, "\n")
	var regions []ConflictRegion
	idx := 0
	i := 0
	for i < len(lines) {
		if strings.HasPrefix(lines[i], "<<<<<<<") {
			start := i + 1 // 1-based
			var current []string
			var incoming []string
			j := i + 1
			inIncoming := false
			for j < len(lines) {
				if strings.HasPrefix(lines[j], "=======") {
					inIncoming = true
					j++
					continue
				}
				if strings.HasPrefix(lines[j], ">>>>>>>") {
					break
				}
				if inIncoming {
					incoming = append(incoming, lines[j])
				} else {
					current = append(current, lines[j])
				}
				j++
			}
			end := j + 1 // 1-based line of >>>>>>>
			if current == nil {
				current = []string{}
			}
			if incoming == nil {
				incoming = []string{}
			}
			regions = append(regions, ConflictRegion{
				Index:         idx,
				CurrentLines:  current,
				IncomingLines: incoming,
				StartLine:     start,
				EndLine:       end,
			})
			idx++
			i = j + 1
			continue
		}
		i++
	}
	return regions
}

// ConflictResolve applies resolutions to conflict markers and stages the file.
// POST /api/git/conflict/resolve  {"path":"...","resolutions":[{"index":0,"choice":"current"|"incoming"|"both"}]}
func (h *Handler) ConflictResolve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Path        string `json:"path"`
		Resolutions []struct {
			Index  int    `json:"index"`
			Choice string `json:"choice"`
		} `json:"resolutions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	if body.Path == "" || strings.HasPrefix(body.Path, "-") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid path"})
		return
	}

	full := filepath.Join(h.ws.Current(), body.Path)
	wsWithSep := h.ws.Current() + string(filepath.Separator)
	if full != h.ws.Current() && !strings.HasPrefix(full, wsWithSep) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "path outside workspace"})
		return
	}

	data, err := os.ReadFile(full)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "file not found"})
		return
	}

	// Build a resolution map by index.
	choices := map[int]string{}
	for _, res := range body.Resolutions {
		choices[res.Index] = res.Choice
	}

	resolved := applyConflictResolutions(string(data), choices)

	if err := os.WriteFile(full, []byte(resolved), 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	out, err := h.run("add", "--", body.Path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// applyConflictResolutions rewrites text by resolving each conflict region.
func applyConflictResolutions(text string, choices map[int]string) string {
	lines := strings.Split(text, "\n")
	var out []string
	idx := 0
	i := 0
	for i < len(lines) {
		if strings.HasPrefix(lines[i], "<<<<<<<") {
			choice := choices[idx]
			var current []string
			var incoming []string
			j := i + 1
			inIncoming := false
			for j < len(lines) {
				if strings.HasPrefix(lines[j], "=======") {
					inIncoming = true
					j++
					continue
				}
				if strings.HasPrefix(lines[j], ">>>>>>>") {
					break
				}
				if inIncoming {
					incoming = append(incoming, lines[j])
				} else {
					current = append(current, lines[j])
				}
				j++
			}
			switch choice {
			case "incoming":
				out = append(out, incoming...)
			case "both":
				out = append(out, current...)
				out = append(out, incoming...)
			default: // "current" or unspecified
				out = append(out, current...)
			}
			idx++
			i = j + 1
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return strings.Join(out, "\n")
}

// RemoteAdd adds a new git remote.
// POST /api/git/remotes/add  {"name":"origin","url":"https://..."}
func (h *Handler) RemoteAdd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if !validRemoteNameStrict.MatchString(body.Name) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid remote name"})
		return
	}
	if body.URL == "" || strings.HasPrefix(body.URL, "-") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid remote url"})
		return
	}

	out, err := h.run("remote", "add", "--", body.Name, body.URL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RemoteRemove removes a git remote by name.
// POST /api/git/remotes/remove  {"name":"origin"}
func (h *Handler) RemoteRemove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if !validRemoteNameStrict.MatchString(body.Name) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid remote name"})
		return
	}

	out, err := h.run("remote", "remove", "--", body.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// StageHunk applies a patch to the index (staging area) via git apply --cached.
// POST /api/git/stage-hunk  {"patch":"...unified diff..."}
func (h *Handler) StageHunk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Patch   string `json:"patch"`
		Reverse bool   `json:"reverse"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	if body.Patch == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "patch is required"})
		return
	}

	// Basic sanity: reject patches with null bytes.
	if strings.ContainsRune(body.Patch, 0) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid patch"})
		return
	}

	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	if body.Reverse {
		args = append(args, "--reverse")
	}

	dir := h.ws.Current()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewBufferString(body.Patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": strings.TrimSpace(string(out))})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
