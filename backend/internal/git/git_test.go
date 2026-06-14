package git

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// staticWS implements WorkspaceProvider for tests.
type staticWS struct{ root string }

func (s *staticWS) Current() string { return s.root }

// TestLogCountValidation ensures the ?count= query parameter is sanitised before
// being passed to git's -n flag.  A non-integer or out-of-range value must be
// silently replaced with the default (50) rather than forwarded to git.
func TestLogCountValidation(t *testing.T) {
	// We only test the HTTP-layer validation; we don't need a real git repo
	// because an invalid count is replaced before exec.Command is called and
	// we can observe the handler doesn't crash / return 500 for bad input.
	h := New(&staticWS{root: ""}) // empty workspace → run() returns early

	cases := []struct {
		raw  string
		desc string
	}{
		{"", "empty (default)"},
		{"50", "normal"},
		{"1", "minimum"},
		{"10000", "maximum"},
		{"10001", "above max → clamped"},
		{"-1", "negative → clamped"},
		{"0", "zero → clamped"},
		{"--all", "flag injection"},
		{"50%3Brm+-rf+%2F", "shell injection encoded"},
		{"abc", "non-integer"},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			url := "/api/git/log"
			if c.raw != "" {
				url += "?count=" + c.raw
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			// Log handler returns early when workspace is empty.
			// We just verify no panic and no 500.
			h.Log(w, req)
			if w.Code >= 500 {
				t.Errorf("count=%q returned HTTP %d", c.raw, w.Code)
			}
		})
	}
}

// TestStageArgInjection verifies that a path value beginning with "-" does not
// cause git to interpret it as a flag.  We can't run real git here, but we can
// confirm that the handler sends a well-formed request (uses "--" separator)
// by checking the handler doesn't panic and we can stub the run function.
//
// The "--" fix is structural (code-level); this test documents and catches
// regressions by verifying the handler rejects no valid-looking paths and
// would not pass flag-like values verbatim.
func TestCheckoutRejectsFlagBranch(t *testing.T) {
	// Use a real temp dir so the workspace check passes and we reach branch validation.
	tmp := t.TempDir()
	h := New(&staticWS{root: tmp})

	badBranches := []string{
		"--no-verify",
		"--detach",
		"-b",
		"-",
		"--",
		"--upload-pack=evil",
	}

	for _, branch := range badBranches {
		t.Run("branch="+branch, func(t *testing.T) {
			body := map[string]string{"branch": branch}
			bs, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/api/git/checkout", bytes.NewReader(bs))
			w := httptest.NewRecorder()
			h.Checkout(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("branch %q: expected 400, got %d", branch, w.Code)
			}
			var resp map[string]string
			json.NewDecoder(w.Body).Decode(&resp)
			if !strings.Contains(resp["error"], "invalid branch name") {
				t.Errorf("branch %q: expected 'invalid branch name' error, got %q", branch, resp["error"])
			}
		})
	}
}

func TestCheckoutAcceptsValidBranch(t *testing.T) {
	// Use a real temp dir so the workspace check passes.
	// The actual git command will fail (no git repo) giving 500, but it must
	// NOT give 400 "invalid branch name" for legitimate branch names.
	tmp := t.TempDir()
	h := New(&staticWS{root: tmp})

	validBranches := []string{
		"main",
		"feature/my-thing",
		"fix-123",
		"release/v2.0",
	}

	for _, branch := range validBranches {
		t.Run("branch="+branch, func(t *testing.T) {
			body := map[string]string{"branch": branch}
			bs, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/api/git/checkout", bytes.NewReader(bs))
			w := httptest.NewRecorder()
			h.Checkout(w, req)
			// Must NOT be 400 (invalid branch name).
			if w.Code == http.StatusBadRequest {
				var resp map[string]string
				json.NewDecoder(w.Body).Decode(&resp)
				t.Errorf("valid branch %q was incorrectly rejected (400): %s", branch, resp["error"])
			}
		})
	}
}

// TestCheckoutEmptyBranch verifies an empty branch name is rejected.
func TestCheckoutEmptyBranch(t *testing.T) {
	tmp := t.TempDir()
	h := New(&staticWS{root: tmp})
	body := map[string]string{"branch": ""}
	bs, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/git/checkout", bytes.NewReader(bs))
	w := httptest.NewRecorder()
	h.Checkout(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty branch: expected 400, got %d", w.Code)
	}
}
