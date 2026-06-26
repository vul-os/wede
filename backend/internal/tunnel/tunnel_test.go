package tunnel

import (
	"strings"
	"testing"
)

func TestRenderTOML_HTTP(t *testing.T) {
	c := Config{ServerAddr: "vps.example.com", ServerPort: 7000, Token: "secret", Mode: ModeHTTP, Domain: "wede.example.com"}
	out := renderTOML(c, "9090")
	for _, want := range []string{
		`serverAddr = "vps.example.com"`,
		`serverPort = 7000`,
		`auth.token = "secret"`,
		`type = "http"`,
		`localPort = 9090`,
		`customDomains = ["wede.example.com"]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("renderTOML missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "remotePort") {
		t.Error("http config should not contain remotePort")
	}
}

func TestRenderTOML_TCP_NoToken(t *testing.T) {
	c := Config{ServerAddr: "1.2.3.4", ServerPort: 7000, Mode: ModeTCP, RemotePort: 9090}
	out := renderTOML(c, "9090")
	if !strings.Contains(out, "remotePort = 9090") || !strings.Contains(out, `type = "tcp"`) {
		t.Errorf("tcp config wrong:\n%s", out)
	}
	if strings.Contains(out, "auth.token") {
		t.Error("empty token should omit auth.token")
	}
}

func TestPublicURL(t *testing.T) {
	cases := []struct {
		c    Config
		want string
	}{
		{Config{Mode: ModeHTTP, Domain: "wede.example.com"}, "http://wede.example.com"},
		{Config{Mode: ModeHTTP, Domain: "wede.example.com", HTTPS: true}, "https://wede.example.com"},
		{Config{Mode: ModeTCP, ServerAddr: "1.2.3.4", RemotePort: 9090}, "http://1.2.3.4:9090"},
		{Config{Mode: ModeHTTP}, ""},                    // no domain
		{Config{Mode: ModeTCP, ServerAddr: "1.2.3.4"}, ""}, // no remote port
	}
	for _, c := range cases {
		if got := PublicURL(c.c); got != c.want {
			t.Errorf("PublicURL(%+v) = %q, want %q", c.c, got, c.want)
		}
	}
}

func TestStatusFromLine(t *testing.T) {
	cases := []struct {
		line string
		cur  Status
		want Status
	}{
		{"[I] login to server success", StatusStarting, StatusConnected},
		{"start proxy success", StatusStarting, StatusConnected},
		{"login to server failed: token mismatch", StatusStarting, StatusError},
		{"connect to server error", StatusConnected, StatusError},
		{"authentication failed", StatusStarting, StatusError},
		{"some unrelated log line", StatusConnected, StatusConnected}, // unchanged
	}
	for _, c := range cases {
		if got := statusFromLine(c.line, c.cur); got != c.want {
			t.Errorf("statusFromLine(%q, %s) = %s, want %s", c.line, c.cur, got, c.want)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	validHTTP := Config{ServerAddr: "x", Mode: ModeHTTP, Domain: "d"}
	validTCP := Config{ServerAddr: "x", Mode: ModeTCP, RemotePort: 1}
	if err := validateConfig(validHTTP); err != nil {
		t.Errorf("valid http rejected: %v", err)
	}
	if err := validateConfig(validTCP); err != nil {
		t.Errorf("valid tcp rejected: %v", err)
	}
	bad := []Config{
		{Mode: ModeHTTP, Domain: "d"},                  // no serverAddr
		{ServerAddr: "x", Mode: ModeHTTP},              // http, no domain
		{ServerAddr: "x", Mode: ModeTCP},               // tcp, no port
		{ServerAddr: "x", Mode: "bogus"},               // bad mode
	}
	for i, c := range bad {
		if err := validateConfig(c); err == nil {
			t.Errorf("bad config %d should be rejected", i)
		}
	}
}

func TestSnapshotRedactsToken(t *testing.T) {
	m := &Manager{cfg: Config{ServerAddr: "x", Token: "supersecret", Mode: ModeHTTP, Domain: "d"}, status: StatusStopped}
	s := m.Snapshot()
	if s.Config.Token != "" {
		t.Errorf("Snapshot leaked token: %q", s.Config.Token)
	}
}
