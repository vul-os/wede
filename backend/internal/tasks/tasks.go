// Package tasks serves the owner's named run/build/test commands from
// ~/.wede/tasks.json. The commands themselves run client-side in a terminal
// (PTY), so this package only parses + lists them — it never executes anything.
//
// Owner-controlled + global by design: a task is an arbitrary host command, so
// it is deliberately not read from the shared workspace (project-scoped tasks
// will arrive behind a workspace-trust gate).
package tasks

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"wede/backend/internal/trust"
)

// Task is one named command from ~/.wede/tasks.json.
type Task struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"` // relative to the workspace root; optional
}

type config struct {
	Tasks []Task `json:"tasks"`
}

// Load returns the active task list: the owner's global ~/.wede/tasks.json plus
// the workspace's committed <root>/.wede/tasks.json — the latter only when the
// owner has trusted the workspace, since a task runs a host command.
func Load(root string) []Task {
	var out []Task
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, loadFile(filepath.Join(home, ".wede", "tasks.json"))...)
	}
	if root != "" && trust.IsTrusted(root) {
		out = append(out, loadFile(filepath.Join(root, ".wede", "tasks.json"))...)
	}
	return out
}

// loadFile parses + validates one tasks.json (missing/invalid → empty).
func loadFile(path string) []Task {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg config
	if json.Unmarshal(data, &cfg) != nil {
		return nil
	}
	out := make([]Task, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		t.Name = strings.TrimSpace(t.Name)
		t.Command = strings.TrimSpace(t.Command)
		t.Cwd = strings.TrimSpace(t.Cwd)
		if t.Name == "" || t.Command == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Handler serves GET /api/workspaces/{id}/tasks → {"tasks":[...]} for one root.
func Handler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"tasks": Load(root)}) //nolint:errcheck
	}
}
