// Package search implements project-wide text search and filename search.
// It prefers ripgrep (rg) when available on PATH for speed; falls back to a
// pure-Go filepath.Walk + regexp scan when rg is not installed.
// All paths are confined to the workspace via the same safePath guard used by
// the files package.
//
// NEW ROUTES NEEDED (wire in backend/cmd/wede/main.go):
//
//	GET  /api/search/files   →  (*Handler).SearchFiles
//	     Query params: q (required), case, regex, include, exclude
//	     Returns: {"files":[{"path":"..."},...], "count":N, "truncated":bool}
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
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
	File       string   `json:"file"`       // workspace-relative path
	Line       int      `json:"line"`       // 1-based
	Col        int      `json:"col"`        // 1-based byte offset of match start
	Text       string   `json:"text"`       // matched line text (trimmed to 200 chars)
	MatchStart int      `json:"matchStart"` // byte offset of match in Text
	MatchLen   int      `json:"matchLen"`
	Before     []string `json:"before,omitempty"` // context lines before the match
	After      []string `json:"after,omitempty"`  // context lines after the match
}

// FileMatch is a single filename search hit.
type FileMatch struct {
	Path string `json:"path"` // workspace-relative path (forward slashes)
}

// searchOpts collects all options shared across search backends.
type searchOpts struct {
	query         string
	caseSensitive bool
	wholeWord     bool
	useRegex      bool
	includeGlob   string
	excludeGlob   string
	contextLines  int
	maxMatches    int
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

// compilePattern returns a compiled regexp for the given options.
// For non-regex queries the pattern is escaped with QuoteMeta.
// Whole-word wraps with \b...\b. Case-insensitivity is applied via (?i).
func compilePattern(query string, caseSensitive, wholeWord, useRegex bool) (*regexp.Regexp, error) {
	pat := query
	if !useRegex {
		pat = regexp.QuoteMeta(query)
	}
	if wholeWord {
		pat = `\b(?:` + pat + `)\b`
	}
	prefix := ""
	if !caseSensitive {
		prefix = "(?i)"
	}
	return regexp.Compile(prefix + pat)
}

// ── Glob helper ──────────────────────────────────────────────────────────────

// globMatch reports whether the workspace-relative path (forward slashes)
// matches the glob pattern using VSCode/ripgrep conventions:
//   - No "/" in pattern → match against basename only.
//   - "/" in pattern, no "**" → path.Match against full relative path.
//   - "**" → strip leading "**/" and try basename, full path, and each suffix.
func globMatch(pattern, relPath string) bool {
	if pattern == "" {
		return false
	}
	pattern = filepath.ToSlash(pattern)
	relPath = filepath.ToSlash(relPath)
	base := path.Base(relPath)

	if !strings.Contains(pattern, "/") {
		ok, _ := path.Match(pattern, base)
		return ok
	}
	if !strings.Contains(pattern, "**") {
		ok, _ := path.Match(pattern, relPath)
		return ok
	}
	// Strip leading **/ prefix repeatedly.
	inner := pattern
	for strings.HasPrefix(inner, "**/") {
		inner = inner[3:]
	}
	if inner == "**" {
		return true
	}
	// Try against basename.
	if ok, _ := path.Match(inner, base); ok {
		return true
	}
	// Try against full path.
	if ok, _ := path.Match(inner, relPath); ok {
		return true
	}
	// Try each path suffix segment.
	parts := strings.Split(relPath, "/")
	for i := range parts {
		sub := strings.Join(parts[i:], "/")
		if ok, _ := path.Match(inner, sub); ok {
			return true
		}
	}
	return false
}

// ── Search handler ───────────────────────────────────────────────────────────

// Search handles GET /api/search
//
// Query params:
//
//	q        – required search query
//	case     – "true" for case-sensitive
//	word     – "true" for whole-word match
//	regex    – "true" to treat q as a regular expression
//	include  – glob; only search files matching this glob
//	exclude  – glob; skip files matching this glob
//	context  – number of surrounding context lines (0-5, default 2)
//
// Response: {"matches":[...],"count":N,"fileCount":N,"truncated":bool}
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "q is required"})
		return
	}

	opts := searchOpts{
		query:         q,
		caseSensitive: r.URL.Query().Get("case") == "true",
		wholeWord:     r.URL.Query().Get("word") == "true",
		useRegex:      r.URL.Query().Get("regex") == "true",
		includeGlob:   r.URL.Query().Get("include"),
		excludeGlob:   r.URL.Query().Get("exclude"),
		maxMatches:    500,
	}

	ctx, _ := strconv.Atoi(r.URL.Query().Get("context"))
	if ctx < 0 {
		ctx = 0
	}
	if ctx > 5 {
		ctx = 5
	}
	opts.contextLines = ctx

	// Validate regex / whole-word pattern early.
	if opts.useRegex || opts.wholeWord {
		if _, err := compilePattern(opts.query, opts.caseSensitive, opts.wholeWord, opts.useRegex); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid regex: " + err.Error()})
			return
		}
	}

	ws := h.ws.Current()
	var matches []Match
	var searchErr error

	if rgPath, err := exec.LookPath("rg"); err == nil {
		matches, searchErr = searchWithRg(rgPath, ws, opts)
	} else {
		matches, searchErr = searchWithWalk(ws, opts)
	}
	if searchErr != nil {
		matches = []Match{}
	}

	truncated := false
	if len(matches) > opts.maxMatches {
		matches = matches[:opts.maxMatches]
		truncated = true
	}

	// Count unique files.
	fileSet := map[string]struct{}{}
	for _, m := range matches {
		fileSet[m.File] = struct{}{}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"matches":   matches,
		"truncated": truncated,
		"count":     len(matches),
		"fileCount": len(fileSet),
	})
}

// ── SearchFiles handler ───────────────────────────────────────────────────────

// SearchFiles handles GET /api/search/files
//
// Finds files whose workspace-relative path contains (or matches) the query.
// This is a NEW ROUTE — wire it in backend/cmd/wede/main.go.
//
// Query params: q (required), case, regex, include, exclude
// Response: {"files":[{"path":"..."},...], "count":N, "truncated":bool}
func (h *Handler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "q is required"})
		return
	}

	caseSensitive := r.URL.Query().Get("case") == "true"
	useRegex := r.URL.Query().Get("regex") == "true"
	includeGlob := r.URL.Query().Get("include")
	excludeGlob := r.URL.Query().Get("exclude")

	// Validate regex early.
	if useRegex {
		flags := "(?i)"
		if caseSensitive {
			flags = ""
		}
		if _, err := regexp.Compile(flags + q); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid regex: " + err.Error()})
			return
		}
	}

	ws := h.ws.Current()
	files, err := searchFilePaths(ws, q, caseSensitive, useRegex, includeGlob, excludeGlob)
	if err != nil {
		files = []FileMatch{}
	}

	truncated := false
	if len(files) > 500 {
		files = files[:500]
		truncated = true
	}

	json.NewEncoder(w).Encode(map[string]any{
		"files":     files,
		"count":     len(files),
		"truncated": truncated,
	})
}

// searchFilePaths walks the workspace and returns files whose relative path
// contains (or matches via regex) the query.
func searchFilePaths(ws, query string, caseSensitive, useRegex bool, includeGlob, excludeGlob string) ([]FileMatch, error) {
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
	var results []FileMatch

	err := filepath.Walk(ws, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(ws, p)
		relPath = filepath.ToSlash(relPath)

		if includeGlob != "" && !globMatch(includeGlob, relPath) {
			return nil
		}
		if excludeGlob != "" && globMatch(excludeGlob, relPath) {
			return nil
		}

		var matched bool
		if useRegex {
			matched = re.MatchString(relPath)
		} else {
			hay := relPath
			need := query
			if !caseSensitive {
				hay = strings.ToLower(relPath)
				need = queryLower
			}
			matched = strings.Contains(hay, need)
		}

		if matched {
			results = append(results, FileMatch{Path: relPath})
			if len(results) >= 500 {
				return io.EOF
			}
		}
		return nil
	})

	if err == io.EOF {
		err = nil
	}
	return results, err
}

// ── ripgrep backend ──────────────────────────────────────────────────────────

func searchWithRg(rgPath, ws string, opts searchOpts) ([]Match, error) {
	args := []string{
		"--json",
		"--max-count", strconv.Itoa(opts.maxMatches),
		"--max-filesize", "5M",
		// Exclude common large / binary / VCS directories.
		"--glob", "!**/.git/**",
		"--glob", "!**/node_modules/**",
		"--glob", "!**/.cache/**",
		"--glob", "!**/vendor/**",
		"--glob", "!**/dist/**",
		"--glob", "!**/.next/**",
		"--glob", "!**/build/**",
	}

	if !opts.caseSensitive {
		args = append(args, "--ignore-case")
	}
	if !opts.useRegex {
		args = append(args, "--fixed-strings")
	}
	if opts.wholeWord {
		args = append(args, "--word-regexp")
	}
	if opts.includeGlob != "" {
		args = append(args, "--glob", opts.includeGlob)
	}
	if opts.excludeGlob != "" {
		args = append(args, "--glob", "!"+opts.excludeGlob)
	}
	if opts.contextLines > 0 {
		args = append(args, "-C", strconv.Itoa(opts.contextLines))
	}

	args = append(args, "--", opts.query, ws)

	cmd := exec.Command(rgPath, args...)
	cmd.Dir = ws
	out, err := cmd.Output()
	// rg exits 1 when no matches — not an error for us.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []Match{}, nil
		}
		return nil, err
	}

	return parseRgJSON(out, ws, opts.contextLines)
}

// parseRgJSON parses ripgrep's --json (NDJSON) output.
// When contextLines > 0 it also processes "context" messages and populates
// Before/After on each Match.
func parseRgJSON(data []byte, ws string, contextLines int) ([]Match, error) {
	type rgMsg struct {
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

	type pendingAfter struct {
		idx       int
		collected int
	}

	var matches []Match
	var beforeBuf []string
	var pending []pendingAfter

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Increase buffer for long lines.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rgMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "context":
			if contextLines == 0 {
				continue
			}
			lineText := strings.TrimRight(msg.Data.Lines.Text, "\n\r")

			// Feed context line into pending after-buffers.
			var stillPending []pendingAfter
			for _, p := range pending {
				if p.collected < contextLines {
					matches[p.idx].After = append(matches[p.idx].After, lineText)
					p.collected++
				}
				if p.collected < contextLines {
					stillPending = append(stillPending, p)
				}
			}
			pending = stillPending

			// Also accumulate for the next match's before-context.
			beforeBuf = append(beforeBuf, lineText)
			if len(beforeBuf) > contextLines {
				beforeBuf = beforeBuf[len(beforeBuf)-contextLines:]
			}

		case "match":
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

			var beforeCopy []string
			if len(beforeBuf) > 0 {
				beforeCopy = make([]string, len(beforeBuf))
				copy(beforeCopy, beforeBuf)
			}

			matches = append(matches, Match{
				File:       relPath,
				Line:       msg.Data.LineNumber,
				Col:        col,
				Text:       text,
				MatchStart: matchStart,
				MatchLen:   matchLen,
				Before:     beforeCopy,
			})

			if contextLines > 0 {
				pending = append(pending, pendingAfter{idx: len(matches) - 1, collected: 0})
			}
			// Reset before-buffer; context lines after this match will
			// re-populate it for the next match's before-context.
			beforeBuf = nil
		}
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

func searchWithWalk(ws string, opts searchOpts) ([]Match, error) {
	// Compile a regexp when needed: always for regex/wholeWord; never for plain
	// fixed-string (we use strings.Contains for speed in that branch).
	var re *regexp.Regexp
	if opts.useRegex || opts.wholeWord {
		var err error
		re, err = compilePattern(opts.query, opts.caseSensitive, opts.wholeWord, opts.useRegex)
		if err != nil {
			return nil, err
		}
	}

	queryLower := strings.ToLower(opts.query)
	var matches []Match

	err := filepath.Walk(ws, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2*1024*1024 {
			return nil
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(info.Name()), "."))
		base := strings.ToLower(info.Name())
		if ext == "" && !textExtensions[base] {
			return nil
		}
		if ext != "" && !textExtensions[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(ws, fpath)
		relSlash := filepath.ToSlash(relPath)

		// Glob filtering.
		if opts.includeGlob != "" && !globMatch(opts.includeGlob, relSlash) {
			return nil
		}
		if opts.excludeGlob != "" && globMatch(opts.excludeGlob, relSlash) {
			return nil
		}

		data, err := os.ReadFile(fpath)
		if err != nil {
			return nil
		}

		// Quick binary probe.
		probe := data
		if len(probe) > 512 {
			probe = probe[:512]
		}
		if len(probe) > 0 && bytes.Count(probe, []byte{0})*200 > len(probe) {
			return nil
		}

		// Split into lines for context support.
		lines := strings.Split(string(data), "\n")

		for i, lineText := range lines {
			lineNum := i + 1

			var matched bool
			var matchStart, matchLen int

			if re != nil {
				loc := re.FindStringIndex(lineText)
				if loc != nil {
					matched = true
					matchStart = loc[0]
					matchLen = loc[1] - loc[0]
				}
			} else {
				haystack := lineText
				needle := opts.query
				if !opts.caseSensitive {
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

			if !matched {
				continue
			}

			text := lineText
			if len(text) > 200 {
				text = text[:200]
			}

			m := Match{
				File:       relPath,
				Line:       lineNum,
				Col:        matchStart + 1,
				Text:       text,
				MatchStart: matchStart,
				MatchLen:   matchLen,
			}

			// Collect context lines.
			if opts.contextLines > 0 {
				start := max(0, i-opts.contextLines)
				for j := start; j < i; j++ {
					m.Before = append(m.Before, lines[j])
				}
				end := min(len(lines), i+opts.contextLines+1)
				for j := i + 1; j < end; j++ {
					m.After = append(m.After, lines[j])
				}
			}

			matches = append(matches, m)
			if len(matches) >= opts.maxMatches {
				return io.EOF
			}
		}
		return nil
	})

	if err == io.EOF {
		err = nil
	}
	return matches, err
}

// ── Replace preview ──────────────────────────────────────────────────────────

// ReplaceMatch extends Match with a replacedText field for preview.
type ReplaceMatch struct {
	Match
	ReplacedText string `json:"replacedText"`
}

// ReplacePreview handles GET /api/search/replace-preview
//
// Params: q, replace, case, word, regex, include, exclude
// Returns matches with replacedText showing what the replacement would produce.
func (h *Handler) ReplacePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "q is required"})
		return
	}

	replace := r.URL.Query().Get("replace")
	opts := searchOpts{
		query:         q,
		caseSensitive: r.URL.Query().Get("case") == "true",
		wholeWord:     r.URL.Query().Get("word") == "true",
		useRegex:      r.URL.Query().Get("regex") == "true",
		includeGlob:   r.URL.Query().Get("include"),
		excludeGlob:   r.URL.Query().Get("exclude"),
		maxMatches:    500,
	}

	// Always compile for replacement logic.
	re, err := compilePattern(opts.query, opts.caseSensitive, opts.wholeWord, opts.useRegex)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid regex: " + err.Error()})
		return
	}

	ws := h.ws.Current()
	var baseMatches []Match
	var searchErr error
	if rgPath, err2 := exec.LookPath("rg"); err2 == nil {
		baseMatches, searchErr = searchWithRg(rgPath, ws, opts)
	} else {
		baseMatches, searchErr = searchWithWalk(ws, opts)
	}
	if searchErr != nil {
		baseMatches = []Match{}
	}
	truncated := false
	if len(baseMatches) > opts.maxMatches {
		baseMatches = baseMatches[:opts.maxMatches]
		truncated = true
	}

	queryLower := strings.ToLower(opts.query)
	rMatches := make([]ReplaceMatch, 0, len(baseMatches))
	for _, m := range baseMatches {
		replacedText := applyReplace(m.Text, opts.query, replace, queryLower, re, opts)
		rMatches = append(rMatches, ReplaceMatch{Match: m, ReplacedText: replacedText})
	}

	fileSet := map[string]struct{}{}
	for _, m := range rMatches {
		fileSet[m.File] = struct{}{}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"matches":       rMatches,
		"truncated":     truncated,
		"count":         len(rMatches),
		"affectedFiles": len(fileSet),
	})
}

// ── Replace apply ─────────────────────────────────────────────────────────────

// ReplaceApply handles POST /api/search/replace
//
// Body (JSON):
//
//	{ "query":"...", "replace":"...", "caseSensitive":bool, "wholeWord":bool,
//	  "useRegex":bool, "includeGlob":"...", "excludeGlob":"...", "paths":[] }
//
// Applies the replacement to all matched files (or only paths if non-empty).
func (h *Handler) ReplaceApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.checkWorkspace(w) {
		return
	}

	var body struct {
		Query         string   `json:"query"`
		Replace       string   `json:"replace"`
		CaseSensitive bool     `json:"caseSensitive"`
		WholeWord     bool     `json:"wholeWord"`
		UseRegex      bool     `json:"useRegex"`
		IncludeGlob   string   `json:"includeGlob"`
		ExcludeGlob   string   `json:"excludeGlob"`
		Paths         []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "query is required"})
		return
	}

	opts := searchOpts{
		query:         body.Query,
		caseSensitive: body.CaseSensitive,
		wholeWord:     body.WholeWord,
		useRegex:      body.UseRegex,
		includeGlob:   body.IncludeGlob,
		excludeGlob:   body.ExcludeGlob,
		maxMatches:    500,
	}

	re, err := compilePattern(opts.query, opts.caseSensitive, opts.wholeWord, opts.useRegex)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid regex: " + err.Error()})
		return
	}

	ws := h.ws.Current()
	var baseMatches []Match
	var searchErr error
	if rgPath, err2 := exec.LookPath("rg"); err2 == nil {
		baseMatches, searchErr = searchWithRg(rgPath, ws, opts)
	} else {
		baseMatches, searchErr = searchWithWalk(ws, opts)
	}
	if searchErr != nil {
		baseMatches = []Match{}
	}

	pathFilter := map[string]bool{}
	for _, p := range body.Paths {
		pathFilter[p] = true
	}

	fileSet := map[string]struct{}{}
	for _, m := range baseMatches {
		if len(pathFilter) > 0 && !pathFilter[m.File] {
			continue
		}
		fileSet[m.File] = struct{}{}
	}

	if len(fileSet) > 200 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many files (>200)"})
		return
	}

	queryLower := strings.ToLower(opts.query)
	filesChanged := 0
	totalReplacements := 0

	for relPath := range fileSet {
		full, ok := h.safePath(relPath)
		if !ok {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		content := string(data)
		newContent := applyReplace(content, opts.query, body.Replace, queryLower, re, opts)
		count := countReplacements(content, newContent, opts.query, queryLower, re, opts)

		if count == 0 || newContent == content {
			continue
		}
		totalReplacements += count
		if totalReplacements > 10000 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "too many replacements (>10000)"})
			return
		}
		if err := os.WriteFile(full, []byte(newContent), 0644); err != nil {
			continue
		}
		filesChanged++
	}

	json.NewEncoder(w).Encode(map[string]any{
		"filesChanged": filesChanged,
		"replacements": totalReplacements,
	})
}

// ── Replace helpers ───────────────────────────────────────────────────────────

// applyReplace applies the replacement to s using the compiled regexp (which
// handles all option combinations — case, whole-word, regex).
func applyReplace(s, query, replace, queryLower string, re *regexp.Regexp, opts searchOpts) string {
	if opts.useRegex || opts.wholeWord {
		return re.ReplaceAllString(s, replace)
	}
	if opts.caseSensitive {
		return strings.ReplaceAll(s, query, replace)
	}
	return replaceAllCaseInsensitive(s, queryLower, replace)
}

// countReplacements estimates how many replacements were made.
func countReplacements(original, replaced, query, queryLower string, re *regexp.Regexp, opts searchOpts) int {
	if opts.useRegex || opts.wholeWord {
		return len(re.FindAllString(original, -1))
	}
	if opts.caseSensitive {
		return strings.Count(original, query)
	}
	return strings.Count(strings.ToLower(original), queryLower)
}

// replaceAllCaseInsensitive replaces all case-insensitive occurrences of needle in s with repl.
func replaceAllCaseInsensitive(s, needle, repl string) string {
	if needle == "" {
		return s
	}
	lower := strings.ToLower(s)
	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(lower[start:], needle)
		if idx < 0 {
			b.WriteString(s[start:])
			break
		}
		abs := start + idx
		b.WriteString(s[start:abs])
		b.WriteString(repl)
		start = abs + len(needle)
	}
	return b.String()
}

// ── Misc helpers ──────────────────────────────────────────────────────────────

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
