package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ── readLSPMessage ────────────────────────────────────────────────────────────

func TestReadLSPMessage_basic(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"initialized"}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	r := bufio.NewReader(strings.NewReader(raw))
	got, err := readLSPMessage(r)
	if err != nil {
		t.Fatalf("readLSPMessage error: %v", err)
	}
	if string(got) != body {
		t.Errorf("got %q, want %q", got, body)
	}
}

func TestReadLSPMessage_extraHeaders(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{}}`
	raw := fmt.Sprintf("Content-Length: %d\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n%s", len(body), body)
	r := bufio.NewReader(strings.NewReader(raw))
	got, err := readLSPMessage(r)
	if err != nil {
		t.Fatalf("readLSPMessage error: %v", err)
	}
	if string(got) != body {
		t.Errorf("got %q, want %q", got, body)
	}
}

func TestReadLSPMessage_missingContentLength(t *testing.T) {
	raw := "\r\n{}"
	r := bufio.NewReader(strings.NewReader(raw))
	_, err := readLSPMessage(r)
	if err == nil {
		t.Error("expected error for missing Content-Length, got nil")
	}
}

func TestReadLSPMessage_invalidContentLength(t *testing.T) {
	raw := "Content-Length: abc\r\n\r\n{}"
	r := bufio.NewReader(strings.NewReader(raw))
	_, err := readLSPMessage(r)
	if err == nil {
		t.Error("expected error for non-numeric Content-Length, got nil")
	}
}

func TestReadLSPMessage_eof(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))
	_, err := readLSPMessage(r)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadLSPMessage_multipleMessages(t *testing.T) {
	msg1 := `{"jsonrpc":"2.0","method":"initialized"}`
	msg2 := `{"jsonrpc":"2.0","id":1,"method":"textDocument/hover"}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%sContent-Length: %d\r\n\r\n%s",
		len(msg1), msg1, len(msg2), msg2)
	r := bufio.NewReader(strings.NewReader(raw))

	got1, err := readLSPMessage(r)
	if err != nil || string(got1) != msg1 {
		t.Errorf("msg1: got (%q, %v), want (%q, nil)", got1, err, msg1)
	}
	got2, err := readLSPMessage(r)
	if err != nil || string(got2) != msg2 {
		t.Errorf("msg2: got (%q, %v), want (%q, nil)", got2, err, msg2)
	}
}

// ── parseOrigins / checkOrigin ────────────────────────────────────────────────

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
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/lsp", nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestCheckOrigin(t *testing.T) {
	allowed := parseOrigins("https://vulos.org")
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"no origin header", "", "localhost:9090", true},
		{"same-origin http", "http://localhost:9090", "localhost:9090", true},
		{"allowed origin", "https://vulos.org", "localhost:9090", true},
		{"attacker", "https://evil.com", "localhost:9090", false},
		{"partial match", "https://vulos.org.evil.com", "localhost:9090", false},
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

// ── AvailableServers ──────────────────────────────────────────────────────────

func TestAvailableServers_returnsMap(t *testing.T) {
	// AvailableServers should never panic and should return a non-nil map.
	// On a CI machine with no language servers installed, the result will be
	// empty — that is correct behaviour.
	avail := AvailableServers()
	if avail == nil {
		t.Error("AvailableServers returned nil")
	}
	// All values must be non-empty strings (absolute paths from LookPath).
	for lang, path := range avail {
		if path == "" {
			t.Errorf("AvailableServers[%q] = empty path", lang)
		}
	}
}

// ── proxyWSToServer message framing ──────────────────────────────────────────

// writerRecorder captures writes to verify the Content-Length framing.
type writerRecorder struct {
	buf bytes.Buffer
}

func (w *writerRecorder) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *writerRecorder) Close() error                 { return nil }

func TestContentLengthFraming(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"initialized"}`)
	rec := &writerRecorder{}
	proc := &serverProc{stdin: rec}

	// Simulate what proxyWSToServer does for one message.
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	proc.mu.Lock()
	proc.stdin.Write([]byte(header)) //nolint:errcheck
	proc.stdin.Write(body)           //nolint:errcheck
	proc.mu.Unlock()

	written := rec.buf.String()
	if !strings.HasPrefix(written, "Content-Length: ") {
		t.Errorf("expected Content-Length prefix, got %q", written)
	}
	if !strings.Contains(written, string(body)) {
		t.Errorf("body not found in written output: %q", written)
	}

	// Verify round-trip: parse the framed message back out.
	r := bufio.NewReader(strings.NewReader(written))
	got, err := readLSPMessage(r)
	if err != nil {
		t.Fatalf("round-trip readLSPMessage error: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("round-trip: got %q, want %q", got, body)
	}
}

// ── knownServers table ────────────────────────────────────────────────────────

func TestKnownServers_requiredLanguages(t *testing.T) {
	required := []string{"go", "javascript", "typescript", "python"}
	for _, lang := range required {
		if _, ok := knownServers[lang]; !ok {
			t.Errorf("knownServers missing required language %q", lang)
		}
	}
}

func TestKnownServers_noBinIsEmpty(t *testing.T) {
	for lang, ls := range knownServers {
		if ls.bin == "" {
			t.Errorf("knownServers[%q].bin is empty", lang)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/lsp.json"
	cfg := `{
	  "servers": {
	    "lua":  { "command": "lua-language-server", "extensions": ["lua", ".luau"] },
	    "go":   { "command": "gopls-custom", "args": ["serve"], "extensions": ["go"] },
	    "bad":  { "command": "  ", "extensions": ["x"] }
	  }
	}`
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// snapshot + restore the package registry so we don't leak into other tests
	saved := make(map[string]langServer, len(knownServers))
	for k, v := range knownServers {
		saved[k] = v
	}
	t.Cleanup(func() { knownServers = saved })

	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if ls, ok := knownServers["lua"]; !ok || ls.bin != "lua-language-server" {
		t.Fatalf("lua not registered: %+v", knownServers["lua"])
	}
	// config overrides a built-in
	if knownServers["go"].bin != "gopls-custom" {
		t.Fatalf("go override failed: %+v", knownServers["go"])
	}
	// blank command is skipped
	if _, ok := knownServers["bad"]; ok {
		t.Fatal("blank-command entry should be skipped")
	}
	// extension dot is stripped + lowercased
	exts := LanguageExtensions()
	if exts["lua"] != "lua" || exts["luau"] != "lua" {
		t.Fatalf("lua extensions wrong: %v", exts)
	}
	if exts["go"] != "go" {
		t.Fatalf("go extension wrong: %v", exts)
	}
}

func TestLoadConfig_missingFileOK(t *testing.T) {
	if err := LoadConfig(t.TempDir() + "/does-not-exist.json"); err != nil {
		t.Fatalf("missing file should be nil error, got %v", err)
	}
}
