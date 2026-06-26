package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"wede/backend/internal/folder"
)

func TestSetWedeHostMovesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate ~/.wede/wede-hosts.json

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	// seed a .wede at the root with some content
	if err := os.MkdirAll(filepath.Join(root, ".wede"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".wede", "chat.md"), []byte("- 2026-06-26T00:00:00Z [user] ava: hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Workspace{ID: "t", Name: "t", ws: folder.New(root)}

	if got := r.WedeHost(); got != "" {
		t.Fatalf("default host = %q, want empty (root)", got)
	}

	if err := r.SetWedeHost("backend"); err != nil {
		t.Fatalf("SetWedeHost(backend): %v", err)
	}
	if got := r.WedeHost(); got != "backend" {
		t.Errorf("host after set = %q, want backend", got)
	}
	// content moved to the new location
	if _, err := os.Stat(filepath.Join(root, "backend", ".wede", "chat.md")); err != nil {
		t.Errorf("chat.md not at new location: %v", err)
	}
	// old location removed
	if _, err := os.Stat(filepath.Join(root, ".wede")); !os.IsNotExist(err) {
		t.Errorf("old .wede should be gone, stat err=%v", err)
	}
	// WedeDir reflects the new host
	if got, want := r.WedeDir(), filepath.Join(root, "backend", ".wede"); got != want {
		t.Errorf("WedeDir = %q, want %q", got, want)
	}

	// escapes and missing folders are rejected
	if err := r.SetWedeHost("../escape"); err == nil {
		t.Error("expected error for .. escape")
	}
	if err := r.SetWedeHost("does-not-exist"); err == nil {
		t.Error("expected error for missing folder")
	}

	// moving back to root works
	if err := r.SetWedeHost(""); err != nil {
		t.Fatalf("SetWedeHost(root): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".wede", "chat.md")); err != nil {
		t.Errorf("chat.md not moved back to root: %v", err)
	}
}
