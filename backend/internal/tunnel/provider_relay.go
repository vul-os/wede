package tunnel

// The default Provider: wede embeds the Vulos Relay agent
// (github.com/vul-os/vulos-relay/tunnel/agent) in-process rather than
// shelling out to a third-party frp binary. It dials a single outbound
// wss:// connection to the relay server the owner runs, authenticates with a
// bearer token, claims its token-authorized public name, and proxies inbound
// requests to wede's ONE local loopback port (never an arbitrary host).
//
// This is the ONLY file in the package that imports the relay agent — it's
// isolated here so the seam (Provider, in provider.go) stays reusable if a
// different tunnel mechanism is wired in later via NewWithProvider. The
// dependency is vendored (see /vendor and go.mod's replace), so building this
// file needs no sibling vulos-relay checkout.

import (
	"context"

	"github.com/vul-os/vulos-relay/tunnel/agent"
)

// DefaultProviderFactory constructs the Vulos Relay agent as a Provider. This
// is what New() (and therefore main.go) wires in by default.
func DefaultProviderFactory(opts ProviderOptions) Provider {
	a := agent.New(agent.Options{
		ServerURL: opts.ServerURL,
		Token:     opts.Token,
		Name:      opts.Name,
		LocalAddr: opts.LocalAddr,
	})
	return &relayProvider{agent: a}
}

// relayProvider adapts *agent.Agent to the package-local Provider interface.
type relayProvider struct {
	agent *agent.Agent
}

func (r *relayProvider) Start() error { return r.agent.Start(context.Background()) }
func (r *relayProvider) Stop()        { r.agent.Stop() }
func (r *relayProvider) PublicURL() string {
	return r.agent.PublicURL()
}

func (r *relayProvider) Snapshot() ProviderSnapshot {
	s := r.agent.Snapshot()
	return ProviderSnapshot{
		Status:    Status(s.Status),
		Connected: s.Connected,
		PublicURL: s.PublicURL,
		Log:       s.Log,
	}
}
