// Package agent is the box-side half of the Vulos sovereign reverse tunnel.
//
// An Agent dials a SINGLE outbound wss:// connection to a Vulos relay server,
// authenticates with a bearer token, claims its token-authorized name, then hands
// the connection to yamux and services inbound requests: for each yamux stream the
// server opens, the agent reads one HTTP request and proxies it to its ONE
// configured local target (localhost:PORT) — never an arbitrary host (SSRF guard).
//
// The public surface intentionally mirrors wede's internal/tunnel.Manager so wede
// can swap its frp subprocess for this in-process client:
//
//	Status vocabulary: "stopped" | "starting" | "connected" | "error"
//	Start(ctx) / Stop() / PublicURL() / Snapshot()
//
// Start returns immediately after kicking off an async dial+maintain loop that
// reconnects with exponential backoff + jitter; it never blocks on the network.
package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Status is the tunnel's lifecycle state. Values match wede's tunnel.Status.
type Status string

const (
	StatusStopped   Status = "stopped"
	StatusStarting  Status = "starting"
	StatusConnected Status = "connected"
	StatusError     Status = "error"
)

const maxLogLines = 200

// Options configures an Agent.
type Options struct {
	// ServerURL is the relay's control endpoint base, e.g. "wss://relay.vulos.dev"
	// or "https://relay.vulos.dev" (http/https are normalized to ws/wss). The
	// control path is appended automatically.
	ServerURL string
	// Token is the per-agent bearer token; the server validates it and derives the
	// permitted name(s) from it. Owner-only secret.
	Token string
	// LocalAddr is the ONE local target to proxy to, e.g. "127.0.0.1:8080" or
	// "localhost:8080". Must resolve to loopback; non-local targets are refused.
	LocalAddr string
	// Name is the requested public name (subdomain / path segment). The server only
	// honors it if the token authorizes it.
	Name string

	// SMART-AUTOSCALE routing hook. DirectoryURL, when set, is the CP/directory base
	// URL the agent queries for its assigned PoP (nearest + least-loaded) before each
	// connect AND reconnect — so a graceful drain migrates the tunnel to a fresh PoP.
	// Empty => the agent dials ServerURL statically (self-host / single relay). Region
	// is an optional preferred-region hint passed to the directory.
	DirectoryURL string
	Region       string
	// Resolver overrides the PoP resolver (tests inject a fake). nil => a default
	// HTTP resolver is built from DirectoryURL (and is itself nil when DirectoryURL
	// is empty, i.e. static ServerURL dialing).
	Resolver PoPResolver

	// DirectEndpoint (DIRECT-IP) is an OPTIONAL public https:// base URL at which
	// this box is ALSO directly reachable (static IP / public hostname). When set,
	// the agent advertises it to the relay, which independently verifies it
	// (reachable + ownership-proven) before surfacing it to clients as a faster
	// direct fast-path. Leave empty for the pure-relay path (NAT'd/CGNAT boxes).
	// Advertising it here does NOT bypass the relay's verification — an endpoint the
	// relay cannot reach or prove the box owns is silently dropped and the box stays
	// on the relay tunnel.
	DirectEndpoint string

	// TLSConfig, if set, is used for the wss dial (e.g. a pinned CA for self-hosted
	// relays). nil uses the system roots.
	TLSConfig *tls.Config
	// InsecureSkipVerify disables TLS verification — for local testing only.
	InsecureSkipVerify bool

	// HandshakeTimeout bounds the control dial + register round-trip. 0 -> 15s.
	HandshakeTimeout time.Duration
	// MaxBackoff caps the reconnect backoff. 0 -> 30s.
	MaxBackoff time.Duration
	// ReconnectJitter is the STAGGER WINDOW over which a SIGNALED reconnect (a relay
	// drain telling every agent to re-dial, or a retryable "at capacity / saturated"
	// refusal) is randomly delayed, so a mass reconnect of N agents does NOT
	// thundering-herd the target PoP — the reconnects are spread uniformly across
	// [0, ReconnectJitter). Make-before-break keeps the OLD tunnel up during the
	// wait, so the stagger costs no connectivity (it must stay < the handoff window).
	// 0 -> 3s. A negative value disables the stagger (reconnect immediately). This is
	// distinct from MaxBackoff, which governs RETRY backoff after a FAILED dial.
	ReconnectJitter time.Duration
	// now is injectable for tests; nil -> time.Now.
	now func() time.Time
}

// Snapshot is the state a UI reads. Field set mirrors wede's expectations.
type Snapshot struct {
	Status    Status   `json:"status"`
	PublicURL string   `json:"publicUrl,omitempty"`
	Connected bool     `json:"connected"`
	LastError string   `json:"lastError,omitempty"`
	Log       []string `json:"log,omitempty"`

	// DIRECT-IP: the box's VERIFIED direct endpoint as confirmed by the relay this
	// session, or "" when the box advertised none or verification failed. A UI can
	// surface "direct fast-path active" vs "relay only". DirectError carries a short
	// non-fatal reason when an advertised endpoint was rejected (e.g. "unreachable").
	DirectEndpoint string `json:"directEndpoint,omitempty"`
	DirectVerified bool   `json:"directVerified,omitempty"`
	DirectError    string `json:"directError,omitempty"`

	// SMART-AUTOSCALE: the PoP the directory assigned for the current session (empty
	// when dialing a static ServerURL). A UI can surface which region/PoP is serving.
	AssignedPoP    string `json:"assignedPoP,omitempty"`
	AssignedRegion string `json:"assignedRegion,omitempty"`
}

// Agent maintains one outbound tunnel. Safe for concurrent use.
type Agent struct {
	opts Options

	mu        sync.Mutex
	status    Status
	publicURL string
	lastErr   string
	log       []string
	cancel    context.CancelFunc
	running   bool
	// DIRECT-IP: the relay-confirmed direct endpoint for the current session (and
	// the last non-fatal rejection reason). Reset each connect.
	directEndpoint string
	directVerified bool
	directErr      string

	// SMART-AUTOSCALE. resolver is the PoP routing hook (nil => static ServerURL).
	// reconnectReq carries a PROACTIVE graceful-reconnect request from a relay drain
	// signal to the maintain loop (buffered 1 so repeats coalesce). handoffWG tracks
	// make-before-break successor goroutines so Stop waits for them. assignedPoP /
	// assignedRegion are the directory's current assignment, surfaced in Snapshot.
	resolver       PoPResolver
	reconnectReq   chan string
	handoffWG      sync.WaitGroup
	assignedPoP    string
	assignedRegion string

	// dialHook lets tests replace the wss dial with an in-memory net.Conn. When set,
	// it is passed the resolved endpoint so a test can route by assignment.
	dialHook func(ctx context.Context, endpoint string) (net.Conn, error)
}

// New returns an idle Agent. Call Start to bring the tunnel up.
func New(opts Options) *Agent {
	if opts.now == nil {
		opts.now = time.Now
	}
	if opts.HandshakeTimeout == 0 {
		opts.HandshakeTimeout = 15 * time.Second
	}
	if opts.MaxBackoff == 0 {
		opts.MaxBackoff = 30 * time.Second
	}
	// ReconnectJitter: 0 => default 3s stagger window; a negative value disables the
	// stagger (mirrors the "0=default, <0=off" convention used across the relay).
	switch {
	case opts.ReconnectJitter < 0:
		opts.ReconnectJitter = 0
	case opts.ReconnectJitter == 0:
		opts.ReconnectJitter = 3 * time.Second
	}
	// SMART-AUTOSCALE: wire the PoP resolver — an explicit one if injected, else a
	// default HTTP resolver built from DirectoryURL (nil when DirectoryURL is empty,
	// which keeps the static-ServerURL behavior).
	resolver := opts.Resolver
	if resolver == nil {
		resolver = newHTTPResolver(opts.DirectoryURL, opts.Token, opts.Region)
	}
	return &Agent{
		opts:         opts,
		status:       StatusStopped,
		resolver:     resolver,
		reconnectReq: make(chan string, 1),
	}
}

// Start validates options and launches the async dial+maintain loop. It returns
// quickly; connection progress is observable via Snapshot. Calling Start on an
// already-running Agent is a no-op.
func (a *Agent) Start(ctx context.Context) error {
	if err := validateOptions(a.opts); err != nil {
		return err
	}
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return nil
	}
	loopCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.running = true
	a.status = StatusStarting
	a.lastErr = ""
	a.log = nil
	a.mu.Unlock()

	a.appendLog("starting: dialing %s for name %q -> %s", a.opts.ServerURL, a.opts.Name, a.opts.LocalAddr)
	go a.maintain(loopCtx)
	return nil
}

// Stop tears down the tunnel and blocks briefly for the loop to observe cancel.
// No-op if not running.
func (a *Agent) Stop() {
	a.mu.Lock()
	cancel := a.cancel
	a.running = false
	a.cancel = nil
	a.status = StatusStopped
	a.publicURL = ""
	a.directEndpoint = ""
	a.directVerified = false
	a.directErr = ""
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Wait for any make-before-break successor goroutines to observe the cancel and
	// exit, so Stop leaves no tunnel goroutine running. Bounded: successors watch ctx.
	a.handoffWG.Wait()
}

// PublicURL returns the current public URL, or "" if not connected.
func (a *Agent) PublicURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != StatusConnected {
		return ""
	}
	return a.publicURL
}

// Snapshot returns the current observable state. The token is never included.
func (a *Agent) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	logCopy := make([]string, len(a.log))
	copy(logCopy, a.log)
	url := ""
	if a.status == StatusConnected {
		url = a.publicURL
	}
	return Snapshot{
		Status:         a.status,
		PublicURL:      url,
		Connected:      a.status == StatusConnected,
		LastError:      a.lastErr,
		Log:            logCopy,
		DirectEndpoint: a.directEndpoint,
		DirectVerified: a.directVerified,
		DirectError:    a.directErr,
		AssignedPoP:    a.assignedPoP,
		AssignedRegion: a.assignedRegion,
	}
}

// setAssignment records the directory's current PoP assignment for Snapshot.
func (a *Agent) setAssignment(pop, region string) {
	a.mu.Lock()
	a.assignedPoP = pop
	a.assignedRegion = region
	a.mu.Unlock()
}

// requestReconnect asks the maintain loop to gracefully migrate to the agent's
// (re-resolved) assigned PoP. Non-blocking + coalescing: a second request while one
// is already pending is dropped (buffered channel of 1), so a burst of drain signals
// triggers exactly one migration.
func (a *Agent) requestReconnect(reason string) {
	select {
	case a.reconnectReq <- reason:
	default:
	}
}

// --- internals ---

func validateOptions(o Options) error {
	if strings.TrimSpace(o.ServerURL) == "" {
		return fmt.Errorf("agent: ServerURL is required")
	}
	if strings.TrimSpace(o.Token) == "" {
		return fmt.Errorf("agent: Token is required")
	}
	if strings.TrimSpace(o.Name) == "" {
		return fmt.Errorf("agent: Name is required")
	}
	if strings.TrimSpace(o.LocalAddr) == "" {
		return fmt.Errorf("agent: LocalAddr is required")
	}
	if err := ensureLoopback(o.LocalAddr); err != nil {
		return err
	}
	return nil
}

// ensureLoopback rejects any LocalAddr that is not a loopback host. This is the
// agent's SSRF guard, checked at configuration time AND re-checked in serveStream
// before every dial (defence in depth — the check is cheap and the target must
// never drift off-loopback). It accepts ONLY the literal host "localhost" or a
// literal loopback IP (127.0.0.0/8, ::1); every other hostname is rejected rather
// than resolved, so there is no name-resolution step an attacker could steer
// (no DNS-rebinding surface). "localhost" is dialed by name and resolved by the
// OS at dial time; on a sane host that is a loopback address, and the target is
// static operator configuration, never request-influenced.
func ensureLoopback(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("agent: LocalAddr must be host:port: %w", err)
	}
	if strings.TrimSpace(port) == "" {
		return fmt.Errorf("agent: LocalAddr must include a port")
	}
	host = strings.TrimSpace(host)
	switch strings.ToLower(host) {
	case "localhost":
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("agent: LocalAddr host %q must be localhost or a loopback IP", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("agent: LocalAddr %q is not loopback (SSRF guard)", host)
	}
	return nil
}

// controlURL normalizes ServerURL to a wss/ws URL with the control path.
func controlURL(serverURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil {
		return "", fmt.Errorf("agent: invalid ServerURL: %w", err)
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("agent: ServerURL scheme %q must be wss/https (or ws/http for testing)", u.Scheme)
	}
	// Preserve any base path the operator put in front of the relay.
	u.Path = strings.TrimRight(u.Path, "/") + controlPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (a *Agent) setStatus(s Status, publicURL, errMsg string) {
	a.mu.Lock()
	a.status = s
	if publicURL != "" {
		a.publicURL = publicURL
	}
	if errMsg != "" {
		a.lastErr = errMsg
	}
	if s == StatusConnected {
		a.lastErr = ""
	}
	a.mu.Unlock()
}

// setDirectResult records the relay's verdict on the box's advertised direct
// endpoint for the current session (DIRECT-IP). Observable via Snapshot.
func (a *Agent) setDirectResult(endpoint string, verified bool, errMsg string) {
	a.mu.Lock()
	a.directEndpoint = endpoint
	a.directVerified = verified
	a.directErr = errMsg
	a.mu.Unlock()
}

func (a *Agent) appendLog(format string, args ...any) {
	line := fmt.Sprintf("%s "+format, append([]any{a.opts.now().Format(time.RFC3339)}, args...)...)
	a.mu.Lock()
	a.log = append(a.log, line)
	if len(a.log) > maxLogLines {
		a.log = a.log[len(a.log)-maxLogLines:]
	}
	a.mu.Unlock()
}
