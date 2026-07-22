package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// resolve.go — SMART-AUTOSCALE (agent side): the ROUTING HOOK.
//
// A managed agent does not hard-code which relay PoP to dial. Instead it asks a
// DIRECTORY (the CP, or any endpoint implementing the contract) for its assigned
// PoP — nearest region + least-loaded — and dials THAT, both on first connect and
// on every reconnect. This is what makes a graceful drain migrate cleanly: when a
// PoP drains, the CP stops handing it out, so the agent's next resolve returns a
// DIFFERENT PoP and the tunnel moves there.
//
// CP-OPTIONAL: an agent with no resolver configured dials its static ServerURL
// exactly as before (self-host / single relay). The resolver is purely additive.
//
// ── directory contract (agent → CP) ─────────────────────────────────────────
//
//	GET {directory}/api/relay/assign?name=<name>[&region=<pref>]
//	  headers: Authorization: Bearer <token>
//	  → 200 {"endpoint":"wss://hel1.relay.vulos.org","region":"eu-central","pop_id":"hel1-a"}
//
// The CP computes the assignment from its PoP directory, which is fed by the PoPs'
// registration + load heartbeats (see the relay's poplink.go) — closing the loop:
// heartbeats → CP placement → agent dials assigned PoP → drain re-resolves → new PoP.

// Assignment is the PoP a directory assigns to an agent.
type Assignment struct {
	Endpoint string `json:"endpoint"` // wss:// (or ws://) base URL of the assigned PoP
	Region   string `json:"region"`
	PoPID    string `json:"pop_id"`
}

// PoPResolver yields the PoP an agent should dial for its name. Implementations
// must be safe for concurrent use. The default is httpResolver; tests inject a fake.
type PoPResolver interface {
	Resolve(ctx context.Context, name string) (Assignment, error)
}

// httpResolver queries a directory endpoint (the CP) for the assigned PoP.
type httpResolver struct {
	directoryURL string
	token        string
	region       string // preferred-region hint sent to the directory
	client       *http.Client
}

// newHTTPResolver builds the default directory resolver. A blank directoryURL
// yields nil (no resolver — the agent falls back to its static ServerURL).
func newHTTPResolver(directoryURL, token, region string) PoPResolver {
	if strings.TrimSpace(directoryURL) == "" {
		return nil
	}
	return &httpResolver{
		directoryURL: strings.TrimRight(strings.TrimSpace(directoryURL), "/"),
		token:        token,
		region:       region,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Resolve asks the directory for the PoP assigned to name.
func (h *httpResolver) Resolve(ctx context.Context, name string) (Assignment, error) {
	u := h.directoryURL + "/api/relay/assign?name=" + url.QueryEscape(name)
	if h.region != "" {
		u += "&region=" + url.QueryEscape(h.region)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Assignment{}, err
	}
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return Assignment{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return Assignment{}, fmt.Errorf("directory assign status %d", resp.StatusCode)
	}
	var a Assignment
	if err := json.Unmarshal(body, &a); err != nil {
		return Assignment{}, fmt.Errorf("directory assign decode: %w", err)
	}
	if strings.TrimSpace(a.Endpoint) == "" {
		return Assignment{}, fmt.Errorf("directory returned no endpoint")
	}
	return a, nil
}

// resolveEndpoint returns the relay endpoint the agent should dial next. It queries
// the resolver (routing hook) when one is configured, falling back to the static
// ServerURL on any resolver error so a transient directory blip never strands the
// agent. With no resolver it always returns ServerURL. It also records the assigned
// PoP id/region for Snapshot.
func (a *Agent) resolveEndpoint(ctx context.Context) string {
	if a.resolver == nil {
		return a.opts.ServerURL
	}
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	asg, err := a.resolver.Resolve(rctx, a.opts.Name)
	if err != nil || strings.TrimSpace(asg.Endpoint) == "" {
		if err != nil {
			a.appendLog("PoP resolve failed (%v); falling back to %s", err, a.opts.ServerURL)
		}
		a.setAssignment("", "")
		return a.opts.ServerURL
	}
	a.setAssignment(asg.PoPID, asg.Region)
	a.appendLog("assigned PoP %s region=%s endpoint=%s", asg.PoPID, asg.Region, asg.Endpoint)
	return asg.Endpoint
}
