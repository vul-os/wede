// Package trust records which workspace roots the owner has approved to run
// commands from their *committed* .wede/ config (formatters, tasks, language
// servers). Project tooling executes host commands, so an untrusted workspace's
// .wede/ config is ignored until the owner trusts it — this prevents a
// collaborator (editor) from running code as the owner via a committed config
// file. The owner's global ~/.wede/ config is always trusted.
//
// The trusted set is persisted to ~/.wede/trusted.json (owner-controlled).
package trust

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var mu sync.Mutex

func storePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wede", "trusted.json"), nil
}

func load() map[string]bool {
	p, err := storePath()
	if err != nil {
		return map[string]bool{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return map[string]bool{}
	}
	var list []string
	if json.Unmarshal(data, &list) != nil {
		return map[string]bool{}
	}
	m := make(map[string]bool, len(list))
	for _, r := range list {
		m[r] = true
	}
	return m
}

func save(m map[string]bool) error {
	p, err := storePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	list := make([]string, 0, len(m))
	for r := range m {
		list = append(list, r)
	}
	sort.Strings(list)
	data, _ := json.MarshalIndent(list, "", "  ") //nolint:errcheck
	return os.WriteFile(p, data, 0o600)
}

// norm canonicalises a root path so trust checks are stable.
func norm(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return filepath.Clean(root)
}

// IsTrusted reports whether the owner has approved running commands from this
// workspace's committed .wede/ config.
func IsTrusted(root string) bool {
	if root == "" {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	return load()[norm(root)]
}

// Trust marks a workspace root as approved.
func Trust(root string) error {
	mu.Lock()
	defer mu.Unlock()
	m := load()
	m[norm(root)] = true
	return save(m)
}

// Untrust revokes approval.
func Untrust(root string) error {
	mu.Lock()
	defer mu.Unlock()
	m := load()
	delete(m, norm(root))
	return save(m)
}

// projectConfigFiles are the committed .wede/ tooling files whose presence makes
// a workspace worth prompting to trust.
var projectConfigFiles = []string{"lsp.json", "formatters.json", "tasks.json"}

// HasProjectConfig reports whether the workspace ships any committed tooling
// config under .wede/ (so the UI can prompt to trust only when relevant).
func HasProjectConfig(root string) bool {
	if root == "" {
		return false
	}
	for _, f := range projectConfigFiles {
		if _, err := os.Stat(filepath.Join(root, ".wede", f)); err == nil {
			return true
		}
	}
	return false
}

// Handler serves the trust status + mutations for one workspace root. The owner
// gate is applied by the route (RequireOwner) in main.go.
func Handler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"trusted":          IsTrusted(root),
				"hasProjectConfig": HasProjectConfig(root),
			})
		case http.MethodPost:
			if err := Trust(root); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"trusted": true}) //nolint:errcheck
		case http.MethodDelete:
			if err := Untrust(root); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"trusted": false}) //nolint:errcheck
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
