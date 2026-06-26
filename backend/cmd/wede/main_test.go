package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"wede/backend/internal/config"
)

// okHandler is a trivial next-handler used to exercise the middleware.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestSecurityHeadersDefault verifies the standalone default denies all
// cross-origin framing.
func TestSecurityHeadersDefault(t *testing.T) {
	cfg := &config.Config{FrameAncestors: ""}
	h := securityHeaders(cfg, okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	res := rec.Result()
	if got := res.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); got != "frame-ancestors 'self'" {
		t.Errorf("CSP = %q, want frame-ancestors 'self'", got)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

// TestSecurityHeadersEmbed verifies that configuring FrameAncestors switches to
// a CSP-only policy (no X-Frame-Options) so trusted origins can embed wede.
func TestSecurityHeadersEmbed(t *testing.T) {
	cfg := &config.Config{FrameAncestors: "https://vulos.org"}
	h := securityHeaders(cfg, okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	res := rec.Result()
	if got := res.Header.Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options = %q, want empty in embed mode", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); got != "frame-ancestors https://vulos.org" {
		t.Errorf("CSP = %q, want frame-ancestors https://vulos.org", got)
	}
}
