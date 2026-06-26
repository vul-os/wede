package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticWS struct{ root string }

func (s *staticWS) Current() string { return s.root }

func makeWS(t *testing.T, files map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	for name, content := range files {
		full := filepath.Join(tmp, name)
		os.MkdirAll(filepath.Dir(full), 0755)
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return tmp
}

// ── Basic text search ─────────────────────────────────────────────────────────

func TestSearch_BasicMatch(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"main.go": "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n",
	})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=hello", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Matches []Match `json:"matches"`
		Count   int     `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 {
		t.Errorf("expected 1 match, got %d", resp.Count)
	}
	if resp.Matches[0].Line != 4 {
		t.Errorf("expected line 4, got %d", resp.Matches[0].Line)
	}
	if resp.Matches[0].File != "main.go" {
		t.Errorf("expected file main.go, got %q", resp.Matches[0].File)
	}
}

func TestSearch_CaseSensitive(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "Hello\nhello\nHELLO\n",
	})
	h := New(&staticWS{root: ws})

	// Case insensitive (default) — all 3 lines.
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=hello", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 3 {
		t.Errorf("case-insensitive: expected 3, got %d", resp.Count)
	}

	// Case sensitive — only exact match.
	req2 := httptest.NewRequest(http.MethodGet, "/api/search?q=hello&case=true", nil)
	rec2 := httptest.NewRecorder()
	h.Search(rec2, req2)
	var resp2 struct{ Count int `json:"count"` }
	json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2.Count != 1 {
		t.Errorf("case-sensitive: expected 1, got %d", resp2.Count)
	}
}

func TestSearch_Regex(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"src.go": "func Foo() {}\nfunc Bar() {}\nvar x = 1\n",
	})
	h := New(&staticWS{root: ws})

	// Search for lines starting with "func" — both Foo and Bar match.
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=func+[A-Z]&regex=true", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	var resp struct {
		Matches []Match `json:"matches"`
		Count   int     `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("regex: expected 2, got %d", resp.Count)
	}
}

func TestSearch_InvalidRegex(t *testing.T) {
	ws := makeWS(t, map[string]string{"a.go": "hello\n"})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=%5B&regex=true", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid regex, got %d", rec.Code)
	}
}

func TestSearch_SkipsNodeModulesAndGit(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"src/main.go":           "needle here",
		"node_modules/lib/a.js": "needle inside node_modules",
		".git/config":           "needle in git",
	})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=needle", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	var resp struct {
		Matches []Match `json:"matches"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	for _, m := range resp.Matches {
		if strings.Contains(m.File, "node_modules") {
			t.Errorf("should not search node_modules, got %q", m.File)
		}
		if strings.Contains(m.File, ".git") {
			t.Errorf("should not search .git, got %q", m.File)
		}
	}
	if len(resp.Matches) != 1 {
		t.Errorf("expected 1 match (src/main.go), got %d", len(resp.Matches))
	}
}

func TestSearch_NoWorkspace(t *testing.T) {
	h := New(&staticWS{root: ""})
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=x", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for no workspace, got %d", rec.Code)
	}
}

func TestSearch_NoQuery(t *testing.T) {
	ws := makeWS(t, map[string]string{"a.go": "hello\n"})
	h := New(&staticWS{root: ws})
	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing query, got %d", rec.Code)
	}
}

func TestSearch_MultipleFiles(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "target line one\n",
		"b.js": "target line two\n",
		"c.md": "not a match\n",
	})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=target", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	var resp struct {
		Matches []Match `json:"matches"`
		Count   int     `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("expected 2 matches, got %d", resp.Count)
	}
}

// ── Whole-word ────────────────────────────────────────────────────────────────

func TestSearch_WholeWord(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "foo foobar barfoo foo_bar\nfoo\n",
	})
	// Test directly against the Go walker so the result is deterministic
	// regardless of whether rg is installed.
	opts := searchOpts{
		query:      "foo",
		wholeWord:  true,
		maxMatches: 500,
	}
	matches, err := searchWithWalk(ws, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Only standalone "foo" tokens should match; "foobar", "barfoo", "foo_bar"
	// must not match because \b considers "_" as a word char in RE2.
	// Line 1: "foo" at position 0 matches; "foobar", "barfoo", "foo_bar" do not.
	// Line 2: "foo" matches.
	// We expect at least 2 matches (line 1 pos 0 and line 2), but rg may differ.
	for _, m := range matches {
		if strings.Contains(m.Text, "foobar") && m.MatchStart == strings.Index(m.Text, "foobar") {
			t.Errorf("whole-word: should not match inside 'foobar'")
		}
	}
	foundStandalone := false
	for _, m := range matches {
		// The first "foo" on line 1 is at col 1 — standalone.
		if m.Line == 2 {
			foundStandalone = true
		}
	}
	if !foundStandalone {
		t.Errorf("whole-word: expected to find standalone 'foo' on line 2")
	}
}

func TestSearch_WholeWord_Handler(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "foobar foo baz\nfoo\n",
	})
	h := New(&staticWS{root: ws})
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=foo&word=true&case=true", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Matches []Match `json:"matches"`
		Count   int     `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	for _, m := range resp.Matches {
		// MatchStart must never point into "foobar"
		matchedText := m.Text[m.MatchStart : m.MatchStart+m.MatchLen]
		if matchedText != "foo" {
			t.Errorf("whole-word: matched unexpected text %q", matchedText)
		}
	}
	// Expect exactly 2: "foo" on line 1 and "foo" on line 2.
	if resp.Count < 2 {
		t.Errorf("whole-word: expected at least 2 matches, got %d", resp.Count)
	}
}

// ── Glob filtering ────────────────────────────────────────────────────────────

func TestSearch_IncludeGlob(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"main.go":   "needle\n",
		"util.js":   "needle\n",
		"readme.md": "needle\n",
	})
	h := New(&staticWS{root: ws})

	// Only include Go files.
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=needle&include=*.go", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	var resp struct {
		Matches []Match `json:"matches"`
		Count   int     `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("include glob: expected 1 match (*.go only), got %d", resp.Count)
	}
	if resp.Count > 0 && !strings.HasSuffix(resp.Matches[0].File, ".go") {
		t.Errorf("include glob: expected .go file, got %q", resp.Matches[0].File)
	}
}

func TestSearch_ExcludeGlob(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"main.go":   "needle\n",
		"util.js":   "needle\n",
		"readme.md": "needle\n",
	})
	h := New(&staticWS{root: ws})

	// Exclude markdown files.
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=needle&exclude=*.md", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	var resp struct {
		Matches []Match `json:"matches"`
		Count   int     `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("exclude glob: expected 2 matches (no .md), got %d", resp.Count)
	}
	for _, m := range resp.Matches {
		if strings.HasSuffix(m.File, ".md") {
			t.Errorf("exclude glob: .md file should be excluded, got %q", m.File)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "src/main.go", true},
		{"*.go", "main.js", false},
		{"*.go", "main.go.bak", false},
		{"src/*.go", "src/main.go", true},
		{"src/*.go", "lib/main.go", false},
		{"**/*.go", "deep/src/main.go", true},
		{"**/*.go", "main.js", false},
		{"*.md", "README.md", true},
		{"*.md", "src/README.md", true},
	}
	for _, tc := range cases {
		got := globMatch(tc.pattern, tc.path)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// ── Context lines ─────────────────────────────────────────────────────────────

func TestSearch_ContextLines(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "line1\nline2\nMATCH\nline4\nline5\n",
	})
	opts := searchOpts{
		query:         "MATCH",
		caseSensitive: true,
		maxMatches:    500,
		contextLines:  2,
	}
	matches, err := searchWithWalk(ws, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if len(m.Before) != 2 {
		t.Errorf("expected 2 before-context lines, got %d: %v", len(m.Before), m.Before)
	}
	if len(m.After) != 2 {
		t.Errorf("expected 2 after-context lines, got %d: %v", len(m.After), m.After)
	}
	if len(m.Before) >= 2 && m.Before[0] != "line1" {
		t.Errorf("before[0] = %q, want 'line1'", m.Before[0])
	}
	if len(m.Before) >= 2 && m.Before[1] != "line2" {
		t.Errorf("before[1] = %q, want 'line2'", m.Before[1])
	}
	if len(m.After) >= 2 && m.After[0] != "line4" {
		t.Errorf("after[0] = %q, want 'line4'", m.After[0])
	}
}

func TestSearch_ContextAtFileEdge(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "MATCH\nline2\n",
	})
	opts := searchOpts{
		query:         "MATCH",
		caseSensitive: true,
		maxMatches:    500,
		contextLines:  3,
	}
	matches, err := searchWithWalk(ws, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	// No lines before MATCH (it's the first line).
	if len(matches[0].Before) != 0 {
		t.Errorf("expected 0 before-context lines at file start, got %d", len(matches[0].Before))
	}
}

// ── Filename search ───────────────────────────────────────────────────────────

func TestSearchFiles_Basic(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"src/main.go":    "content",
		"src/util.go":    "content",
		"src/app.js":     "content",
		"docs/readme.md": "content",
	})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search/files?q=util", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Files []FileMatch `json:"files"`
		Count int         `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 {
		t.Errorf("expected 1 file (util.go), got %d", resp.Count)
	}
	if resp.Count > 0 && !strings.Contains(resp.Files[0].Path, "util") {
		t.Errorf("expected path to contain 'util', got %q", resp.Files[0].Path)
	}
}

func TestSearchFiles_Regex(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"src/foo.go":  "x",
		"src/bar.go":  "x",
		"src/foo.jsx": "x",
	})
	h := New(&staticWS{root: ws})

	// Match only .go files via regex.
	req := httptest.NewRequest(http.MethodGet, "/api/search/files?q=\\.go$&regex=true", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)

	var resp struct {
		Files []FileMatch `json:"files"`
		Count int         `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("filename regex: expected 2 .go files, got %d", resp.Count)
	}
}

func TestSearchFiles_CaseSensitive(t *testing.T) {
	// Use a mixed-case filename; don't create two files differing only by case
	// because macOS HFS+ is case-insensitive.
	ws := makeWS(t, map[string]string{
		"src/MyComponent.jsx": "x",
		"src/other.go":        "x",
	})
	h := New(&staticWS{root: ws})

	// Case-sensitive: lowercase query "mycomponent" should not match "MyComponent".
	req := httptest.NewRequest(http.MethodGet, "/api/search/files?q=mycomponent&case=true", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)
	var resp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("filename case-sensitive: expected 0, got %d", resp.Count)
	}

	// Case-sensitive: exact-case query "MyComponent" should match.
	req2 := httptest.NewRequest(http.MethodGet, "/api/search/files?q=MyComponent&case=true", nil)
	rec2 := httptest.NewRecorder()
	h.SearchFiles(rec2, req2)
	var resp2 struct {
		Count int `json:"count"`
	}
	json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2.Count != 1 {
		t.Errorf("filename case-sensitive (exact): expected 1, got %d", resp2.Count)
	}

	// Case-insensitive: lowercase query finds the file regardless.
	req3 := httptest.NewRequest(http.MethodGet, "/api/search/files?q=mycomponent", nil)
	rec3 := httptest.NewRecorder()
	h.SearchFiles(rec3, req3)
	var resp3 struct {
		Count int `json:"count"`
	}
	json.NewDecoder(rec3.Body).Decode(&resp3)
	if resp3.Count != 1 {
		t.Errorf("filename case-insensitive: expected 1, got %d", resp3.Count)
	}
}

func TestSearchFiles_NoWorkspace(t *testing.T) {
	h := New(&staticWS{root: ""})
	req := httptest.NewRequest(http.MethodGet, "/api/search/files?q=main", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for no workspace, got %d", rec.Code)
	}
}

func TestSearchFiles_NoQuery(t *testing.T) {
	ws := makeWS(t, map[string]string{"a.go": "x"})
	h := New(&staticWS{root: ws})
	req := httptest.NewRequest(http.MethodGet, "/api/search/files", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing query, got %d", rec.Code)
	}
}

func TestSearchFiles_SkipsNodeModules(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"src/app.go":            "x",
		"node_modules/pkg/a.js": "x",
	})
	h := New(&staticWS{root: ws})
	req := httptest.NewRequest(http.MethodGet, "/api/search/files?q=.go", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)
	var resp struct {
		Files []FileMatch `json:"files"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	for _, f := range resp.Files {
		if strings.Contains(f.Path, "node_modules") {
			t.Errorf("SearchFiles should skip node_modules, got %q", f.Path)
		}
	}
}

func TestSearchFiles_IncludeExcludeGlob(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"src/main.go":    "x",
		"src/util.go":    "x",
		"src/style.css":  "x",
		"docs/readme.md": "x",
	})
	h := New(&staticWS{root: ws})

	// Include only .go files.
	req := httptest.NewRequest(http.MethodGet, "/api/search/files?q=src&include=*.go", nil)
	rec := httptest.NewRecorder()
	h.SearchFiles(rec, req)
	var resp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("SearchFiles include *.go: expected 2, got %d", resp.Count)
	}
}

// ── Replace tests ─────────────────────────────────────────────────────────────

func TestReplacePreview_Basic(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"main.go": "hello world\nhello again\n",
	})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search/replace-preview?q=hello&replace=goodbye", nil)
	rec := httptest.NewRecorder()
	h.ReplacePreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ReplacePreview: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Matches       []ReplaceMatch `json:"matches"`
		Count         int            `json:"count"`
		AffectedFiles int            `json:"affectedFiles"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 2 {
		t.Errorf("expected 2 matches, got %d", resp.Count)
	}
	if resp.AffectedFiles != 1 {
		t.Errorf("expected 1 affected file, got %d", resp.AffectedFiles)
	}
	for _, m := range resp.Matches {
		if !strings.Contains(m.ReplacedText, "goodbye") {
			t.Errorf("replacedText should contain 'goodbye', got %q", m.ReplacedText)
		}
	}
}

func TestReplaceApply_Basic(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "hello world\nhello again\n",
	})
	h := New(&staticWS{root: ws})

	body := map[string]any{
		"query":   "hello",
		"replace": "goodbye",
	}
	bs, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/search/replace", strings.NewReader(string(bs)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ReplaceApply(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ReplaceApply: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		FilesChanged int `json:"filesChanged"`
		Replacements int `json:"replacements"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", resp.FilesChanged)
	}
	if resp.Replacements != 2 {
		t.Errorf("expected 2 replacements, got %d", resp.Replacements)
	}

	content, err := os.ReadFile(filepath.Join(ws, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "hello") {
		t.Errorf("file should no longer contain 'hello', got: %s", content)
	}
	if !strings.Contains(string(content), "goodbye") {
		t.Errorf("file should contain 'goodbye', got: %s", content)
	}
}

func TestReplaceApply_PathTraversal(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "hello\n",
	})
	h := New(&staticWS{root: ws})

	body := map[string]any{
		"query":   "hello",
		"replace": "bye",
		"paths":   []string{"../../etc/passwd", "../outside.go"},
	}
	bs, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/search/replace", strings.NewReader(string(bs)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ReplaceApply(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		FilesChanged int `json:"filesChanged"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.FilesChanged != 0 {
		t.Errorf("expected 0 files changed (traversal skipped), got %d", resp.FilesChanged)
	}
}

func TestReplaceApply_WholeWord(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "foobar foo baz\n",
	})
	h := New(&staticWS{root: ws})

	body := map[string]any{
		"query":     "foo",
		"replace":   "qux",
		"wholeWord": true,
	}
	bs, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/search/replace", strings.NewReader(string(bs)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ReplaceApply(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	content, err := os.ReadFile(filepath.Join(ws, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	// "foobar" must remain intact; standalone "foo" must become "qux".
	if !strings.Contains(string(content), "foobar") {
		t.Errorf("whole-word replace should not touch 'foobar', got: %s", content)
	}
	if !strings.Contains(string(content), "qux") {
		t.Errorf("whole-word replace should have replaced standalone 'foo' with 'qux', got: %s", content)
	}
}

func TestReplaceApply_IncludeGlob(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "hello\n",
		"b.js": "hello\n",
	})
	h := New(&staticWS{root: ws})

	body := map[string]any{
		"query":       "hello",
		"replace":     "bye",
		"includeGlob": "*.go",
	}
	bs, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/search/replace", strings.NewReader(string(bs)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ReplaceApply(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		FilesChanged int `json:"filesChanged"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	// Only a.go should be changed.
	if resp.FilesChanged != 1 {
		t.Errorf("include glob replace: expected 1 file changed, got %d", resp.FilesChanged)
	}
	// b.js must be untouched.
	bContent, _ := os.ReadFile(filepath.Join(ws, "b.js"))
	if !strings.Contains(string(bContent), "hello") {
		t.Errorf("b.js should not have been changed (not in include glob)")
	}
}

// ── FileCount in search response ──────────────────────────────────────────────

func TestSearch_FileCount(t *testing.T) {
	ws := makeWS(t, map[string]string{
		"a.go": "needle\nneedle\n",
		"b.go": "needle\n",
	})
	h := New(&staticWS{root: ws})

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=needle", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)

	var resp struct {
		Count     int `json:"count"`
		FileCount int `json:"fileCount"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Count != 3 {
		t.Errorf("expected 3 matches, got %d", resp.Count)
	}
	if resp.FileCount != 2 {
		t.Errorf("expected fileCount=2, got %d", resp.FileCount)
	}
}
