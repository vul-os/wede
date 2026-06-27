package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()

	if IsTrusted(root) {
		t.Fatal("a fresh workspace should be untrusted")
	}
	if err := Trust(root); err != nil {
		t.Fatal(err)
	}
	if !IsTrusted(root) {
		t.Fatal("should be trusted after Trust")
	}
	// Survives a fresh read (persisted to disk).
	if !load()[norm(root)] {
		t.Fatal("trust not persisted")
	}
	if err := Untrust(root); err != nil {
		t.Fatal(err)
	}
	if IsTrusted(root) {
		t.Fatal("should be untrusted after Untrust")
	}
}

func TestIsTrusted_emptyRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if IsTrusted("") {
		t.Fatal("empty root must never be trusted")
	}
}

func TestHasProjectConfig(t *testing.T) {
	root := t.TempDir()
	if HasProjectConfig(root) {
		t.Fatal("empty workspace should report no project config")
	}
	if err := os.MkdirAll(filepath.Join(root, ".wede"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".wede", "tasks.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !HasProjectConfig(root) {
		t.Fatal("should detect .wede/tasks.json")
	}
}
