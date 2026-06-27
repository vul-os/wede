package apiclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendProxy(t *testing.T) {
	// The proxy blocks loopback/private targets by default (SSRF guard); the test
	// server is on 127.0.0.1, so opt in to private targets for this test.
	t.Setenv("WEDE_APICLIENT_ALLOW_PRIVATE", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("header not forwarded: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	h := New(func() string { return t.TempDir() })
	body, _ := json.Marshal(map[string]any{
		"method": "POST", "url": srv.URL,
		"headers": map[string]string{"Accept": "application/json"},
		"body":    `{"x":1}`,
	})
	rec := httptest.NewRecorder()
	h.Send(rec, httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("send returned %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"].(float64) != 201 {
		t.Errorf("proxied status = %v, want 201", resp["status"])
	}
	if !strings.Contains(resp["body"].(string), "ok") {
		t.Errorf("proxied body = %v", resp["body"])
	}
	if resp["timeMs"] == nil {
		t.Error("expected timeMs in response")
	}
}

func TestSaveAndTree(t *testing.T) {
	dir := t.TempDir()
	h := New(func() string { return dir })

	reqData := `{"name":"Get tasks","method":"GET","url":"{{base}}/tasks"}`
	body, _ := json.Marshal(map[string]any{"type": "request", "path": "api/get-tasks", "request": json.RawMessage(reqData)})
	rec := httptest.NewRecorder()
	h.SaveItem(rec, httptest.NewRequest(http.MethodPut, "/item", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("SaveItem returned %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", "api", "get-tasks.json")); err != nil {
		t.Fatalf("request file not written: %v", err)
	}

	rec = httptest.NewRecorder()
	h.Tree(rec, httptest.NewRequest(http.MethodGet, "/apiclient", nil))
	if !strings.Contains(rec.Body.String(), "get-tasks") || !strings.Contains(rec.Body.String(), "Get tasks") {
		t.Errorf("tree missing saved request: %s", rec.Body.String())
	}
}

func TestSaveEnvironmentRoundtrip(t *testing.T) {
	dir := t.TempDir()
	h := New(func() string { return dir })
	body, _ := json.Marshal(map[string]any{"name": "local", "variables": map[string]string{"base": "http://localhost:8080"}})
	rec := httptest.NewRecorder()
	h.SaveEnvironment(rec, httptest.NewRequest(http.MethodPut, "/environment", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("SaveEnvironment returned %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Tree(rec, httptest.NewRequest(http.MethodGet, "/apiclient", nil))
	if !strings.Contains(rec.Body.String(), "localhost:8080") {
		t.Errorf("tree missing environment: %s", rec.Body.String())
	}
}

func TestSaveItemRejectsEscape(t *testing.T) {
	h := New(func() string { return t.TempDir() })
	// ".." escapes must be rejected; a leading "/" is stripped to a safe relative path.
	for _, p := range []string{"../../etc/passwd", "../outside", "a/../../escape"} {
		body, _ := json.Marshal(map[string]any{"type": "request", "path": p, "request": json.RawMessage(`{}`)})
		rec := httptest.NewRecorder()
		h.SaveItem(rec, httptest.NewRequest(http.MethodPut, "/item", bytes.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q should be rejected, got %d", p, rec.Code)
		}
	}
}
