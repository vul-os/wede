package folder

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome points $HOME at a fresh temp dir so Manager persistence (~/.wede)
// and the home-directory security checks operate on throwaway state, never the
// developer's real home. It returns the temp home path.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// --- ValidateRoot: the security contract -----------------------------------

func TestValidateRootAcceptsPlainDirectory(t *testing.T) {
	home := isolateHome(t)
	proj := filepath.Join(home, "projects", "app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ValidateRoot(proj, "")
	if err != nil {
		t.Fatalf("ValidateRoot(%q) unexpected error: %v", proj, err)
	}
	// EvalSymlinks may canonicalize (e.g. /var → /private/var on macOS); compare
	// resolved forms.
	want, _ := filepath.EvalSymlinks(proj)
	if got != want {
		t.Errorf("ValidateRoot returned %q, want %q", got, want)
	}
}

func TestValidateRootRejections(t *testing.T) {
	home := isolateHome(t)

	// A real directory we can reference in negative cases.
	dotdir := filepath.Join(home, "projects", ".secret")
	if err := os.MkdirAll(dotdir, 0o755); err != nil {
		t.Fatal(err)
	}
	afile := filepath.Join(home, "projects", "file.txt")
	if err := os.WriteFile(afile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		wantSub string // substring expected in the error
	}{
		{"empty", "", "path required"},
		{"whitespace", "   ", "path required"},
		{"filesystem root", string(filepath.Separator), "filesystem root"},
		{"home itself", home, "home directory"},
		{"home via tilde", "~", "home directory"},
		{"dotfile component", dotdir, "dotfile"},
		{"nonexistent", filepath.Join(home, "projects", "nope"), "does not exist"},
		{"not a directory", afile, "not a directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateRoot(tc.path, "")
			if err == nil {
				t.Fatalf("ValidateRoot(%q) = nil error, want rejection containing %q", tc.path, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("ValidateRoot(%q) error = %q, want substring %q", tc.path, err.Error(), tc.wantSub)
			}
		})
	}
}

func TestValidateRootAllowedBaseConfinement(t *testing.T) {
	home := isolateHome(t)
	base := filepath.Join(home, "roots")
	inside := filepath.Join(base, "proj")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	// A sibling of base that shares its name prefix — the classic prefix-collision
	// bypass. base=".../roots", sibling=".../roots-evil". A naive HasPrefix(abs,
	// base) would wrongly admit it; the separator-guarded check must reject it.
	sibling := filepath.Join(home, "roots-evil", "proj")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "elsewhere")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := ValidateRoot(inside, base); err != nil {
		t.Errorf("ValidateRoot(inside, base) unexpected error: %v", err)
	}
	if _, err := ValidateRoot(base, base); err != nil {
		t.Errorf("ValidateRoot(base, base) should allow the base dir itself: %v", err)
	}
	for _, bad := range []string{sibling, outside} {
		if _, err := ValidateRoot(bad, base); err == nil {
			t.Errorf("ValidateRoot(%q, base=%q) = nil, want confinement rejection", bad, base)
		}
	}
}

func TestValidateRootSymlinkEscapeDenied(t *testing.T) {
	home := isolateHome(t)
	base := filepath.Join(home, "roots")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside-target")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	// A symlink inside the allowed base that resolves outside it must be rejected:
	// ValidateRoot resolves symlinks before the confinement check.
	link := filepath.Join(base, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink:", err)
	}
	if _, err := ValidateRoot(link, base); err == nil {
		t.Errorf("ValidateRoot(symlink escaping base) = nil, want rejection")
	}
}

// --- HandleBrowse: home-directory confinement ------------------------------

func TestHandleBrowseConfinedToHome(t *testing.T) {
	home := isolateHome(t)
	if err := os.MkdirAll(filepath.Join(home, "visible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New("")

	// Browsing home lists non-dot subdirectories only.
	rr := httptest.NewRecorder()
	m.HandleBrowse(rr, httptest.NewRequest(http.MethodGet, "/api/folder/browse", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("HandleBrowse(home) status = %d, want 200", rr.Code)
	}
	var resp struct {
		Dirs []struct{ Name string } `json:"dirs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, d := range resp.Dirs {
		names[d.Name] = true
	}
	if !names["visible"] {
		t.Errorf("HandleBrowse omitted the 'visible' dir; got %v", names)
	}
	if names[".hidden"] {
		t.Errorf("HandleBrowse leaked a dotfile dir; got %v", names)
	}

	// A path outside the home tree (e.g. /etc) must be forbidden, so an
	// authenticated user cannot enumerate the whole filesystem.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/folder/browse?path=/etc", nil)
	m.HandleBrowse(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("HandleBrowse(/etc) status = %d, want 403", rr.Code)
	}
}

// --- HandleOpen: validation + allowedRoot enforcement ----------------------

func openBody(t *testing.T, m *Manager, path string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"path": path})
	rr := httptest.NewRecorder()
	m.HandleOpen(rr, httptest.NewRequest(http.MethodPost, "/api/folder/open", strings.NewReader(string(b))))
	return rr
}

func TestHandleOpenEnforcesAllowedRoot(t *testing.T) {
	home := isolateHome(t)
	base := filepath.Join(home, "roots")
	inside := filepath.Join(base, "proj")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	m := New("")
	m.SetAllowedRoot(base)

	if rr := openBody(t, m, outside); rr.Code != http.StatusBadRequest {
		t.Errorf("HandleOpen(outside allowed root) status = %d, want 400", rr.Code)
	}
	if m.HasWorkspace() {
		t.Errorf("a rejected open must not set the current workspace")
	}

	rr := openBody(t, m, inside)
	if rr.Code != http.StatusOK {
		t.Fatalf("HandleOpen(inside allowed root) status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	if !m.HasWorkspace() {
		t.Errorf("a successful open must set the current workspace")
	}
}

func TestHandleOpenRejectsMissingPath(t *testing.T) {
	isolateHome(t)
	m := New("")
	rr := httptest.NewRecorder()
	m.HandleOpen(rr, httptest.NewRequest(http.MethodPost, "/api/folder/open", strings.NewReader("{}")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleOpen(no path) status = %d, want 400", rr.Code)
	}
}

// --- Manager: state, recents, listeners ------------------------------------

func TestSetWorkspaceNotifiesAndPersistsRecents(t *testing.T) {
	home := isolateHome(t)
	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	m := New("")
	var notified []string
	m.OnChange(func(p string) { notified = append(notified, p) })

	if err := m.SetWorkspace(a); err != nil {
		t.Fatal(err)
	}
	if err := m.SetWorkspace(b); err != nil {
		t.Fatal(err)
	}
	if err := m.SetWorkspace(a); err != nil { // re-select a: should move to front, no dup
		t.Fatal(err)
	}

	if len(notified) != 3 {
		t.Errorf("OnChange fired %d times, want 3", len(notified))
	}
	if m.Current() != mustEval(a) && m.Current() != a {
		t.Errorf("Current() = %q, want %q", m.Current(), a)
	}

	recents := m.Recents()
	if len(recents) != 2 {
		t.Fatalf("Recents() = %v, want 2 unique entries", recents)
	}
	// Most-recent-first: a was selected last, so it heads the list.
	if recents[0] != m.Current() {
		t.Errorf("Recents()[0] = %q, want most-recent %q", recents[0], m.Current())
	}

	// A fresh Manager (same $HOME) must reload the persisted recents.
	m2 := New("")
	if len(m2.Recents()) != 2 {
		t.Errorf("reloaded Recents() = %v, want 2", m2.Recents())
	}
}

func TestSetWorkspaceRejectsNonDirectory(t *testing.T) {
	home := isolateHome(t)
	f := filepath.Join(home, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New("")
	if err := m.SetWorkspace(f); err == nil {
		t.Errorf("SetWorkspace(file) = nil, want error")
	}
	if err := m.SetWorkspace(filepath.Join(home, "nope")); err == nil {
		t.Errorf("SetWorkspace(missing) = nil, want error")
	}
}

func mustEval(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
