// Package wire defines the small control-plane message set exchanged between a
// Vulos relay agent and the sovereign relay server over the WebSocket control
// connection, BEFORE yamux takes over the same net.Conn for request multiplexing.
//
// The handshake is a single JSON request/response:
//
//	agent  --> server : Register{Name, Token}
//	server --> agent  : RegisterAck{OK, PublicURL, Error}
//
// After a successful ack, both sides hand the connection to yamux: the server is
// the yamux *client* (it opens streams, one per inbound HTTP request); the agent
// is the yamux *server* (it accepts streams and proxies each to its one local
// target). Each stream carries a plain HTTP/1.1 request and response — no extra
// framing — so http.ReadRequest / Request.Write / http.ReadResponse can be used
// directly, which also gives us transparent WebSocket-upgrade passthrough.
package wire

// Protocol constants shared by agent and server.
const (
	// ControlPath is the server route the agent dials for control/registration.
	ControlPath = "/_vulos-relay/control"

	// Subprotocol identifies the Vulos tunnel control protocol on the wss handshake.
	Subprotocol = "vulos-relay.v1"

	// MaxControlMessage bounds a single handshake JSON message (bytes).
	MaxControlMessage = 8 << 10 // 8 KiB
)

// Register is the agent's first message: it claims a name and presents its token.
// The server treats the token as authoritative for which names are permitted; the
// Name is only honored if the token grants it.
type Register struct {
	Type  string `json:"type"` // always "register"
	Name  string `json:"name"`
	Token string `json:"token"`
	// AgentVersion is informational (for server logs); never trusted.
	AgentVersion string `json:"agentVersion,omitempty"`

	// DirectEndpoint (DIRECT-IP) is an OPTIONAL direct-connect endpoint the box
	// advertises alongside its relay tunnel: a public https:// base URL a client
	// can dial DIRECTLY (near-native latency, full bandwidth) instead of routing
	// through the relay. It is only surfaced to clients AFTER the relay has
	// independently verified it is (a) reachable from the public internet and (b)
	// actually controlled by this box — see DirectVerified in RegisterAck. An empty
	// value means "relay only" (the always-works path for NAT'd/CGNAT boxes).
	//
	// SECURITY: advertising a direct endpoint is NOT trusted on the agent's word.
	// The relay probes the endpoint over the internet and requires the box to
	// echo a one-time probe nonce, so a box cannot advertise an IP/hostname it
	// does not control to hijack another box's client traffic (endpoint-ownership
	// proof). See tunnel/server/directprobe.go.
	DirectEndpoint string `json:"directEndpoint,omitempty"`
}

// RegisterAck is the server's reply. On failure OK is false and Error carries a
// short, non-leaky reason.
type RegisterAck struct {
	Type      string `json:"type"` // always "register_ack"
	OK        bool   `json:"ok"`
	PublicURL string `json:"publicUrl,omitempty"`
	Error     string `json:"error,omitempty"`

	// Retryable, on a failed ack (OK=false), tells the agent this refusal is a
	// LOAD/CAPACITY shed — the PoP is draining, at capacity, or saturated — NOT an
	// auth/authorization failure. The agent treats it as transient: it re-resolves
	// its assigned PoP (the CP steers it elsewhere) and retries with jittered backoff,
	// rather than surfacing a hard error. A non-retryable failure (bad token, name
	// taken, entitlement denied) is terminal for that endpoint. CONNECTION-FLOOD.
	Retryable bool `json:"retryable,omitempty"`

	// DirectEndpoint (DIRECT-IP) echoes back the box's advertised direct endpoint
	// IF AND ONLY IF the relay verified it (reachable + ownership-proven). Empty
	// when the box advertised none, or when verification failed — in which case the
	// box (and its clients) transparently fall back to the relay tunnel. A box can
	// read this to learn whether its direct fast-path is live.
	DirectEndpoint string `json:"directEndpoint,omitempty"`
	// DirectVerified reports whether the relay confirmed the direct endpoint. It is
	// distinct from a non-empty DirectEndpoint only for clarity in logs/UX: when
	// true, DirectEndpoint is set; when false, DirectEndpoint is empty.
	DirectVerified bool `json:"directVerified,omitempty"`
	// DirectError is a short, non-leaky reason the direct endpoint was not accepted
	// (e.g. "unreachable", "ownership proof failed", "not https"). Advisory only —
	// the tunnel still comes up on the relay path regardless.
	DirectError string `json:"directError,omitempty"`
}

// DirectProbePath is the well-known path the relay GETs on a box's advertised
// direct endpoint to verify reachability AND ownership. The box MUST serve it on
// the SAME public TLS listener it advertises, echoing back the one-time nonce the
// relay supplies via the DirectProbeHeader (proving it received the relay's probe
// and therefore controls the endpoint). The response body is the nonce verbatim.
//
// This endpoint is UNAUTHENTICATED by design (it carries no user data — only the
// relay's own nonce is echoed) and MUST be exempt from the box's auth stack, but
// it does NOT weaken auth for any other path: it serves only the nonce echo.
const DirectProbePath = "/_vulos-direct/probe"

// DirectProbeHeader carries the one-time probe nonce from the relay to the box on
// the reachability/ownership probe, and is echoed by the box in its response.
const DirectProbeHeader = "X-Vulos-Direct-Probe"

// ──────────────────────────────────────────────────────────────────────────
// AGENT-CONTROL channel (SMART-AUTOSCALE): relay → agent control signals.
//
// The relay is the yamux CLIENT — it opens one stream per inbound public request,
// which the agent proxies to its local target. It ALSO uses the same stream
// mechanism to deliver AGENT-TERMINATED control commands (never proxied to the
// box's local app): the relay opens a stream and writes a plain HTTP request whose
// path is AgentControlPath and which carries AgentCommandHeader. The agent
// recognizes such a stream BEFORE any local dial, handles the command itself, and
// replies 200 — nothing reaches the loopback target (so the SSRF guard is
// untouched: a control stream never causes a local connection).
//
// The only command today is CommandReconnect: a PROACTIVE "re-dial your assigned
// PoP now" signal used for GRACEFUL DRAIN. When a PoP is being decommissioned the
// CP tells it to drain; the relay broadcasts CommandReconnect to every connected
// agent, each agent re-resolves its nearest/least-loaded PoP and migrates there —
// make-before-break, so a drain moves every tunnel with no dropped connectivity.
//
// TRUST: the command arrives over the agent's OWN authenticated control connection
// to a relay it dialed and trusts. The relay can already open arbitrary proxied
// streams to the box's local app; a control stream grants it no NEW capability, so
// this adds no attack surface.
const (
	// AgentControlPath is the reserved request path the relay uses for an
	// agent-terminated control command. It is never forwarded to the local target.
	AgentControlPath = "/_vulos-relay/agent-control"

	// AgentCommandHeader carries the control command name. Its presence is what
	// marks a stream as a control stream (checked before any local dial).
	AgentCommandHeader = "X-Vulos-Relay-Command"

	// AgentReasonHeader is an optional, informational reason for the command
	// (e.g. "drain") the agent may log. Never trusted for control flow.
	AgentReasonHeader = "X-Vulos-Relay-Reason"

	// CommandReconnect asks the agent to gracefully re-dial its assigned PoP now
	// (make-before-break). Used by GRACEFUL DRAIN.
	CommandReconnect = "reconnect"
)
