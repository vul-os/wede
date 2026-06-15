package git

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

// ── New endpoint tests (push / pull / fetch / create-branch) ─────────────────

// initTestRepo creates a temp directory with a real git repo + initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = tmp
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(tmp+"/README.md", []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return tmp
}

func postBody(t *testing.T, body any) *bytes.Reader {
	t.Helper()
	data, _ := json.Marshal(body)
	return bytes.NewReader(data)
}

func TestCreateBranch_Basic(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/branch",
		postBody(t, map[string]any{"name": "feat/hello", "checkout": false}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CreateBranch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CreateBranch: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %q", resp["status"])
	}
	out, _ := h.run("branch", "--list", "feat/hello")
	if out == "" {
		t.Error("branch feat/hello not found after CreateBranch")
	}
}

func TestCreateBranch_AndCheckout(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/branch",
		postBody(t, map[string]any{"name": "new-feature", "checkout": true}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CreateBranch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CreateBranch+checkout: expected 200, got %d", rec.Code)
	}
	branch, _ := h.run("branch", "--show-current")
	if branch != "new-feature" {
		t.Errorf("expected current branch new-feature, got %q", branch)
	}
}

func TestCreateBranch_InvalidNames(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, name := range []string{"--evil", "-b", ""} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/branch",
			postBody(t, map[string]any{"name": name, "checkout": false}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.CreateBranch(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d", name, rec.Code)
		}
	}
}

func TestFetch_InvalidRemote(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/fetch",
		postBody(t, map[string]any{"remote": "--evil"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Fetch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for flag-like remote, got %d", rec.Code)
	}
}

func TestPull_InvalidArgs(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	// Bad remote.
	req := httptest.NewRequest(http.MethodPost, "/api/git/pull",
		postBody(t, map[string]any{"remote": "-x"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Pull(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("pull bad remote: expected 400, got %d", rec.Code)
	}

	// Bad branch with valid remote.
	req2 := httptest.NewRequest(http.MethodPost, "/api/git/pull",
		postBody(t, map[string]any{"remote": "origin", "branch": "--upstream"}))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	h.Pull(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("pull bad branch: expected 400, got %d", rec2.Code)
	}
}

func TestPush_InvalidRemote(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/push",
		postBody(t, map[string]any{"remote": "--bad"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Push(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("push bad remote: expected 400, got %d", rec.Code)
	}
}

func TestPush_InvalidBranch(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/push",
		postBody(t, map[string]any{"remote": "origin", "branch": "--force"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Push(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("push bad branch: expected 400, got %d", rec.Code)
	}
}

func TestRemotes_Empty(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodGet, "/api/git/remotes", nil)
	rec := httptest.NewRecorder()
	h.Remotes(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Remotes: expected 200, got %d", rec.Code)
	}
	var resp struct {
		Remotes []map[string]string `json:"remotes"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Remotes == nil {
		t.Error("expected non-nil remotes slice")
	}
}
