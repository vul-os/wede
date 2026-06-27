package files

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// formatterSpec is a user-configured formatter. The command receives the source
// on stdin and must write the formatted result to stdout (the convention every
// mainstream formatter supports). "{file}" anywhere in args is replaced with the
// file's base name, for tools that need a filename hint (e.g. prettier's
// --stdin-filepath, clang-format's --assume-filename).
type formatterSpec struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type formatterConfig struct {
	Formatters map[string]formatterSpec `json:"formatters"`
}

// loadFormatters reads owner-defined formatters from ~/.wede/formatters.json so
// any language can be formatted on save without recompiling. It is intentionally
// global (owner-controlled) — not read from the shared workspace — because the
// command runs on the host and a project-committed command would let any editor
// run code as the owner. Project-scoped tool config arrives later behind an
// explicit workspace-trust gate. Keys are extensions, normalised to lower-case
// without a leading dot. A missing/invalid file yields an empty map.
func loadFormatters() map[string]formatterSpec {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".wede", "formatters.json"))
	if err != nil {
		return nil
	}
	var cfg formatterConfig
	if json.Unmarshal(data, &cfg) != nil {
		return nil
	}
	out := make(map[string]formatterSpec, len(cfg.Formatters))
	for ext, spec := range cfg.Formatters {
		ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
		if ext == "" || strings.TrimSpace(spec.Command) == "" {
			continue
		}
		out[ext] = spec
	}
	return out
}

// runFormatter pipes src through a configured formatter, substituting {file}
// with base in the args. Returns (formatted, ok, errMsg); ok=false leaves the
// caller to fall back to a built-in or no-op, exactly like the built-ins.
func runFormatter(spec formatterSpec, src, base string) (string, bool, string) {
	bin, err := exec.LookPath(spec.Command)
	if err != nil {
		return "", false, "formatter not available"
	}
	args := make([]string, len(spec.Args))
	for i, a := range spec.Args {
		args[i] = strings.ReplaceAll(a, "{file}", base)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(src)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", false, strings.TrimSpace(stderr.String())
	}
	if stdout.Len() == 0 {
		return "", false, "formatter produced no output"
	}
	return stdout.String(), true, ""
}
