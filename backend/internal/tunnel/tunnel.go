// Package tunnel exposes a loopback-bound wede on the public internet without
// opening ports — using the owner's OWN sovereign Vulos Relay server.
//
// wede does NOT bundle a public relay and no longer shells out to a third-party
// frp binary. Instead it embeds the Vulos Relay agent
// (github.com/vul-os/vulos-relay/tunnel/agent), which dials a single outbound
// wss:// connection to the relay server the owner runs, authenticates with a
// bearer token, claims its token-authorized public name, and proxies inbound
// requests to wede's ONE local loopback port (never an arbitrary host).
//
// The Manager wraps the agent: it persists the owner's relay config
// (~/.wede/tunnel.json), constructs the agent with LocalAddr pinned to wede's
// own loopback listen address, and delegates Start/Stop/status/PublicURL.
//
// This is an owner-only feature: publishing wede to the internet is privileged.
package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vul-os/vulos-relay/tunnel/agent"
)

// Status is the tunnel's lifecycle state. Values match the agent's Status.
type Status string

const (
	StatusStopped   Status = "stopped"
	StatusStarting  Status = "starting"
	StatusConnected Status = "connected"
	StatusError     Status = "error"
)

// Config is the owner-supplied relay configuration (persisted to ~/.wede/tunnel.json).
//
// The frp-era fields (Mode/ServerPort/Domain/RemotePort/HTTPS) are gone: the
// sovereign relay derives the public URL itself and the owner only supplies where
// their relay lives (ServerURL), the bearer token, and the public name they want.
type Config struct {
	// ServerURL is the owner's Vulos Relay control endpoint. http/https/ws/wss are
	// all accepted (http/https are normalized to ws/wss by the agent).
	ServerURL string `json:"serverUrl"`
	// Token is the per-agent bearer token; the relay validates it and derives the
	// permitted name(s) from it. Owner-only secret — redacted on read.
	Token string `json:"token"`
	// Name is the requested public name (subdomain / path segment). The relay only
	// honors it if the token authorizes it.
	Name string `json:"name"`
}

// Manager owns the Vulos Relay agent lifecycle. Safe for concurrent use.
type Manager struct {
	localAddr string // wede's loopback listen addr, e.g. "127.0.0.1:9090"
	dataDir   string // ~/.wede

	mu    sync.Mutex
	cfg   Config
	agent *agent.Agent
	// last non-agent error (e.g. a Start validation failure) surfaced to the UI
	// until the agent takes over the status.
	startErr string
}

// New returns a Manager that tunnels wede's local loopback address. localAddr is
// wede's own listen address (host:port) and MUST be loopback. It loads any
// persisted config.
func New(localAddr string) *Manager {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".wede")
	m := &Manager{localAddr: localAddr, dataDir: dataDir}
	m.loadConfig()
	return m
}

func (m *Manager) configFile() string { return filepath.Join(m.dataDir, "tunnel.json") }

func (m *Manager) loadConfig() {
	data, err := os.ReadFile(m.configFile())
	if err != nil {
		return
	}
	var c Config
	if json.Unmarshal(data, &c) == nil {
		m.cfg = c
	}
}

func (m *Manager) saveConfig() {
	data, _ := json.MarshalIndent(m.cfg, "", "  ")
	os.MkdirAll(m.dataDir, 0o700)
	os.WriteFile(m.configFile(), data, 0o600) // contains the relay token
}

// SetConfig validates and persists the relay config. Restarts the tunnel if running.
func (m *Manager) SetConfig(c Config) error {
	if err := validateConfig(c); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = c
	wasRunning := m.agent != nil
	m.mu.Unlock()
	m.saveConfig()
	if wasRunning {
		if err := m.Stop(); err != nil {
			return err
		}
		return m.Start()
	}
	return nil
}

func validateConfig(c Config) error {
	if strings.TrimSpace(c.ServerURL) == "" {
		return fmt.Errorf("tunnel: serverUrl is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("tunnel: token is required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("tunnel: name is required")
	}
	return nil
}

// Start brings the tunnel up by launching the Vulos Relay agent against wede's
// loopback address. No-op if already running.
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.agent != nil {
		m.mu.Unlock()
		return nil // already running
	}
	cfg := m.cfg
	m.mu.Unlock()

	if err := validateConfig(cfg); err != nil {
		m.mu.Lock()
		m.startErr = err.Error()
		m.mu.Unlock()
		return err
	}

	a := agent.New(agent.Options{
		ServerURL: cfg.ServerURL,
		Token:     cfg.Token,
		Name:      cfg.Name,
		LocalAddr: m.localAddr, // wede's own loopback listen addr (SSRF-guarded)
	})
	if err := a.Start(context.Background()); err != nil {
		m.mu.Lock()
		m.startErr = err.Error()
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	m.agent = a
	m.startErr = ""
	m.mu.Unlock()
	return nil
}

// Stop tears the tunnel down. No-op if not running.
func (m *Manager) Stop() error {
	m.mu.Lock()
	a := m.agent
	m.agent = nil
	m.startErr = ""
	m.mu.Unlock()
	if a != nil {
		a.Stop()
	}
	return nil
}

// Close stops the tunnel (called on shutdown).
func (m *Manager) Close() { _ = m.Stop() }

// PublicURL returns the current public URL, or "" if not connected.
func (m *Manager) PublicURL() string {
	m.mu.Lock()
	a := m.agent
	m.mu.Unlock()
	if a == nil {
		return ""
	}
	return a.PublicURL()
}

// State is the JSON snapshot returned to the owner UI (token redacted).
type State struct {
	Status    Status   `json:"status"`
	PublicURL string   `json:"publicUrl,omitempty"`
	Config    Config   `json:"config"`
	Log       []string `json:"log,omitempty"`
}

// Snapshot returns the current state. The relay token is redacted.
func (m *Manager) Snapshot() State {
	m.mu.Lock()
	a := m.agent
	cfg := m.cfg
	startErr := m.startErr
	m.mu.Unlock()

	if cfg.Token != "" {
		cfg.Token = "" // never leak the token back to the client
	}

	if a == nil {
		var log []string
		status := StatusStopped
		if startErr != "" {
			status = StatusError
			log = []string{startErr}
		}
		return State{Status: status, Config: cfg, Log: log}
	}

	snap := a.Snapshot()
	url := ""
	if snap.Connected {
		url = snap.PublicURL
	}
	return State{
		Status:    Status(snap.Status),
		PublicURL: url,
		Config:    cfg,
		Log:       snap.Log,
	}
}
