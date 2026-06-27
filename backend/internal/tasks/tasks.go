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

// Load reads and validates the task list. A missing or invalid file yields an
// empty slice (never an error to the caller).
func Load() []Task {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".wede", "tasks.json"))
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

// HandleList serves GET /api/tasks → {"tasks":[...]}.
func HandleList(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"tasks": Load()}) //nolint:errcheck
}
