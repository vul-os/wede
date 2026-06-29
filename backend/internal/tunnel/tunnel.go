// Package tunnel manages an optional frp (Fast Reverse Proxy) client so an owner
// can expose a loopback-bound wede on the public internet without opening ports.
//
// wede does NOT bundle a public relay — the user supplies their own frps server
// (a cheap VPS). wede detects the `frpc` binary, generates its config from the
// owner's relay settings (pointing frpc at wede's local port), runs frpc as a
// managed subprocess, watches its output to report a live connection status, and
// computes the public URL to show the user.
//
// This is an owner-only feature: publishing wede to the internet is privileged.
package tunnel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Status is the tunnel's lifecycle state.
type Status string

const (
	StatusStopped   Status = "stopped"
	StatusStarting  Status = "starting"
	StatusConnected Status = "connected"
	StatusError     Status = "error"
)

// Mode selects how frp exposes wede.
type Mode string

const (
	ModeHTTP Mode = "http" // vhost by customDomains → http://<domain>
	ModeTCP  Mode = "tcp"  // raw tcp remotePort → http://<serverAddr>:<remotePort>
)

// Config is the owner-supplied relay configuration (persisted to ~/.wede/tunnel.json).
type Config struct {
	ServerAddr string `json:"serverAddr"`
	ServerPort int    `json:"serverPort"`
	Token      string `json:"token"`
	Mode       Mode   `json:"mode"`
	Domain     string `json:"domain,omitempty"`     // ModeHTTP
	RemotePort int    `json:"remotePort,omitempty"` // ModeTCP
	HTTPS      bool   `json:"https,omitempty"`      // build https:// public URL (TLS terminated at VPS)
}

const maxLogLines = 200

// Manager owns the frpc lifecycle. Safe for concurrent use.
type Manager struct {
	localPort string // wede's port (what we tunnel)
	dataDir   string // ~/.wede

	mu        sync.Mutex
	cfg       Config
	cmd       *exec.Cmd
	status    Status
	stopping  bool
	log       []string
	startedAt time.Time
}

// New returns a Manager that tunnels wede's localPort. It loads any persisted config.
func New(localPort string) *Manager {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".wede")
	m := &Manager{localPort: localPort, dataDir: dataDir, status: StatusStopped}
	m.loadConfig()
	return m
}

// FrpcPath returns the resolved frpc binary path and whether it was found.
func FrpcPath() (string, bool) {
	p, err := exec.LookPath("frpc")
	if err != nil {
		return "", false
	}
	return p, true
}

func (m *Manager) configFile() string { return filepath.Join(m.dataDir, "tunnel.json") }
func (m *Manager) frpcToml() string   { return filepath.Join(m.dataDir, "frpc.toml") }

func (m *Manager) loadConfig() {
	data, err := os.ReadFile(m.configFile())
	if err != nil {
		m.cfg = Config{ServerPort: 7000, Mode: ModeHTTP}
		return
	}
	var c Config
	if json.Unmarshal(data, &c) == nil {
		if c.ServerPort == 0 {
			c.ServerPort = 7000
		}
		if c.Mode == "" {
			c.Mode = ModeHTTP
		}
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
	if c.ServerPort == 0 {
		c.ServerPort = 7000
	}
	m.mu.Lock()
	m.cfg = c
	wasRunning := m.cmd != nil
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
	if strings.TrimSpace(c.ServerAddr) == "" {
		return fmt.Errorf("tunnel: serverAddr is required")
	}
	switch c.Mode {
	case ModeHTTP:
		if strings.TrimSpace(c.Domain) == "" {
			return fmt.Errorf("tunnel: a domain is required for http mode")
		}
	case ModeTCP:
		if c.RemotePort <= 0 {
			return fmt.Errorf("tunnel: a remotePort is required for tcp mode")
		}
	default:
		return fmt.Errorf("tunnel: mode must be %q or %q", ModeHTTP, ModeTCP)
	}
	return nil
}

// renderTOML builds the frpc.toml content for the config + local port. Pure.
func renderTOML(c Config, localPort string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "serverAddr = %q\n", c.ServerAddr)
	fmt.Fprintf(&b, "serverPort = %d\n", c.ServerPort)
	if c.Token != "" {
		fmt.Fprintf(&b, "auth.method = \"token\"\nauth.token = %q\n", c.Token)
	}
	b.WriteString("\n[[proxies]]\n")
	b.WriteString("name = \"wede\"\n")
	fmt.Fprintf(&b, "type = %q\n", string(c.Mode))
	b.WriteString("localIP = \"127.0.0.1\"\n")
	fmt.Fprintf(&b, "localPort = %s\n", localPort)
	switch c.Mode {
	case ModeHTTP:
		fmt.Fprintf(&b, "customDomains = [%q]\n", c.Domain)
	case ModeTCP:
		fmt.Fprintf(&b, "remotePort = %d\n", c.RemotePort)
	}
	return b.String()
}

// PublicURL computes the URL the tunnel exposes. Pure.
func PublicURL(c Config) string {
	scheme := "http"
	if c.HTTPS {
		scheme = "https"
	}
	switch c.Mode {
	case ModeHTTP:
		if c.Domain == "" {
			return ""
		}
		return scheme + "://" + c.Domain
	case ModeTCP:
		if c.ServerAddr == "" || c.RemotePort <= 0 {
			return ""
		}
		return fmt.Sprintf("%s://%s:%d", scheme, c.ServerAddr, c.RemotePort)
	}
	return ""
}

// statusFromLine inspects an frpc output line and returns an updated status (or
// the current one if the line is not significant). Pure — the heart of live
// status reporting. frpc keeps the process alive and retries, so we read its log
// rather than the process state to know if the tunnel is actually up.
func statusFromLine(line string, cur Status) Status {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "login to server success"),
		strings.Contains(l, "start proxy success"):
		return StatusConnected
	case strings.Contains(l, "login to server failed"),
		strings.Contains(l, "connect to server error"),
		strings.Contains(l, "authentication failed"),
		strings.Contains(l, "proxy"+" "+"already exists"):
		return StatusError
	}
	return cur
}

// Start launches frpc with a freshly-generated config. No-op if already running.
func (m *Manager) Start() error {
	path, ok := FrpcPath()
	if !ok {
		return fmt.Errorf("tunnel: frpc not found on PATH — install frp (https://github.com/fatedier/frp)")
	}

	m.mu.Lock()
	if m.cmd != nil {
		m.mu.Unlock()
		return nil // already running
	}
	cfg := m.cfg
	m.mu.Unlock()

	if err := validateConfig(cfg); err != nil {
		return err
	}

	os.MkdirAll(m.dataDir, 0o700)
	if err := os.WriteFile(m.frpcToml(), []byte(renderTOML(cfg, m.localPort)), 0o600); err != nil {
		return fmt.Errorf("tunnel: write config: %w", err)
	}

	cmd := exec.Command(path, "-c", m.frpcToml())
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("tunnel: start frpc: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.status = StatusStarting
	m.stopping = false
	m.startedAt = time.Now()
	m.log = nil
	m.mu.Unlock()

	go m.consume(io.MultiReader(stdout, stderr))
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		m.cmd = nil
		if m.stopping {
			m.status = StatusStopped
		} else if m.status != StatusError {
			m.status = StatusStopped // exited on its own
		}
		m.mu.Unlock()
	}()
	return nil
}

// consume reads frpc output, keeps a bounded tail, and updates status.
func (m *Manager) consume(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		m.mu.Lock()
		m.log = append(m.log, line)
		if len(m.log) > maxLogLines {
			m.log = m.log[len(m.log)-maxLogLines:]
		}
		m.status = statusFromLine(line, m.status)
		m.mu.Unlock()
	}
}

// Stop terminates frpc. No-op if not running.
func (m *Manager) Stop() error {
	m.mu.Lock()
	cmd := m.cmd
	if cmd == nil {
		m.status = StatusStopped
		m.mu.Unlock()
		return nil
	}
	m.stopping = true
	m.mu.Unlock()

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	// Give Wait() a moment to flip status.
	time.Sleep(50 * time.Millisecond)
	m.mu.Lock()
	m.status = StatusStopped
	m.cmd = nil
	m.mu.Unlock()
	return nil
}

// Close stops the tunnel (called on shutdown).
func (m *Manager) Close() { _ = m.Stop() }

// State is the JSON snapshot returned to the owner UI (token redacted).
type State struct {
	Detected  bool     `json:"detected"`
	FrpcPath  string   `json:"frpcPath,omitempty"`
	Status    Status   `json:"status"`
	PublicURL string   `json:"publicUrl,omitempty"`
	Config    Config   `json:"config"`
	Log       []string `json:"log,omitempty"`
}

// Snapshot returns the current state. The relay token is redacted.
func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, detected := FrpcPath()
	cfg := m.cfg
	if cfg.Token != "" {
		cfg.Token = "" // never leak the token back to the client
	}
	logCopy := make([]string, len(m.log))
	copy(logCopy, m.log)
	url := ""
	if m.status == StatusConnected {
		url = PublicURL(m.cfg)
	}
	return State{
		Detected:  detected,
		FrpcPath:  path,
		Status:    m.status,
		PublicURL: url,
		Config:    cfg,
		Log:       logCopy,
	}
}
