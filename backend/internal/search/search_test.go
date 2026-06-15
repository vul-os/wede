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
	// Both Foo and Bar start with uppercase after "func ".
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
		"src/main.go":             "needle here",
		"node_modules/lib/a.js":   "needle inside node_modules",
		".git/config":             "needle in git",
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
