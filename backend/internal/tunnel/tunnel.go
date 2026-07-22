// Package tunnel exposes a loopback-bound wede on the public internet without
// opening ports.
//
// The actual tunneling mechanism sits behind the Provider seam (see
// provider.go): Manager never names a specific implementation. By default
// wede wires in the embedded Vulos Relay agent (see provider_relay.go), which
// dials a single outbound wss:// connection to the relay server the owner
// runs, authenticates with a bearer token, claims its token-authorized
// public name, and proxies inbound requests to wede's ONE local loopback port
// (never an arbitrary host). That's the shipped default, not a hard
// requirement — an alternate Provider (Cloudflare Tunnel, ngrok, frp,
// Tailscale Funnel, ...) can be substituted via NewWithProvider without
// touching Manager, main.go, or the HTTP handlers. See also
// docs/PUBLIC-ACCESS.md for exposing wede publicly without any tunnel at all
// (direct bind + reverse proxy).
//
// The Manager wraps the provider: it persists the owner's config
// (~/.wede/tunnel.json), constructs the provider with LocalAddr pinned to
// wede's own loopback listen address, and delegates Start/Stop/status/PublicURL.
//
// This is an owner-only feature: publishing wede to the internet is privileged.
package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Status is the tunnel's lifecycle state. Values match the provider's Status.
type Status string

const (
	StatusStopped   Status = "stopped"
	StatusStarting  Status = "starting"
	StatusConnected Status = "connected"
	StatusError     Status = "error"
)

// Tunnel is the relay-free surface that main.go and the HTTP handlers rely on.
// It names no specific tunnel mechanism, so an alternate Provider (see
// provider.go) can be swapped in later without any caller change. Manager is
// the only implementation.
type Tunnel interface {
	Start() error
	Stop() error
	PublicURL() string
	Snapshot() State
}

var _ Tunnel = (*Manager)(nil)

// Config is the owner-supplied tunnel configuration (persisted to ~/.wede/tunnel.json).
//
// These fields describe the shipped default provider (a Vulos Relay server:
// ServerURL/Token/Name). An alternate Provider is free to interpret them
// differently, or a future provider-specific config could be layered in —
// Manager itself only persists and redacts this struct, it doesn't interpret it.
type Config struct {
	// ServerURL is the relay control endpoint (for the default provider). http/
	// https/ws/wss are all accepted (http/https are normalized to ws/wss).
	ServerURL string `json:"serverUrl"`
	// Token is the per-agent bearer secret. Owner-only secret — redacted on read.
	Token string `json:"token"`
	// Name is the requested public name (subdomain / path segment).
	Name string `json:"name"`
}

// Manager owns the tunnel Provider's lifecycle. Safe for concurrent use.
type Manager struct {
	localAddr   string // wede's loopback listen addr, e.g. "127.0.0.1:9090"
	dataDir     string // ~/.wede
	newProvider ProviderFactory

	mu       sync.Mutex
	cfg      Config
	provider Provider
	// last non-provider error (e.g. a Start validation failure) surfaced to the
	// UI until the provider takes over the status.
	startErr string
}

// New returns a Manager that tunnels wede's local loopback address using the
// default Provider (the embedded Vulos Relay agent). localAddr is wede's own
// listen address (host:port) and MUST be loopback. It loads any persisted
// config.
func New(localAddr string) *Manager {
	return NewWithProvider(localAddr, DefaultProviderFactory)
}

// NewWithProvider is New, but with an explicit ProviderFactory — the seam for
// wiring in an alternate tunnel mechanism (Cloudflare Tunnel, ngrok, frp,
// Tailscale Funnel, a test fake, ...) instead of the default Vulos Relay
// agent. Callers of the returned *Manager (main.go, the HTTP handlers) are
// unaffected either way.
func NewWithProvider(localAddr string, factory ProviderFactory) *Manager {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".wede")
	m := &Manager{localAddr: localAddr, dataDir: dataDir, newProvider: factory}
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

// SetConfig validates and persists the tunnel config. Restarts the tunnel if running.
func (m *Manager) SetConfig(c Config) error {
	if err := validateConfig(c); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = c
	wasRunning := m.provider != nil
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

// Start brings the tunnel up by launching the configured Provider against
// wede's loopback address. No-op if already running.
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.provider != nil {
		m.mu.Unlock()
		return nil // already running
	}
	cfg := m.cfg
	factory := m.newProvider
	m.mu.Unlock()

	if err := validateConfig(cfg); err != nil {
		m.mu.Lock()
		m.startErr = err.Error()
		m.mu.Unlock()
		return err
	}

	p := factory(ProviderOptions{
		ServerURL: cfg.ServerURL,
		Token:     cfg.Token,
		Name:      cfg.Name,
		LocalAddr: m.localAddr, // wede's own loopback listen addr (SSRF-guarded)
	})
	if err := p.Start(); err != nil {
		m.mu.Lock()
		m.startErr = err.Error()
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	m.provider = p
	m.startErr = ""
	m.mu.Unlock()
	return nil
}

// Stop tears the tunnel down. No-op if not running.
func (m *Manager) Stop() error {
	m.mu.Lock()
	p := m.provider
	m.provider = nil
	m.startErr = ""
	m.mu.Unlock()
	if p != nil {
		p.Stop()
	}
	return nil
}

// Close stops the tunnel (called on shutdown).
func (m *Manager) Close() { _ = m.Stop() }

// PublicURL returns the current public URL, or "" if not connected.
func (m *Manager) PublicURL() string {
	m.mu.Lock()
	p := m.provider
	m.mu.Unlock()
	if p == nil {
		return ""
	}
	return p.PublicURL()
}

// State is the JSON snapshot returned to the owner UI (token redacted).
type State struct {
	Status    Status   `json:"status"`
	PublicURL string   `json:"publicUrl,omitempty"`
	Config    Config   `json:"config"`
	Log       []string `json:"log,omitempty"`
}

// Snapshot returns the current state. The tunnel token is redacted.
func (m *Manager) Snapshot() State {
	m.mu.Lock()
	p := m.provider
	cfg := m.cfg
	startErr := m.startErr
	m.mu.Unlock()

	if cfg.Token != "" {
		cfg.Token = "" // never leak the token back to the client
	}

	if p == nil {
		var log []string
		status := StatusStopped
		if startErr != "" {
			status = StatusError
			log = []string{startErr}
		}
		return State{Status: status, Config: cfg, Log: log}
	}

	snap := p.Snapshot()
	url := ""
	if snap.Connected {
		url = snap.PublicURL
	}
	return State{
		Status:    snap.Status,
		PublicURL: url,
		Config:    cfg,
		Log:       snap.Log,
	}
}
