package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// staticWSF implements WorkspaceProvider for format tests.
// Format doesn't need a workspace, but the Handler requires one.
type staticWSF struct{}

func (s *staticWSF) Current() string { return "" }

func newFormatHandler() *Handler {
	return New(&staticWSF{})
}

func postFormatBody(t *testing.T, path, content string) *bytes.Reader {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"path": path, "content": content})
	return bytes.NewReader(data)
}

// TestFormat_GoFmt verifies that gofmt is applied when available.
func TestFormat_GoFmt(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not available")
	}

	h := newFormatHandler()

	unformatted := "package main\nfunc main(){println(\"hello\")}\n"
	req := httptest.NewRequest(http.MethodPost, "/api/files/format",
		postFormatBody(t, "main.go", unformatted))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Format(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Format: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Content   string `json:"content"`
		Formatted bool   `json:"formatted"`
		Error     string `json:"error"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Formatted {
		t.Errorf("expected formatted=true, got false; error=%q", resp.Error)
	}
	if resp.Content == "" {
		t.Error("expected non-empty formatted content")
	}
	// gofmt should add spacing; the output should differ from the input
	// (or at minimum be valid Go).
	if !strings.Contains(resp.Content, "package main") {
		t.Error("formatted output doesn't look like valid Go")
	}
}

// TestFormat_GoFmt_BadSyntax verifies that gofmt syntax errors return
// formatted=false without a 500.
func TestFormat_GoFmt_BadSyntax(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not available")
	}

	h := newFormatHandler()

	// Intentionally invalid Go syntax.
	badGo := "package main\nfunc main( { }\n"
	req := httptest.NewRequest(http.MethodPost, "/api/files/format",
		postFormatBody(t, "bad.go", badGo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Format(rec, req)

	if rec.Code >= 500 {
		t.Fatalf("Format with bad Go syntax returned HTTP %d — expected 200 with formatted=false", rec.Code)
	}
	var resp struct {
		Content   string `json:"content"`
		Formatted bool   `json:"formatted"`
		Error     string `json:"error"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Formatted {
		t.Error("expected formatted=false for bad syntax")
	}
	if resp.Content != badGo {
		t.Errorf("expected original content returned, got %q", resp.Content)
	}
}

// TestFormat_UnknownExtension verifies that an unrecognised extension returns
// formatted=false gracefully without a 500.
func TestFormat_UnknownExtension(t *testing.T) {
	h := newFormatHandler()

	content := "hello world"
	req := httptest.NewRequest(http.MethodPost, "/api/files/format",
		postFormatBody(t, "data.xyz", content))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Format(rec, req)

	if rec.Code >= 500 {
		t.Fatalf("Format for .xyz returned HTTP %d — expected 200 with formatted=false", rec.Code)
	}
	var resp struct {
		Content   string `json:"content"`
		Formatted bool   `json:"formatted"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Formatted {
		t.Error("expected formatted=false for unknown extension")
	}
	if resp.Content != content {
		t.Errorf("expected original content returned, got %q", resp.Content)
	}
}

// TestFormat_MissingPath verifies that omitting the path returns 400.
func TestFormat_MissingPath(t *testing.T) {
	h := newFormatHandler()

	data, _ := json.Marshal(map[string]string{"content": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/files/format", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Format(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing path: expected 400, got %d", rec.Code)
	}
}
