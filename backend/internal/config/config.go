package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

type Config struct {
	Password       string `json:"password"`
	Port           string `json:"port"`
	// FrameAncestors controls which origins may embed wede in an iframe.
	// Emitted as: Content-Security-Policy: frame-ancestors <value>
	// Leave empty (default) for 'self' — blocks all cross-origin framing.
	// Set to a space-separated list of origins to allow embedding, e.g.:
	//   "https://vulos.org https://app.vulos.org"
	// When non-empty, X-Frame-Options is omitted so the CSP takes effect.
	FrameAncestors string `json:"frame_ancestors,omitempty"`
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

	cfg := &Config{Port: "9090"}
	if err := json.Unmarshal(data, cfg); err != nil {
		log.Fatal("invalid wede.config.json:", err)
	}
	if cfg.Password == "" {
		log.Fatal("password is required in wede.config.json")
	}
	return cfg
}
