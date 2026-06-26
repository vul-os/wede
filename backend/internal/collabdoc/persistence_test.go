package collabdoc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reearth/ygo/crdt"
)

func TestDiskPersistenceLoadSeedsFromFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("file body"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewDiskPersistence(dir)

	update, err := p.LoadDoc("a.txt")
	if err != nil {
		t.Fatalf("LoadDoc: %v", err)
	}
	if len(update) == 0 {
		t.Fatal("expected a non-empty update for an existing file")
	}

	// A fresh doc seeded from the update must materialize the file's text.
	d := crdt.New()
	if err := crdt.ApplyUpdateV1(d, update, nil); err != nil {
		t.Fatalf("ApplyUpdateV1: %v", err)
	}
	if got := d.GetText(contentField).ToString(); got != "file body" {
		t.Fatalf("materialized = %q, want %q", got, "file body")
	}
}

func TestDiskPersistenceLoadSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewDiskPersistence(dir)

	update, err := p.LoadDoc("src/main.go")
	if err != nil || len(update) == 0 {
		t.Fatalf("LoadDoc subdir: err=%v len=%d", err, len(update))
	}
	d := crdt.New()
	crdt.ApplyUpdateV1(d, update, nil) //nolint:errcheck
	if got := d.GetText(contentField).ToString(); got != "package main" {
		t.Errorf("subdir materialized = %q", got)
	}
}

func TestDiskPersistenceMissingFileEmptyDoc(t *testing.T) {
	p := NewDiskPersistence(t.TempDir())
	update, err := p.LoadDoc("does-not-exist.txt")
	if err != nil {
		t.Fatalf("LoadDoc: %v", err)
	}
	if update != nil {
		t.Errorf("missing file should seed empty (nil update), got %d bytes", len(update))
	}
}

func TestDiskPersistenceTraversalBlocked(t *testing.T) {
	p := NewDiskPersistence(t.TempDir())
	for _, bad := range []string{"../../etc/passwd", "..", "", "."} {
		update, err := p.LoadDoc(bad)
		if err != nil {
			t.Fatalf("LoadDoc(%q): %v", bad, err)
		}
		if update != nil {
			t.Errorf("LoadDoc(%q) should be blocked/empty, got %d bytes", bad, len(update))
		}
	}
}

func TestDiskPersistenceStoreUpdateNoop(t *testing.T) {
	p := NewDiskPersistence(t.TempDir())
	if err := p.StoreUpdate("a.txt", []byte{1, 2, 3}); err != nil {
		t.Errorf("StoreUpdate should be a no-op returning nil, got %v", err)
	}
}
