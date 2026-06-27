package files

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"wede/backend/internal/trust"
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

// loadFormatters builds the active formatter map: the owner's global
// ~/.wede/formatters.json always applies, and the workspace's committed
// <root>/.wede/formatters.json is merged on top (overriding by extension) only
// when the owner has trusted the workspace — because a formatter runs a host
// command, so an untrusted project file must not be executed. Keys are
// extensions, normalised to lower-case without a leading dot.
func loadFormatters(root string) map[string]formatterSpec {
	out := map[string]formatterSpec{}
	if home, err := os.UserHomeDir(); err == nil {
		mergeFormatterFile(out, filepath.Join(home, ".wede", "formatters.json"))
	}
	if root != "" && trust.IsTrusted(root) {
		mergeFormatterFile(out, filepath.Join(root, ".wede", "formatters.json"))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergeFormatterFile parses one formatters.json into out (later calls override).
func mergeFormatterFile(out map[string]formatterSpec, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg formatterConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	for ext, spec := range cfg.Formatters {
		ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
		if ext == "" || strings.TrimSpace(spec.Command) == "" {
			continue
		}
		out[ext] = spec
	}
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
