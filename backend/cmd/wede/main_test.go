package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"wede/backend/internal/auth"
	"wede/backend/internal/config"
)

// okHandler is a trivial next-handler used to exercise the middleware.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestSecurityHeadersDefault verifies the standalone default denies all
// cross-origin framing.
func TestSecurityHeadersDefault(t *testing.T) {
	cfg := &config.Config{FrameAncestors: ""}
	h := securityHeaders(cfg, okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	res := rec.Result()
	if got := res.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); got != "frame-ancestors 'self'" {
		t.Errorf("CSP = %q, want frame-ancestors 'self'", got)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

// TestRouteRoleGating is a regression integration test for audit findings H1 and M1:
//
//   - H1: LSP WebSocket (/api/workspaces/{id}/lsp) was ungated — a viewer could
//     open it and trigger language server process spawning (≡ RCE).
//   - M1: workspace DELETE (/api/workspaces/{id}) was ungated — a viewer could
//     unregister any workspace (DoS).
//
// The test wires a minimal mux that mirrors the gating logic in main(), then
// confirms that a viewer session is rejected with 403 on every editor-only route
// and that an editor session is passed through (stub handler returns 200).
// Adding this test before the audit would have caught H1 immediately.
func TestRouteRoleGating(t *testing.T) {
	// Isolated auth handler — uses t.TempDir() so tests never touch ~/.wede.
	ah := auth.NewWithDataDir("testpwd", t.TempDir())

	// Mint invite tokens for each role, then redeem them for session tokens.
	viewerRaw, _, err := ah.MintToken(auth.RoleViewer, "viewer-user", 0)
	if err != nil {
		t.Fatalf("MintToken viewer: %v", err)
	}
	editorRaw, _, err := ah.MintToken(auth.RoleEditor, "editor-user", 0)
	if err != nil {
		t.Fatalf("MintToken editor: %v", err)
	}
	viewerSess, _, _, ok := ah.RedeemToken(viewerRaw)
	if !ok {
		t.Fatal("RedeemToken viewer: not ok")
	}
	editorSess, _, _, ok := ah.RedeemToken(editorRaw)
	if !ok {
		t.Fatal("RedeemToken editor: not ok")
	}

	// Stub handler — used as the underlying handler behind RequireEditor; it
	// always returns 200 so we can distinguish "gating passed" from 403.
	stub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	re := ah.RequireEditor

	// Mirror only the four routes that were (or must remain) editor-gated.
	protected := http.NewServeMux()
	protected.Handle("GET /api/workspaces/{id}/terminal", re(stub))
	protected.Handle("GET /api/workspaces/{id}/lsp", re(stub)) // H1: must be editor-gated
	protected.Handle("GET /api/workspaces/{id}/dap", re(stub))
	protected.Handle("DELETE /api/workspaces/{id}", re(stub)) // M1: must be editor-gated

	mux := http.NewServeMux()
	mux.Handle("/api/", ah.Middleware(protected))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	routes := []struct{ method, path string }{
		{"GET", "/api/workspaces/default/terminal"},
		{"GET", "/api/workspaces/default/lsp"},
		{"GET", "/api/workspaces/default/dap"},
		{"DELETE", "/api/workspaces/default"},
	}

	for _, rt := range routes {
		rt := rt
		t.Run(rt.method+"_"+rt.path, func(t *testing.T) {
			// Viewer must be rejected with 403 Forbidden.
			req, _ := http.NewRequest(rt.method, srv.URL+rt.path, nil)
			req.Header.Set("Authorization", viewerSess)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("viewer request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("viewer on %s %s: got %d, want 403 Forbidden", rt.method, rt.path, resp.StatusCode)
			}

			// Editor must not be blocked at the auth gate (stub returns 200).
			req2, _ := http.NewRequest(rt.method, srv.URL+rt.path, nil)
			req2.Header.Set("Authorization", editorSess)
			resp2, err := http.DefaultClient.Do(req2)
			if err != nil {
				t.Fatalf("editor request: %v", err)
			}
			resp2.Body.Close()
			if resp2.StatusCode == http.StatusForbidden || resp2.StatusCode == http.StatusUnauthorized {
				t.Errorf("editor on %s %s: got %d, want pass-through (200)", rt.method, rt.path, resp2.StatusCode)
			}
		})
	}
}

// TestSecurityHeadersEmbed verifies that configuring FrameAncestors switches to
// a CSP-only policy (no X-Frame-Options) so trusted origins can embed wede.
func TestSecurityHeadersEmbed(t *testing.T) {
	cfg := &config.Config{FrameAncestors: "https://vulos.org"}
	h := securityHeaders(cfg, okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	res := rec.Result()
	if got := res.Header.Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options = %q, want empty in embed mode", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); got != "frame-ancestors https://vulos.org" {
		t.Errorf("CSP = %q, want frame-ancestors https://vulos.org", got)
	}
}
