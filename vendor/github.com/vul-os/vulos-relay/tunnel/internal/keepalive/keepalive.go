// Package keepalive drives an ADAPTIVE, idle-aware keepalive for a tunnel's yamux
// control session, replacing yamux's built-in fixed-interval keepalive.
//
// WHY: every registered box holds one permanent control connection whose keepalive
// pings run at a fixed interval (server 10s / agent 20s) for the box's whole
// lifetime, even when the tunnel is carrying no traffic. Under the ratified relay
// cost model (direct-first, relay as metered fallback) that standing heartbeat is
// pure idle cost. This package LENGTHENS the ping interval once a session has had
// no active streams for a while and RESTORES the base interval the moment activity
// returns — cutting idle heartbeat traffic WITHOUT ever evicting the session, so
// reachability is unaffected.
//
// SAFETY: this is not session eviction. The keepalive still runs; it just runs
// slower while idle. Dead-peer detection stays BOUNDED: the worst-case time to
// notice a dead peer is the idle interval plus the session's own ping write
// timeout — a genuinely dead tunnel is always torn down within that bound, never
// left to linger. The decision logic (Adaptive.Next) is pure and deterministic;
// the Run loop is a thin driver over an injected Pinger, so no real sockets are
// needed to test the interval policy.
package keepalive

import (
	"context"
	"time"
)

// Params configures the adaptive interval policy for one session.
type Params struct {
	// Base is the ping interval while the session has recent stream activity. This
	// matches the pre-existing fixed keepalive interval, so active sessions behave
	// exactly as before.
	Base time.Duration
	// Idle is the (longer) ping interval used once the session has been idle for at
	// least IdleAfter. Must be >= Base to ever back off; if <= Base the policy never
	// lengthens (degrades safely to a fixed Base interval).
	Idle time.Duration
	// IdleAfter is how long with NO observed stream activity before a session is
	// treated as idle and backed off to the Idle interval.
	IdleAfter time.Duration
}

// normalized returns a copy with unsafe/zero values coerced to safe defaults so a
// misconfiguration can never produce a zero/negative timer interval (which would
// hot-loop) or a shorter-than-base "idle" interval.
func (p Params) normalized() Params {
	if p.Base <= 0 {
		p.Base = 10 * time.Second
	}
	if p.Idle < p.Base {
		// Never "back off" to something faster than base; treat as no-backoff.
		p.Idle = p.Base
	}
	if p.IdleAfter <= 0 {
		p.IdleAfter = 2 * time.Minute
	}
	return p
}

// Adaptive tracks the activity state needed to pick the next keepalive interval.
// It is NOT safe for concurrent use; a single Run loop owns one Adaptive.
type Adaptive struct {
	params     Params
	lastActive time.Time // last time streams were observed open (>0)
}

// NewAdaptive returns an Adaptive seeded as active-at-now, so a fresh session
// starts at the Base interval and only backs off after IdleAfter of quiet.
func NewAdaptive(params Params, now time.Time) *Adaptive {
	return &Adaptive{params: params.normalized(), lastActive: now}
}

// Next records the current stream count observed at time now and returns the
// interval to wait before the NEXT keepalive ping. It is pure given its inputs and
// the accumulated lastActive state:
//
//   - numStreams > 0  => activity observed; lastActive advances to now; Base.
//   - otherwise, if the session has been quiet for < IdleAfter => still Base.
//   - quiet for >= IdleAfter => Idle (the backed-off, longer interval).
//
// Sampling stream count only at tick boundaries is intentionally conservative: any
// stream seen at a tick keeps the session at Base, and a short-lived stream missed
// between ticks can at worst leave the interval longer for a bit — it can never
// shorten dead-peer detection below the bound, and an in-flight stream keeps the
// underlying connection exercised regardless of ping cadence.
func (a *Adaptive) Next(now time.Time, numStreams int) time.Duration {
	if numStreams > 0 {
		a.lastActive = now
	}
	if now.Sub(a.lastActive) >= a.params.IdleAfter {
		return a.params.Idle
	}
	return a.params.Base
}

// Pinger is the subset of *yamux.Session the loop needs. Ping sends a keepalive
// ping and returns an error if the peer does not respond within the session's own
// write timeout (dead-peer signal). NumStreams reports currently-open streams.
type Pinger interface {
	Ping() (time.Duration, error)
	NumStreams() int
}

// Run drives the adaptive keepalive until ctx is cancelled or a ping fails. It
// returns nil on ctx cancellation and the ping error when the peer is dead — the
// caller MUST close the session on a non-nil return so the tunnel is torn down and
// (on the agent) reconnected. now is injectable for tests (pass time.Now in prod).
//
// Run replaces yamux's own keepalive goroutine, so the yamux session it drives MUST
// be created with EnableKeepAlive=false; otherwise both would ping.
func Run(ctx context.Context, p Pinger, params Params, now func() time.Time) error {
	a := NewAdaptive(params, now())
	interval := a.params.Base
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if _, err := p.Ping(); err != nil {
				return err
			}
			interval = a.Next(now(), p.NumStreams())
			timer.Reset(interval)
		}
	}
}
