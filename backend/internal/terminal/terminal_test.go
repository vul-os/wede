package terminal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseOrigins(t *testing.T) {
	cases := []struct {
		input    string
		wantKeys []string
	}{
		{"", nil},
		{"https://vulos.org", []string{"https://vulos.org"}},
		{"https://vulos.org https://app.vulos.org", []string{"https://vulos.org", "https://app.vulos.org"}},
		{"  https://vulos.org  ", []string{"https://vulos.org"}},
	}
	for _, c := range cases {
		got := parseOrigins(c.input)
		for _, key := range c.wantKeys {
			if _, ok := got[key]; !ok {
				t.Errorf("parseOrigins(%q): missing key %q", c.input, key)
			}
		}
		if len(got) != len(c.wantKeys) {
			t.Errorf("parseOrigins(%q): got %d keys, want %d", c.input, len(got), len(c.wantKeys))
		}
	}
}

func makeReq(origin, host string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/terminal", nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestCheckOrigin(t *testing.T) {
	allowed := parseOrigins("https://vulos.org https://app.vulos.org")

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		// No Origin header → non-browser client → allow
		{"no origin header", "", "localhost:9090", true},

		// Same-origin (http)
		{"same-origin http", "http://localhost:9090", "localhost:9090", true},

		// Allowed cross-origin from frame_ancestors config
		{"allowed origin vulos.org", "https://vulos.org", "localhost:9090", true},
		{"allowed origin app.vulos.org", "https://app.vulos.org", "localhost:9090", true},

		// Attacker page on different origin
		{"attacker evil.com", "https://evil.com", "localhost:9090", false},
		{"attacker subdomain", "https://sub.evil.com", "localhost:9090", false},

		// Partial match must NOT be allowed (e.g. "https://vulos.org.evil.com")
		{"partial match", "https://vulos.org.evil.com", "localhost:9090", false},

		// Origin != host
		{"different origin", "http://other.host", "localhost:9090", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := makeReq(tt.origin, tt.host)
			got := checkOrigin(r, allowed)
			if got != tt.want {
				t.Errorf("checkOrigin(origin=%q, host=%q) = %v, want %v", tt.origin, tt.host, got, tt.want)
			}
		})
	}
}

func TestCheckOriginEmptyAllowedList(t *testing.T) {
	// When no frame_ancestors are configured, only same-origin and no-origin pass.
	allowed := parseOrigins("")

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"no origin", "", "localhost:9090", true},
		{"same-origin", "http://localhost:9090", "localhost:9090", true},
		{"cross-origin", "https://vulos.org", "localhost:9090", false},
		{"attacker", "https://evil.com", "localhost:9090", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := makeReq(tt.origin, tt.host)
			got := checkOrigin(r, allowed)
			if got != tt.want {
				t.Errorf("checkOrigin(origin=%q, host=%q) = %v, want %v", tt.origin, tt.host, got, tt.want)
			}
		})
	}
}
