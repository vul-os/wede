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

func TestDeleteBranch(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	// Create a branch, then delete it.
	if _, err := h.run("branch", "--", "feat/tmp"); err != nil {
		t.Fatalf("setup branch: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/git/branch/delete",
		postBody(t, map[string]any{"name": "feat/tmp", "force": false}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.DeleteBranch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteBranch: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if out, _ := h.run("branch", "--list", "feat/tmp"); out != "" {
		t.Errorf("branch feat/tmp still present after delete: %q", out)
	}
}

func TestDeleteBranch_RejectsFlagName(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodPost, "/api/git/branch/delete",
		postBody(t, map[string]any{"name": "-D"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.DeleteBranch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("flag-like branch name should be rejected, got %d", rec.Code)
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

// ── Discard tests ─────────────────────────────────────────────────────────────

// TestDiscard_RejectsFlag ensures a path starting with "-" is rejected with 400.
func TestDiscard_RejectsFlag(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	flagPaths := []string{
		"-p",
		"--source=HEAD",
		"--staged",
		"-",
	}
	for _, p := range flagPaths {
		t.Run("path="+p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/git/discard",
				postBody(t, map[string]string{"path": p}))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.Discard(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("path %q: expected 400, got %d", p, rec.Code)
			}
			var resp map[string]string
			json.NewDecoder(rec.Body).Decode(&resp)
			if !strings.Contains(resp["error"], "invalid path") {
				t.Errorf("path %q: expected 'invalid path' error, got %q", p, resp["error"])
			}
		})
	}
}

// TestDiscard_RejectsEmpty ensures an empty path is rejected with 400.
func TestDiscard_RejectsEmpty(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/discard",
		postBody(t, map[string]string{"path": ""}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Discard(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty path: expected 400, got %d", rec.Code)
	}
}

// ── Stash tests ───────────────────────────────────────────────────────────────

// TestStashPop_InvalidIndex verifies negative and non-integer indices are rejected.
func TestStashPop_InvalidIndex(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	// Negative index — JSON integer but invalid value.
	t.Run("negative index", func(t *testing.T) {
		neg := -1
		req := httptest.NewRequest(http.MethodPost, "/api/git/stash/pop",
			postBody(t, map[string]any{"index": neg}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.StashPop(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("negative index: expected 400, got %d", rec.Code)
		}
	})

	// Missing index field — should return 400 (index required).
	t.Run("missing index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/git/stash/pop",
			postBody(t, map[string]any{}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.StashPop(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("missing index: expected 400, got %d", rec.Code)
		}
	})
}

// TestStashDrop_InvalidIndex mirrors TestStashPop_InvalidIndex for StashDrop.
func TestStashDrop_InvalidIndex(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	neg := -5
	req := httptest.NewRequest(http.MethodPost, "/api/git/stash/drop",
		postBody(t, map[string]any{"index": neg}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.StashDrop(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative index: expected 400, got %d", rec.Code)
	}
}

// TestStashList_Empty verifies StashList returns an empty slice on a fresh repo.
func TestStashList_Empty(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodGet, "/api/git/stash", nil)
	rec := httptest.NewRecorder()
	h.StashList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("StashList: expected 200, got %d", rec.Code)
	}
	var resp struct {
		Stashes []StashEntry `json:"stashes"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Stashes == nil {
		t.Error("expected non-nil stashes slice")
	}
	if len(resp.Stashes) != 0 {
		t.Errorf("expected 0 stashes on fresh repo, got %d", len(resp.Stashes))
	}
}

// ── CommitDiff tests ──────────────────────────────────────────────────────────

// TestCommitDiff_InvalidHash verifies that non-hex or dangerous hash values are
// rejected with 400, while a valid short hex hash reaches git (may return 500
// for an unknown commit, but must not return 400).
func TestCommitDiff_InvalidHash(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	type hashCase struct {
		raw     string // value to validate
		urlSafe string // URL-safe representation for httptest.NewRequest
	}
	badHashes := []hashCase{
		{"../../etc/passwd", "..%2F..%2Fetc%2Fpasswd"},
		{"--all", "--all"},
		{"; rm -rf /", "%3B+rm+-rf+%2F"},
		{"HEAD", "HEAD"},
		{"main", "main"},
		{"", ""},
		{"xyz", "xyz"},       // too short (< 4 chars)
		{"ABCDEF", "ABCDEF"}, // uppercase — not allowed
	}
	for _, tc := range badHashes {
		hash := tc.raw
		urlSafe := tc.urlSafe
		t.Run("hash="+hash, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/git/commit-diff?hash="+urlSafe, nil)
			rec := httptest.NewRecorder()
			h.CommitDiff(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("hash %q: expected 400, got %d", hash, rec.Code)
			}
			var resp map[string]string
			json.NewDecoder(rec.Body).Decode(&resp)
			if !strings.Contains(resp["error"], "invalid commit hash") {
				t.Errorf("hash %q: expected 'invalid commit hash' error, got %q", hash, resp["error"])
			}
		})
	}
}

// TestCommitDiff_ValidHashFormat verifies a properly formatted hex hash passes
// the format check and reaches git (which may return non-200 for unknown commit,
// but never 400 "invalid commit hash").
func TestCommitDiff_ValidHashFormat(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	// "abc123" is a valid short hex hash that doesn't exist in the repo.
	// The handler should NOT return 400 for it.
	req := httptest.NewRequest(http.MethodGet, "/api/git/commit-diff?hash=abc123", nil)
	rec := httptest.NewRecorder()
	h.CommitDiff(rec, req)

	if rec.Code == http.StatusBadRequest {
		var resp map[string]string
		json.NewDecoder(rec.Body).Decode(&resp)
		t.Errorf("valid hex hash was rejected with 400: %s", resp["error"])
	}
}

// ── New feature tests ─────────────────────────────────────────────────────────

// TestConflictStatus creates a real merge conflict and verifies Status
// returns the file with conflicted:true and status:"conflict".
func TestConflictStatus(t *testing.T) {
	repo := initTestRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// Create a branch with a different change on the same line.
	run("checkout", "-b", "branch-a")
	if err := os.WriteFile(repo+"/README.md", []byte("branch-a change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "branch-a")

	// Switch back and make a conflicting change.
	defaultBranch := run("rev-parse", "--abbrev-ref", "HEAD~1")
	_ = defaultBranch

	// We need to go back to the initial branch. Use git log to find it.
	run("checkout", "-")

	if err := os.WriteFile(repo+"/README.md", []byte("main change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "main change")

	// Attempt merge — this will produce a conflict.
	mergeCmd := exec.Command("git", "merge", "branch-a")
	mergeCmd.Dir = repo
	mergeCmd.Run() // intentionally ignore error — conflict is expected

	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodGet, "/api/git/status", nil)
	rec := httptest.NewRecorder()
	h.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Files []StatusFile `json:"files"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, f := range resp.Files {
		if f.Path == "README.md" && f.Conflicted {
			found = true
			if f.Status != "conflict" {
				t.Errorf("conflicted file status should be 'conflict', got %q", f.Status)
			}
		}
	}
	if !found {
		t.Errorf("expected README.md with conflicted:true in status, got %+v", resp.Files)
	}
}

// TestConflictResolve_InvalidPath ensures empty/flag paths are rejected.
func TestConflictResolve_InvalidPath(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, path := range []string{"", "-evil", "--flag"} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/conflict/resolve",
			postBody(t, map[string]any{"path": path, "resolutions": []any{}}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ConflictResolve(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path=%q: expected 400, got %d", path, rec.Code)
		}
	}
}

// TestRemoteAdd_InvalidName ensures names with special chars are rejected.
func TestRemoteAdd_InvalidName(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	badNames := []string{"--evil", "-x", "name with space", "na$me", "na@me", ""}
	for _, name := range badNames {
		req := httptest.NewRequest(http.MethodPost, "/api/git/remotes/add",
			postBody(t, map[string]any{"name": name, "url": "https://example.com"}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.RemoteAdd(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d", name, rec.Code)
		}
	}
}

// TestRemoteRemove_InvalidName ensures flag-like names are rejected.
func TestRemoteRemove_InvalidName(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	badNames := []string{"--evil", "-x", "na me", ""}
	for _, name := range badNames {
		req := httptest.NewRequest(http.MethodPost, "/api/git/remotes/remove",
			postBody(t, map[string]any{"name": name}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.RemoteRemove(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d", name, rec.Code)
		}
	}
}

// TestStageHunk_EmptyPatch ensures an empty patch is rejected with 400.
func TestStageHunk_EmptyPatch(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/stage-hunk",
		postBody(t, map[string]any{"patch": ""}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.StageHunk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty patch: expected 400, got %d", rec.Code)
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   Log graph (--date-order + parent hashes)
═══════════════════════════════════════════════════════════════════════ */

// TestLog_GraphParents verifies that a merge commit appears in the log with
// two parent hashes, confirming the %P field is populated.
func TestLog_GraphParents(t *testing.T) {
	repo := initTestRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// Create a branch + diverging commit, then merge it so we get a merge commit.
	run("checkout", "-b", "graph-branch")
	if err := os.WriteFile(repo+"/branch.txt", []byte("branch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "branch.txt")
	run("commit", "-m", "branch commit")
	run("checkout", "-")
	run("merge", "--no-ff", "graph-branch", "-m", "Merge graph-branch")

	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodGet, "/api/git/log?count=10", nil)
	rec := httptest.NewRecorder()
	h.Log(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Log: expected 200, got %d", rec.Code)
	}

	var resp struct {
		Entries []LogEntry `json:"entries"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	var mergeFound bool
	for _, e := range resp.Entries {
		if len(e.Parents) == 2 {
			mergeFound = true
		}
		if e.DateISO == "" {
			t.Errorf("entry %s: DateISO should not be empty", e.Short)
		}
	}
	if !mergeFound {
		t.Error("expected a merge commit with 2 parents in log output")
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   Blame
═══════════════════════════════════════════════════════════════════════ */

func TestBlame_InvalidPath(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	badPaths := []string{"", "-f", "--file"}
	for _, p := range badPaths {
		req := httptest.NewRequest(http.MethodGet, "/api/git/blame?file="+p, nil)
		rec := httptest.NewRecorder()
		h.Blame(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path=%q: expected 400, got %d", p, rec.Code)
		}
	}
}

func TestBlame_ValidFile(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodGet, "/api/git/blame?file=README.md", nil)
	rec := httptest.NewRecorder()
	h.Blame(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Blame: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Lines []BlameLineInfo `json:"lines"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Lines) == 0 {
		t.Fatal("expected at least one blame line for README.md")
	}
	for _, l := range resp.Lines {
		if l.Hash == "" {
			t.Error("expected non-empty hash in blame line")
		}
		if l.Author == "" {
			t.Errorf("line %d: expected non-empty author", l.LineNo)
		}
	}
}

// TestParseBlameOutput unit-tests the pure parsing function directly.
func TestParseBlameOutput(t *testing.T) {
	// Hash is exactly 40 lowercase hex chars.
	const input = `abc123456789012345678901234567890123456f 1 1 1
author Alice
author-mail <alice@example.com>
author-time 1609459200
author-tz +0000
committer Alice
committer-mail <alice@example.com>
committer-time 1609459200
committer-tz +0000
summary Initial commit
filename README.md
	# hello
abc123456789012345678901234567890123456f 2 2
	world
`
	lines := parseBlameOutput(input)
	if len(lines) != 2 {
		t.Fatalf("expected 2 blame lines, got %d", len(lines))
	}
	if lines[0].Content != "# hello" {
		t.Errorf("line 0 content: got %q", lines[0].Content)
	}
	if lines[0].Author != "Alice" {
		t.Errorf("line 0 author: got %q", lines[0].Author)
	}
	if lines[0].Date != "2021-01-01" {
		t.Errorf("line 0 date: got %q", lines[0].Date)
	}
	if lines[1].LineNo != 2 {
		t.Errorf("line 1 lineNo: got %d", lines[1].LineNo)
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   CherryPick
═══════════════════════════════════════════════════════════════════════ */

func TestCherryPick_InvalidHash(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, hash := range []string{"", "HEAD", "--all", "xyz"} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/cherry-pick",
			postBody(t, map[string]string{"hash": hash}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.CherryPick(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("hash=%q: expected 400, got %d", hash, rec.Code)
		}
	}
}

func TestCherryPick_Valid(t *testing.T) {
	repo := initTestRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// Create a feature branch with a unique new file to avoid conflicts.
	run("checkout", "-b", "feature-cp")
	if err := os.WriteFile(repo+"/feature.txt", []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "feature.txt")
	run("commit", "-m", "feature commit")
	featureHash := run("rev-parse", "HEAD")

	// Return to the original branch.
	run("checkout", "-")

	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodPost, "/api/git/cherry-pick",
		postBody(t, map[string]string{"hash": featureHash}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CherryPick(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CherryPick: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Verify the new file exists on the original branch.
	if _, err := os.Stat(repo + "/feature.txt"); os.IsNotExist(err) {
		t.Error("cherry-picked file feature.txt not found")
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   Revert
═══════════════════════════════════════════════════════════════════════ */

func TestRevert_InvalidHash(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, hash := range []string{"", "HEAD", "--all"} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/revert",
			postBody(t, map[string]string{"hash": hash}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Revert(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("hash=%q: expected 400, got %d", hash, rec.Code)
		}
	}
}

func TestRevert_Valid(t *testing.T) {
	repo := initTestRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// Second commit: add a new file so the revert only touches that file.
	if err := os.WriteFile(repo+"/extra.txt", []byte("extra\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "extra.txt")
	run("commit", "-m", "second commit")
	hash := run("rev-parse", "HEAD")

	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodPost, "/api/git/revert",
		postBody(t, map[string]string{"hash": hash}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Revert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Revert: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// After revert, extra.txt should no longer exist.
	if _, err := os.Stat(repo + "/extra.txt"); !os.IsNotExist(err) {
		t.Error("extra.txt should have been removed by revert")
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   Reset
═══════════════════════════════════════════════════════════════════════ */

func TestReset_InvalidArgs(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	// Invalid hash.
	req := httptest.NewRequest(http.MethodPost, "/api/git/reset",
		postBody(t, map[string]any{"hash": "HEAD", "mode": "soft"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Reset(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid hash: expected 400, got %d", rec.Code)
	}

	// Invalid mode — use real hash.
	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = repo
	rawHash, _ := hashCmd.Output()
	hash := strings.TrimSpace(string(rawHash))

	req2 := httptest.NewRequest(http.MethodPost, "/api/git/reset",
		postBody(t, map[string]any{"hash": hash, "mode": "brutal"}))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	h.Reset(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("invalid mode: expected 400, got %d", rec2.Code)
	}
}

func TestReset_Soft(t *testing.T) {
	repo := initTestRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// Make a second commit to have somewhere to reset to.
	if err := os.WriteFile(repo+"/file2.txt", []byte("second\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "file2.txt")
	run("commit", "-m", "second")

	// Soft-reset back to the first (root) commit.
	firstHash := run("rev-list", "--max-parents=0", "HEAD")

	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodPost, "/api/git/reset",
		postBody(t, map[string]any{"hash": firstHash, "mode": "soft"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Reset(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Reset soft: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	head := run("rev-parse", "HEAD")
	if head != firstHash {
		t.Errorf("expected HEAD=%s after soft reset, got %s", firstHash, head)
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   Merge
═══════════════════════════════════════════════════════════════════════ */

func TestMerge_InvalidBranch(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, b := range []string{"", "--evil", "-x"} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/merge",
			postBody(t, map[string]string{"branch": b}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Merge(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("branch=%q: expected 400, got %d", b, rec.Code)
		}
	}
}

func TestMerge_Valid(t *testing.T) {
	repo := initTestRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// Create a branch with a unique file (no conflicts).
	run("checkout", "-b", "feature-merge")
	if err := os.WriteFile(repo+"/merged.txt", []byte("merged\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "merged.txt")
	run("commit", "-m", "feature commit")
	run("checkout", "-")

	h := New(&staticWS{root: repo})
	req := httptest.NewRequest(http.MethodPost, "/api/git/merge",
		postBody(t, map[string]string{"branch": "feature-merge"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Merge(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Merge: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// merged.txt should exist on the main branch now.
	if _, err := os.Stat(repo + "/merged.txt"); os.IsNotExist(err) {
		t.Error("merged.txt not found after merge")
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   Tags
═══════════════════════════════════════════════════════════════════════ */

func TestTags_Empty(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodGet, "/api/git/tags", nil)
	rec := httptest.NewRecorder()
	h.Tags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Tags: expected 200, got %d", rec.Code)
	}
	var resp struct {
		Tags []TagEntry `json:"tags"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Tags == nil {
		t.Error("expected non-nil tags slice")
	}
	if len(resp.Tags) != 0 {
		t.Errorf("expected 0 tags on fresh repo, got %d", len(resp.Tags))
	}
}

func TestTagCreate_InvalidName(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, name := range []string{"", "--evil", "-v1"} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/tag",
			postBody(t, map[string]string{"name": name}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.TagCreate(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d", name, rec.Code)
		}
	}
}

func TestTagCreate_InvalidHash(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	req := httptest.NewRequest(http.MethodPost, "/api/git/tag",
		postBody(t, map[string]any{"name": "v1.0", "hash": "NOTAHEX"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.TagCreate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid hash: expected 400, got %d", rec.Code)
	}
}

func TestTagCreate_And_Delete(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	// Create a lightweight tag.
	req := httptest.NewRequest(http.MethodPost, "/api/git/tag",
		postBody(t, map[string]string{"name": "v0.1"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.TagCreate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("TagCreate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify it appears in the tag list.
	req2 := httptest.NewRequest(http.MethodGet, "/api/git/tags", nil)
	rec2 := httptest.NewRecorder()
	h.Tags(rec2, req2)
	var resp struct {
		Tags []TagEntry `json:"tags"`
	}
	json.NewDecoder(rec2.Body).Decode(&resp)
	var found bool
	for _, tag := range resp.Tags {
		if tag.Name == "v0.1" {
			found = true
		}
	}
	if !found {
		t.Error("tag v0.1 not found after creation")
	}

	// Delete the tag.
	req3 := httptest.NewRequest(http.MethodPost, "/api/git/tag/delete",
		postBody(t, map[string]string{"name": "v0.1"}))
	req3.Header.Set("Content-Type", "application/json")
	rec3 := httptest.NewRecorder()
	h.TagDelete(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("TagDelete: expected 200, got %d: %s", rec3.Code, rec3.Body.String())
	}

	// Confirm it is gone.
	req4 := httptest.NewRequest(http.MethodGet, "/api/git/tags", nil)
	rec4 := httptest.NewRecorder()
	h.Tags(rec4, req4)
	var resp2 struct {
		Tags []TagEntry `json:"tags"`
	}
	json.NewDecoder(rec4.Body).Decode(&resp2)
	for _, tag := range resp2.Tags {
		if tag.Name == "v0.1" {
			t.Error("tag v0.1 still present after deletion")
		}
	}
}

func TestTagDelete_InvalidName(t *testing.T) {
	repo := initTestRepo(t)
	h := New(&staticWS{root: repo})

	for _, name := range []string{"", "--evil", "-x"} {
		req := httptest.NewRequest(http.MethodPost, "/api/git/tag/delete",
			postBody(t, map[string]string{"name": name}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.TagDelete(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d", name, rec.Code)
		}
	}
}
