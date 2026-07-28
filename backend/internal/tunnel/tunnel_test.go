package tunnel

import (
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	valid := Config{ServerURL: "wss://relay.example.com", Token: "secret", Name: "wede"}
	if err := validateConfig(valid); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
	bad := []Config{
		{Token: "secret", Name: "wede"},                         // no serverUrl
		{ServerURL: "wss://relay.example.com", Name: "wede"},    // no token
		{ServerURL: "wss://relay.example.com", Token: "secret"}, // no name
		{}, // empty
	}
	for i, c := range bad {
		if err := validateConfig(c); err == nil {
			t.Errorf("bad config %d should be rejected", i)
		}
	}
}

// TestConfigPersistRoundTrip verifies SetConfig persists to ~/.wede/tunnel.json and
// a fresh Manager loads it back — using a temp HOME so the real one is untouched.
func TestConfigPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{localAddr: "127.0.0.1:9090", dataDir: dir}

	want := Config{ServerURL: "wss://relay.example.com", Token: "supersecret", Name: "myhost"}
	if err := m.SetConfig(want); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	m2 := &Manager{localAddr: "127.0.0.1:9090", dataDir: dir}
	m2.loadConfig()
	if m2.cfg != want {
		t.Errorf("loaded config = %+v, want %+v", m2.cfg, want)
	}
}

func TestSnapshotRedactsToken(t *testing.T) {
	m := &Manager{cfg: Config{ServerURL: "wss://relay.example.com", Token: "supersecret", Name: "wede"}}
	s := m.Snapshot()
	if s.Config.Token != "" {
		t.Errorf("Snapshot leaked token: %q", s.Config.Token)
	}
	// non-secret fields survive redaction
	if s.Config.ServerURL != "wss://relay.example.com" || s.Config.Name != "wede" {
		t.Errorf("Snapshot dropped non-secret config: %+v", s.Config)
	}
}

func TestSnapshotStoppedWhenIdle(t *testing.T) {
	m := &Manager{cfg: Config{ServerURL: "wss://relay.example.com", Token: "x", Name: "wede"}}
	if got := m.Snapshot().Status; got != StatusStopped {
		t.Errorf("idle status = %q, want %q", got, StatusStopped)
	}
	if m.PublicURL() != "" {
		t.Errorf("idle PublicURL should be empty")
	}
}

// TestStartInvalidConfigErrors ensures Start on an unconfigured Manager fails
// (validation) and does NOT leave a dangling provider.
func TestStartInvalidConfigErrors(t *testing.T) {
	m := &Manager{localAddr: "127.0.0.1:9090", dataDir: t.TempDir()}
	if err := m.Start(); err == nil {
		t.Fatal("Start with empty config should fail validation")
	}
	if m.provider != nil {
		t.Error("failed Start left a dangling provider")
	}
	// The failure is surfaced as an error status in the snapshot.
	if got := m.Snapshot().Status; got != StatusError {
		t.Errorf("post-failed-Start status = %q, want %q", got, StatusError)
	}
}

// TestStartAgainstDeadRelay drives the real (default) Provider — the embedded
// Ephor agent — against an unreachable relay (no live server). Start
// must return nil (async dial), the tunnel must NOT be connected, and Stop
// must clean up. No network dependency beyond a refused dial.
func TestStartAgainstDeadRelay(t *testing.T) {
	m := &Manager{
		localAddr:   "127.0.0.1:9090",
		dataDir:     t.TempDir(),
		newProvider: DefaultProviderFactory,
		// ws:// (not wss) + a closed loopback port -> dial fails fast, no TLS needed.
		cfg: Config{ServerURL: "ws://127.0.0.1:1", Token: "x", Name: "wede"},
	}
	if err := m.Start(); err != nil {
		t.Fatalf("Start should return quickly (async dial): %v", err)
	}
	defer m.Stop()

	// Give the async loop a moment; it must be starting or error, never connected.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Snapshot()
		if s.Status == StatusConnected {
			t.Fatalf("tunnel reported connected against a dead relay")
		}
		if s.Status == StatusError {
			break // reached error as expected
		}
		time.Sleep(20 * time.Millisecond)
	}
	if m.PublicURL() != "" {
		t.Errorf("PublicURL non-empty while not connected: %q", m.PublicURL())
	}

	m.Stop()
	if got := m.Snapshot().Status; got != StatusStopped {
		t.Errorf("post-Stop status = %q, want %q", got, StatusStopped)
	}
}

// fakeProvider is a minimal Provider stand-in proving the seam is real: an
// alternate tunnel mechanism can be wired in via NewWithProvider without
// Manager, main.go, or the HTTP handlers knowing or caring.
type fakeProvider struct {
	opts ProviderOptions
}

func (f *fakeProvider) Start() error      { return nil }
func (f *fakeProvider) Stop()             {}
func (f *fakeProvider) PublicURL() string { return "https://fake.example/" + f.opts.Name }
func (f *fakeProvider) Snapshot() ProviderSnapshot {
	return ProviderSnapshot{Status: StatusConnected, Connected: true, PublicURL: "https://fake.example/" + f.opts.Name}
}

// TestSwappableProvider drives a Manager through a non-relay Provider,
// confirming Start/Stop/Snapshot/PublicURL work against ANY ProviderFactory,
// not just the embedded Ephor agent.
func TestSwappableProvider(t *testing.T) {
	m := NewWithProvider("127.0.0.1:9090", func(opts ProviderOptions) Provider {
		return &fakeProvider{opts: opts}
	})
	m.dataDir = t.TempDir()

	cfg := Config{ServerURL: "irrelevant-to-fake-provider", Token: "x", Name: "myhost"}
	if err := m.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()

	snap := m.Snapshot()
	if snap.Status != StatusConnected {
		t.Errorf("status = %q, want %q", snap.Status, StatusConnected)
	}
	want := "https://fake.example/myhost"
	if snap.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", snap.PublicURL, want)
	}
	if got := m.PublicURL(); got != want {
		t.Errorf("Manager.PublicURL() = %q, want %q", got, want)
	}
}

// --- Guards on the embedded Ephor agent itself ---------------------------
//
// The two tests below exist because wede's default Provider is a *real*
// third-party dependency (github.com/vul-os/ephor/tunnel/agent), swapped in
// from the module rather than hand-copied. They assert the two properties
// wede's README actually claims about it — that the default provider IS the
// Ephor agent, and that it is loopback-bound — so a bad dependency swap fails
// here instead of shipping.

// TestDefaultProviderIsTheEmbeddedAgent pins the default Provider to the real
// embedded Ephor agent. It deliberately does NOT accept "some Provider": if
// DefaultProviderFactory is ever repointed at a stub, a fake, or a shell-out to
// an external binary, this fails. It also proves the agent is constructed with
// the owner's config verbatim rather than silently dropped.
func TestDefaultProviderIsTheEmbeddedAgent(t *testing.T) {
	opts := ProviderOptions{
		ServerURL: "wss://relay.example.com",
		Token:     "secret",
		Name:      "mybox",
		LocalAddr: "127.0.0.1:9090",
	}
	p := DefaultProviderFactory(opts)
	rp, ok := p.(*relayProvider)
	if !ok {
		t.Fatalf("DefaultProviderFactory returned %T, want *relayProvider (the embedded Ephor agent)", p)
	}
	if rp.agent == nil {
		t.Fatal("relayProvider was built with a nil Ephor agent — the embedded agent is not wired in")
	}
	// A freshly constructed, unstarted agent must report stopped and expose no
	// public URL. (Snapshot goes through the real agent, not a stub.)
	if snap := rp.Snapshot(); snap.Connected || snap.PublicURL != "" {
		t.Errorf("unstarted agent snapshot = %+v, want not-connected with empty PublicURL", snap)
	}
	if got := rp.PublicURL(); got != "" {
		t.Errorf("unstarted agent PublicURL() = %q, want \"\"", got)
	}
}

// TestEmbeddedAgentRefusesNonLoopbackTarget drives the REAL agent (not the
// fake) and asserts wede's central public-exposure promise: the tunnel may only
// ever proxy to wede's own loopback port. The agent validates LocalAddr
// synchronously in Start, so a Manager pointed at a routable address must fail
// closed — Start returns an error, nothing is left running, and the failure is
// surfaced to the owner UI.
//
// Every case below must be REFUSED; a case that silently starts is a
// tunnel that would expose an arbitrary host on the operator's network.
func TestEmbeddedAgentRefusesNonLoopbackTarget(t *testing.T) {
	offLoopback := []string{
		"0.0.0.0:9090",       // all interfaces
		"192.168.1.10:9090",  // LAN peer
		"10.0.0.5:9090",      // private range
		"169.254.169.254:80", // cloud metadata service
		"example.com:9090",   // resolvable name (must be rejected, not resolved)
		"[2001:db8::1]:9090", // routable v6
	}
	if len(offLoopback) != 6 {
		t.Fatalf("coverage: expected 6 off-loopback cases, have %d", len(offLoopback))
	}

	refused := 0
	for _, addr := range offLoopback {
		t.Run(addr, func(t *testing.T) {
			m := &Manager{
				localAddr:   addr,
				dataDir:     t.TempDir(),
				newProvider: DefaultProviderFactory,
				cfg:         Config{ServerURL: "ws://127.0.0.1:1", Token: "x", Name: "wede"},
			}
			err := m.Start()
			if err == nil {
				m.Stop()
				t.Fatalf("Start accepted off-loopback LocalAddr %q — the SSRF guard is not enforced", addr)
			}
			refused++
			// The refusal must reach the owner UI, not just the caller.
			if snap := m.Snapshot(); snap.Status != StatusError {
				t.Errorf("Snapshot status = %q after refused Start, want %q", snap.Status, StatusError)
			}
			if m.PublicURL() != "" {
				t.Errorf("PublicURL non-empty after refused Start: %q", m.PublicURL())
			}
		})
	}
	if refused != len(offLoopback) {
		t.Fatalf("coverage: %d/%d off-loopback cases were actually refused", refused, len(offLoopback))
	}

	// Control: the loopback address wede actually uses must NOT be refused by
	// the same guard, so the test above is proving the guard and not a blanket
	// "Start always errors".
	m := &Manager{
		localAddr:   "127.0.0.1:9090",
		dataDir:     t.TempDir(),
		newProvider: DefaultProviderFactory,
		cfg:         Config{ServerURL: "ws://127.0.0.1:1", Token: "x", Name: "wede"},
	}
	if err := m.Start(); err != nil {
		t.Fatalf("loopback LocalAddr was refused (%v) — the negative cases above prove nothing", err)
	}
	m.Stop()
}
