package collab

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// checkOrigin only tests r.TLS != nil, so a zero ConnectionState marks a request
// as served over TLS.
var tlsState tls.ConnectionState

func TestParseOrigins(t *testing.T) {
	got := parseOrigins("https://vulos.org  https://app.vulos.org")
	if len(got) != 2 {
		t.Fatalf("parseOrigins len = %d, want 2 (%v)", len(got), got)
	}
	if _, ok := got["https://vulos.org"]; !ok {
		t.Errorf("missing https://vulos.org in %v", got)
	}
	if _, ok := got["https://app.vulos.org"]; !ok {
		t.Errorf("missing https://app.vulos.org in %v", got)
	}
	if len(parseOrigins("")) != 0 {
		t.Errorf("parseOrigins(\"\") should be empty")
	}
}

// TestCheckOrigin locks down the collab WebSocket origin policy — the guard
// against cross-site WebSocket hijacking. A cross-origin browser must be
// rejected unless its origin is explicitly allow-listed (the Vulos OS shell's
// frame_ancestors), while same-origin and non-browser (no Origin) connect.
func TestCheckOrigin(t *testing.T) {
	allowed := parseOrigins("https://vulos.org")

	newReq := func(origin, host, xfProto string, tls bool) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/collab", nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if xfProto != "" {
			r.Header.Set("X-Forwarded-Proto", xfProto)
		}
		if tls {
			r.TLS = &tlsState
		}
		return r
	}

	cases := []struct {
		name    string
		origin  string
		host    string
		xfProto string
		tls     bool
		want    bool
	}{
		{"no origin (non-browser)", "", "example.com", "", false, true},
		{"same origin http", "http://example.com", "example.com", "", false, true},
		{"same origin https via TLS", "https://example.com", "example.com", "", true, true},
		{"same origin https via X-Forwarded-Proto", "https://example.com", "example.com", "https", false, true},
		{"allow-listed cross origin", "https://vulos.org", "example.com", "", false, true},
		{"disallowed cross origin", "https://evil.example", "example.com", "", false, false},
		{"scheme mismatch is not same-origin", "https://example.com", "example.com", "", false, false},
		{"host mismatch", "http://other.com", "example.com", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkOrigin(newReq(tc.origin, tc.host, tc.xfProto, tc.tls), allowed)
			if got != tc.want {
				t.Errorf("checkOrigin(origin=%q host=%q xfProto=%q tls=%v) = %v, want %v",
					tc.origin, tc.host, tc.xfProto, tc.tls, got, tc.want)
			}
		})
	}
}
