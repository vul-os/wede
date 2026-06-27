package dap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAdapters_builtinsAndConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".wede"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"adapters":{
	  "node": {"command":"js-debug","extensions":[".js",".ts"]},
	  "go":   {"command":"dlv-custom","args":["dap"],"extensions":["go"]}
	}}`
	if err := os.WriteFile(filepath.Join(home, ".wede", "debug.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	m := resolveAdapters("")
	if m["go"].bin != "dlv-custom" {
		t.Fatalf("config should override the built-in go adapter: %+v", m["go"])
	}
	if m["python"].bin != "debugpy-adapter" {
		t.Fatalf("built-in python should remain: %+v", m["python"])
	}
	if m["node"].bin != "js-debug" {
		t.Fatalf("config node adapter not added: %+v", m["node"])
	}
	found := false
	for _, e := range m["node"].exts {
		if e == "js" { // ".js" → "js"
			found = true
		}
	}
	if !found {
		t.Fatalf("node extension not normalised: %+v", m["node"].exts)
	}
}

func TestResolveAdapters_builtinsOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := resolveAdapters("")
	if m["go"].bin != "dlv" || len(m["go"].args) != 1 || m["go"].args[0] != "dap" {
		t.Fatalf("built-in go adapter wrong: %+v", m["go"])
	}
}
