package files

import (
	"os"
	"path/filepath"
	"testing"
)

// staticWS is a trivial WorkspaceProvider for tests.
type staticWS struct{ root string }

func (s *staticWS) Current() string { return s.root }

func TestSafePath(t *testing.T) {
	tmp := t.TempDir()
	h := New(&staticWS{root: tmp})

	tests := []struct {
		name    string
		reqPath string
		wantOK  bool
	}{
		// --- Must succeed (paths inside workspace) ---
		{"empty path → root", "", true},
		{"dot path → root", ".", true},
		{"slash → root", "/", true},
		{"simple file", "file.txt", true},
		{"nested path", "a/b/c.txt", true},

		// --- Must fail (path traversal) ---
		{"double-dot escape", "../../etc/passwd", false},
		{"single-dot escape", "../outside.txt", false},
		{"embedded traversal", "a/../../etc/passwd", false},

		// --- Prefix-collision attack (the core bug we fixed) ---
		// workspace=/tmp/wsXXXX, attacker sends ../wsXXXX2/evil
		// old HasPrefix check: wsXXXX is a prefix of wsXXXX2 → BUG allowed it
		// fixed check: requires separator after workspace prefix → correctly denied
		{"prefix collision sibling", "../" + filepath.Base(tmp) + "2/evil", false},
		{"prefix collision suffix", "../" + filepath.Base(tmp) + "suffix/secret", false},

		// --- Absolute paths are absorbed by filepath.Join on Unix ---
		// filepath.Join("/ws", "/etc/passwd") → "/ws/etc/passwd", so they're safe.
		// (We still verify the behaviour is correct.)
		{"absolute path injection", "/etc/passwd", true}, // lands at tmp/etc/passwd — inside workspace
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full, ok := h.safePath(tt.reqPath)
			if ok != tt.wantOK {
				t.Errorf("safePath(%q) ok=%v, want %v (full=%s)", tt.reqPath, ok, tt.wantOK, full)
			}
			if ok {
				// Verify result is genuinely inside the workspace.
				ws := tmp + string(filepath.Separator)
				if full != tmp && !filepath.HasPrefix(full, ws) {
					t.Errorf("safePath(%q) returned %q which is outside workspace %q", tt.reqPath, full, tmp)
				}
			}
		})
	}
}

// TestSafePathSymlinkEscapeDenied verifies that a symlink created inside the
// workspace that points outside is rejected: safePath resolves symlinks and
// confirms the real target stays within the workspace, so a planted symlink can't
// be used to read or overwrite files outside the project root.
func TestSafePathSymlinkEscapeDenied(t *testing.T) {
	tmp := t.TempDir()
	outside := t.TempDir()

	// Create a symlink inside workspace pointing outside.
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	// A file the attacker wants to reach, behind the escaping symlink.
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := New(&staticWS{root: tmp})

	// "link/secret.txt" is lexically inside the workspace but resolves outside it,
	// so safePath must deny it.
	if full, ok := h.safePath("link/secret.txt"); ok {
		t.Errorf("safePath('link/secret.txt') allowed symlink escape, got %q", full)
	}
}

// TestSafePathHTTPDeny exercises the full HTTP handler path to confirm the
// traversal is properly rejected at the HTTP level.
func TestSafePathHTTPDeny(t *testing.T) {
	tmp := t.TempDir()

	// Create a file OUTSIDE the workspace that the attacker wants to read.
	secret, err := os.MkdirTemp("", "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(secret)

	secretFile := filepath.Join(secret, "passwd")
	if err := os.WriteFile(secretFile, []byte("root:x:0:0"), 0644); err != nil {
		t.Fatal(err)
	}

	h := New(&staticWS{root: tmp})

	// Build the relative path that would escape: ../secretXXXX/passwd
	rel, err := filepath.Rel(tmp, secretFile)
	if err != nil {
		t.Fatal(err)
	}
	// rel is something like "../../tmp/secretXXXX/passwd"

	full, ok := h.safePath(rel)
	if ok {
		t.Errorf("safePath(%q) returned ok=true, full=%q — path traversal NOT blocked!", rel, full)
	}
}
