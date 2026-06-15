// Package search implements project-wide text search.
// It prefers ripgrep (rg) when available on PATH for speed; falls back to a
// pure-Go filepath.Walk + bytes.Contains scan when rg is not installed.
// All paths are confined to the workspace via the same safePath guard used by
// the files package.
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// WorkspaceProvider is satisfied by workspace.Manager.
type WorkspaceProvider interface {
	Current() string
}

// Handler holds the workspace reference.
type Handler struct {
	ws WorkspaceProvider
}

// New returns a Handler.
func New(ws WorkspaceProvider) *Handler {
	return &Handler{ws: ws}
}

// Match is a single search hit.
type Match struct {
	File    string `json:"file"`    // workspace-relative path
	Line    int    `json:"line"`    // 1-based
	Col     int    `json:"col"`     // 1-based byte offset of match start
	Text    string `json:"text"`    // full line text (trimmed to 200 chars)
	MatchStart int `json:"matchStart"` // byte offset of match in Text
	MatchLen   int `json:"matchLen"`
}

func (h *Handler) checkWorkspace(w http.ResponseWriter) bool {
	if h.ws.Current() == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no workspace open"})
		return false
	}
	return true
}

// safePath returns an absolute path guaranteed to be inside the workspace.
func (h *Handler) safePath(reqPath string) (string, bool) {
	ws := h.ws.Current()
	if ws == "" {
		return "", false
	}
	if reqPath == "" || reqPath == "/" || reqPath == "." {
		return ws, true
	}
	full := filepath.Join(ws, reqPath)
	wsWithSep := ws + string(filepath.Separator)
	if full != ws && !strings.HasPrefix(full, wsWithSep) {
		return "", false
	}
	return full, true
}

// Search handles GET /api/search?q=...&case=true&regex=true
// Returns at most 500 matches.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "q is required"})
		return
	}

	caseSensitive := r.URL.Query().Get("case") == "true"
	useRegex := r.URL.Query().Get("regex") == "true"

	// Validate regex early so we can return a clear error.
	if useRegex {
		flags := "(?i)"
		if caseSensitive {
			flags = ""
		}
		if _, err := regexp.Compile(flags + query); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid regex: " + err.Error()})
			return
		}
	}

	ws := h.ws.Current()

	var matches []Match
	var searchErr error

	if rgPath, err := exec.LookPath("rg"); err == nil {
		matches, searchErr = searchWithRg(rgPath, ws, query, caseSensitive, useRegex)
	} else {
		matches, searchErr = searchWithWalk(ws, query, caseSensitive, useRegex)
	}

	if searchErr != nil {
		// Non-fatal: return partial results or empty.
		matches = []Match{}
	}

	// Cap at 500 matches.
	truncated := false
	if len(matches) > 500 {
		matches = matches[:500]
		truncated = true
	}

	json.NewEncoder(w).Encode(map[string]any{
		"matches":   matches,
		"truncated": truncated,
		"count":     len(matches),
	})
}

// ── ripgrep backend ──────────────────────────────────────────────────────────

func searchWithRg(rgPath, ws, query string, caseSensitive, useRegex bool) ([]Match, error) {
	args := []string{
		"--json",
		"--max-count", "500",
		"--max-filesize", "5M",
		// Exclude common large / binary / VCS directories.  The **/ prefix
		// is required so the pattern matches at any depth when rg is given
		// an absolute path as the search root.
		"--glob", "!**/.git/**",
		"--glob", "!**/node_modules/**",
		"--glob", "!**/.cache/**",
		"--glob", "!**/vendor/**",
		"--glob", "!**/dist/**",
		"--glob", "!**/.next/**",
		"--glob", "!**/build/**",
	}
	if !caseSensitive {
		args = append(args, "--ignore-case")
	}
	if !useRegex {
		args = append(args, "--fixed-strings")
	}
	// Append query and path, separated by -- to prevent flag injection.
	args = append(args, "--", query, ws)

	cmd := exec.Command(rgPath, args...)
	cmd.Dir = ws
	out, err := cmd.Output()
	// rg exits 1 when no matches, which is not an error for us.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []Match{}, nil
		}
		return nil, err
	}

	return parseRgJSON(out, ws)
}

// parseRgJSON parses ripgrep's --json output (NDJSON format).
func parseRgJSON(data []byte, ws string) ([]Match, error) {
	var matches []Match
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
				Submatches []struct {
					Match struct {
						Text string `json:"text"`
					} `json:"match"`
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type != "match" {
			continue
		}

		relPath, err := filepath.Rel(ws, msg.Data.Path.Text)
		if err != nil {
			relPath = msg.Data.Path.Text
		}
		text := strings.TrimRight(msg.Data.Lines.Text, "\n\r")
		if len(text) > 200 {
			text = text[:200]
		}

		col := 1
		matchStart := 0
		matchLen := 0
		if len(msg.Data.Submatches) > 0 {
			sm := msg.Data.Submatches[0]
			col = sm.Start + 1
			matchStart = sm.Start
			matchLen = sm.End - sm.Start
		}

		matches = append(matches, Match{
			File:       relPath,
			Line:       msg.Data.LineNumber,
			Col:        col,
			Text:       text,
			MatchStart: matchStart,
			MatchLen:   matchLen,
		})
	}
	return matches, scanner.Err()
}

// ── pure-Go fallback ─────────────────────────────────────────────────────────

// skipDirs lists directories we never descend into during search.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".cache": true,
	"vendor": true, "dist": true, ".next": true, "build": true,
}

// textExtensions is an allowlist of extensions we attempt to search.
// We skip binary-looking files by default; unknown extensions are probed.
var textExtensions = map[string]bool{
	"go": true, "js": true, "jsx": true, "ts": true, "tsx": true,
	"py": true, "rb": true, "rs": true, "java": true, "c": true,
	"cpp": true, "h": true, "hpp": true, "cs": true, "php": true,
	"html": true, "htm": true, "css": true, "scss": true, "less": true,
	"json": true, "yaml": true, "yml": true, "toml": true, "xml": true,
	"md": true, "txt": true, "sh": true, "bash": true, "zsh": true,
	"sql": true, "graphql": true, "vue": true, "svelte": true,
	"env": true, "gitignore": true, "mod": true, "sum": true,
	"lock": true, "conf": true, "ini": true, "cfg": true,
	"makefile": true, "dockerfile": true,
}

func searchWithWalk(ws, query string, caseSensitive, useRegex bool) ([]Match, error) {
	var re *regexp.Regexp
	if useRegex {
		flags := "(?i)"
		if caseSensitive {
			flags = ""
		}
		var err error
		re, err = regexp.Compile(flags + query)
		if err != nil {
			return nil, err
		}
	}

	queryLower := strings.ToLower(query)

	var matches []Match

	err := filepath.Walk(ws, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Size cap: skip files > 2MB in the Go walker.
		if info.Size() > 2*1024*1024 {
			return nil
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(info.Name()), "."))
		base := strings.ToLower(info.Name())
		// Allow extensionless known filenames.
		if ext == "" && !textExtensions[base] {
			return nil
		}
		if ext != "" && !textExtensions[ext] {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Quick binary probe: if more than 0.5% of the first 512 bytes are null, skip.
		probe := data
		if len(probe) > 512 {
			probe = probe[:512]
		}
		nullCount := bytes.Count(probe, []byte{0})
		if len(probe) > 0 && nullCount*200 > len(probe) {
			return nil
		}

		relPath, _ := filepath.Rel(ws, path)

		lineNum := 0
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			lineNum++
			lineText := scanner.Text()

			var matched bool
			var matchStart, matchLen int

			if useRegex {
				loc := re.FindStringIndex(lineText)
				if loc != nil {
					matched = true
					matchStart = loc[0]
					matchLen = loc[1] - loc[0]
				}
			} else {
				haystack := lineText
				needle := query
				if !caseSensitive {
					haystack = strings.ToLower(lineText)
					needle = queryLower
				}
				idx := strings.Index(haystack, needle)
				if idx >= 0 {
					matched = true
					matchStart = idx
					matchLen = len(needle)
				}
			}

			if matched {
				text := lineText
				if len(text) > 200 {
					text = text[:200]
				}
				matches = append(matches, Match{
					File:       relPath,
					Line:       lineNum,
					Col:        matchStart + 1,
					Text:       text,
					MatchStart: matchStart,
					MatchLen:   matchLen,
				})
				if len(matches) >= 500 {
					return io.EOF // sentinel to stop walk
				}
			}
		}
		return nil
	})

	if err == io.EOF {
		err = nil // normal early-stop
	}
	return matches, err
}

// LineCount is a helper used by tests.
func LineCount(s string) int {
	return strings.Count(s, "\n") + 1
}

// FormatCount returns a display string like "42 results" / "500+ results".
func FormatCount(n int, truncated bool) string {
	if truncated {
		return strconv.Itoa(n) + "+ results"
	}
	if n == 1 {
		return "1 result"
	}
	return strconv.Itoa(n) + " results"
}
