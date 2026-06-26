package files

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTreeListsFilesAndSkipsNoise(t *testing.T) {
	root := t.TempDir()
	// Real files (some nested).
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o644))
	must(os.MkdirAll(filepath.Join(root, "src"), 0o755))
	must(os.WriteFile(filepath.Join(root, "src", "app.js"), []byte("x"), 0o644))
	// Noise that must be skipped.
	must(os.MkdirAll(filepath.Join(root, "node_modules", "dep"), 0o755))
	must(os.WriteFile(filepath.Join(root, "node_modules", "dep", "index.js"), []byte("x"), 0o644))
	must(os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	must(os.WriteFile(filepath.Join(root, ".git", "config"), []byte("x"), 0o644))

	h := New(&staticWS{root: root})
	rec := httptest.NewRecorder()
	h.Tree(rec, httptest.NewRequest(http.MethodGet, "/api/files/tree", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Files     []string `json:"files"`
		Truncated bool     `json:"truncated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, f := range out.Files {
		got[f] = true
	}
	if !got["main.go"] || !got["src/app.js"] {
		t.Errorf("missing expected files, got %v", out.Files)
	}
	for _, bad := range []string{"node_modules/dep/index.js", ".git/config"} {
		if got[bad] {
			t.Errorf("noise path %q should be skipped", bad)
		}
	}
	// Paths are slash-separated and sorted.
	if len(out.Files) >= 2 && !(out.Files[0] <= out.Files[1]) {
		t.Errorf("files not sorted: %v", out.Files)
	}
}

func TestTreeNoWorkspace(t *testing.T) {
	h := New(&staticWS{root: ""})
	rec := httptest.NewRecorder()
	h.Tree(rec, httptest.NewRequest(http.MethodGet, "/api/files/tree", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no-workspace status = %d, want 400", rec.Code)
	}
}
