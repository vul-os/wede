package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type Config struct {
	Password string `json:"password"`
	Port     string `json:"port"`
	// Host is the interface address wede binds to.  Defaults to "127.0.0.1"
	// (localhost only).  Set to "0.0.0.0" or "" to listen on all interfaces,
	// which is required when wede is accessed from another machine or a
	// Docker host.  Keeping the default protects users running wede on a
	// shared or internet-connected machine.
	Host string `json:"host,omitempty"`
	// FrameAncestors controls which origins may embed wede in an iframe.
	// Emitted as: Content-Security-Policy: frame-ancestors <value>
	// Leave empty (default) for 'self' — blocks all cross-origin framing.
	// Set to a space-separated list of origins to allow embedding, e.g.:
	//   "https://vulos.org https://app.vulos.org"
	// When non-empty, X-Frame-Options is omitted so the CSP takes effect.
	FrameAncestors string `json:"frame_ancestors,omitempty"`
	// WorkspaceRoot is the base directory under which workspaces may be opened.
	// Any path passed to POST /api/workspaces or POST /api/folder/open must
	// resolve to a directory inside this base (and must not be the base itself,
	// the filesystem root, or contain a dotfile component such as .ssh). This
	// stops an authenticated editor from adopting sensitive directories (e.g.
	// $HOME, /, ~/.ssh) as a workspace root and reading their contents.
	//
	// Default (empty): the user's home directory. Override with the
	// WEDE_WORKSPACE_ROOT environment variable or this config key.
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	// ServeLanding controls whether the marketing landing page and /site/*
	// docs viewer are hosted. These are cloud-only assets (wede.vulos.org):
	// a local, self-hosted wede should just serve the IDE app. When false
	// (default), unauthenticated visits to / get the app's login form instead
	// of the landing page, and /site/* is not mounted. Set to true only for
	// the public cloud deployment.
	ServeLanding bool `json:"serve_landing,omitempty"`
}

const configName = "wede.config.json"

func Load() *Config {
	var data []byte
	var found string

	// 1. Walk up from cwd (handles running from backend/ or project root)
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for {
			p := filepath.Join(dir, configName)
			if d, err := os.ReadFile(p); err == nil {
				data = d
				found = p
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 2. ~/.config/wede/
	if data == nil {
		if home, err := os.UserHomeDir(); err == nil {
			p := filepath.Join(home, ".config", "wede", configName)
			if d, err := os.ReadFile(p); err == nil {
				data = d
				found = p
			}
		}
	}

	// 3. Next to executable
	if data == nil {
		if exe, err := os.Executable(); err == nil {
			p := filepath.Join(filepath.Dir(exe), configName)
			if d, err := os.ReadFile(p); err == nil {
				data = d
				found = p
			}
		}
	}

	if data == nil {
		log.Fatal("wede.config.json not found (searched cwd, parent dirs, ~/.config/wede/, and next to executable)")
	}

	log.Printf("loaded config from %s", found)

	cfg, err := parse(data)
	if err != nil {
		log.Fatal(err)
	}

	// Resolve the allowed workspace root: env override > config key > $HOME.
	if env := os.Getenv("WEDE_WORKSPACE_ROOT"); env != "" {
		cfg.WorkspaceRoot = env
	}
	if cfg.WorkspaceRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.WorkspaceRoot = home
		}
	}
	if abs, err := filepath.Abs(cfg.WorkspaceRoot); err == nil {
		cfg.WorkspaceRoot = abs
	}

	return cfg
}

// parse decodes and validates the raw config JSON. It applies defaults (port,
// host), rejects unknown keys, and requires a password. Kept separate from Load
// so the decode/validation rules are unit-testable without touching the
// filesystem. ServeLanding defaults to false (a local self-hosted wede serves
// only the IDE; the marketing landing is cloud-only).
func parse(data []byte) (*Config, error) {
	cfg := &Config{Port: "9090", Host: "127.0.0.1"}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("invalid wede.config.json: %w", err)
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("password is required in wede.config.json")
	}
	return cfg, nil
}
