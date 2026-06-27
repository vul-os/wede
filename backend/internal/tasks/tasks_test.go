package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".wede"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"tasks":[
	  {"name":"Build","command":"go build ./..."},
	  {"name":"Test","command":"go test ./...","cwd":"backend"},
	  {"name":"  ","command":"x"},
	  {"name":"NoCmd","command":"   "}
	]}`
	if err := os.WriteFile(filepath.Join(home, ".wede", "tasks.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Load("")
	if len(got) != 2 {
		t.Fatalf("want 2 valid tasks, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Build" || got[0].Command != "go build ./..." {
		t.Fatalf("task 0 wrong: %+v", got[0])
	}
	if got[1].Cwd != "backend" {
		t.Fatalf("cwd not parsed: %+v", got[1])
	}
}

func TestLoad_missingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := Load(""); got != nil {
		t.Fatalf("missing file should give nil, got %+v", got)
	}
}
