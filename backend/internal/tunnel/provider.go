package tunnel

// Provider is the seam for the underlying tunnel mechanism. Manager (in
// tunnel.go) drives a Provider but never names a concrete implementation —
// that keeps the package's public surface (Tunnel, Manager, Config, Status)
// entirely mechanism-agnostic.
//
// The shipped default (provider_relay.go, always compiled in) wraps the
// embedded Vulos Relay agent. It is the default because it's the one Vulos
// ships and tests end-to-end, not because it's privileged in any way: an
// alternate Provider (Cloudflare Tunnel, ngrok, frp, Tailscale Funnel, a test
// fake, ...) can implement this interface and be wired in via
// NewWithProvider without touching Manager, main.go, or the HTTP handlers.
// See also docs/PUBLIC-ACCESS.md for reaching wede publicly without any
// Provider at all (direct bind + reverse proxy).
type Provider interface {
	// Start begins connecting/dialing. It should return promptly — providers
	// that dial asynchronously (like the relay agent) report progress via
	// Snapshot rather than blocking Start until connected.
	Start() error
	// Stop tears the tunnel down. Must be safe to call on an already-stopped
	// provider.
	Stop()
	// PublicURL returns the current public URL, or "" if not connected.
	PublicURL() string
	// Snapshot returns the provider's current status for the owner UI.
	Snapshot() ProviderSnapshot
}

// ProviderOptions carries what a Provider needs to start: the owner's
// persisted Config plus wede's own loopback listen address (the ONE local
// target it may ever proxy to).
type ProviderOptions struct {
	ServerURL string
	Token     string
	Name      string
	// LocalAddr is wede's own loopback listen address, e.g. "127.0.0.1:9090".
	// A well-behaved Provider must refuse to proxy anywhere else.
	LocalAddr string
}

// ProviderSnapshot is a Provider's current status, translated into Manager's
// State by Snapshot().
type ProviderSnapshot struct {
	Status    Status
	Connected bool
	PublicURL string
	Log       []string
}

// ProviderFactory constructs a Provider from ProviderOptions. DefaultProviderFactory
// (provider_relay.go) is the Vulos Relay agent; pass a different factory to
// NewWithProvider to use something else.
type ProviderFactory func(ProviderOptions) Provider
