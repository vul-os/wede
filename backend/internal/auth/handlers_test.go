package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ownerToken logs in as the owner and returns the session token.
func ownerToken(t *testing.T, h *Handler) string {
	t.Helper()
	out := login(t, h, "secret", "owner")
	tok, _ := out["token"].(string)
	if tok == "" {
		t.Fatal("ownerToken: no token returned from login")
	}
	return tok
}

// freshTokens returns a Handler with a clean (empty) token store.
func freshTokens(t *testing.T) *Handler {
	t.Helper()
	h := newTestHandler(t)
	h.mu.Lock()
	h.tokens = make(map[string]shareToken)
	h.mu.Unlock()
	return h
}

// ── HandleMintToken ───────────────────────────────────────────────────────────

func TestHandleMintTokenViewer(t *testing.T) {
	h := freshTokens(t)

	body := `{"role":"viewer","username":"alice","ttlHours":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleMintToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mint viewer: status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	json.NewDecoder(rec.Result().Body).Decode(&out)
	raw := out["raw"]
	if raw == "" {
		t.Fatal("expected non-empty raw token")
	}
	if out["id"] == "" {
		t.Error("expected non-empty id")
	}
	if out["inviteUrl"] != "?invite="+raw {
		t.Errorf("inviteUrl = %q, want ?invite=%s", out["inviteUrl"], raw)
	}
}

func TestHandleMintTokenEditor(t *testing.T) {
	h := freshTokens(t)

	body := `{"role":"editor","username":"bob"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleMintToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mint editor: status = %d, want 200", rec.Code)
	}
	var out map[string]string
	json.NewDecoder(rec.Result().Body).Decode(&out)
	if out["raw"] == "" {
		t.Error("expected non-empty raw token for editor")
	}
}

func TestHandleMintTokenOwnerRejected(t *testing.T) {
	h := freshTokens(t)

	body := `{"role":"owner","username":"eve"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleMintToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("minting owner role: status = %d, want 400", rec.Code)
	}
}

func TestHandleMintTokenInvalidRoleRejected(t *testing.T) {
	h := freshTokens(t)

	body := `{"role":"superadmin","username":"eve"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleMintToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("minting invalid role: status = %d, want 400", rec.Code)
	}
}

func TestHandleMintTokenNoExpiry(t *testing.T) {
	h := freshTokens(t)

	body := `{"role":"viewer","username":"anon"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleMintToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mint no-expiry: status = %d", rec.Code)
	}
}

// ── HandleListTokens ──────────────────────────────────────────────────────────

func TestHandleListTokens(t *testing.T) {
	h := freshTokens(t)
	h.MintToken(RoleViewer, "a", 0)  //nolint:errcheck
	h.MintToken(RoleEditor, "b", 0)  //nolint:errcheck

	req := httptest.NewRequest(http.MethodGet, "/api/auth/tokens", nil)
	rec := httptest.NewRecorder()
	h.HandleListTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", rec.Code)
	}
	var out map[string]any
	json.NewDecoder(rec.Result().Body).Decode(&out)
	tokens, ok := out["tokens"].([]any)
	if !ok || len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %+v", out["tokens"])
	}
}

func TestHandleListTokensEmpty(t *testing.T) {
	h := freshTokens(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/tokens", nil)
	rec := httptest.NewRecorder()
	h.HandleListTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list empty: status = %d, want 200", rec.Code)
	}
	var out map[string]any
	json.NewDecoder(rec.Result().Body).Decode(&out)
	tokens, _ := out["tokens"].([]any)
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

// ── HandleRevokeToken ─────────────────────────────────────────────────────────

func TestHandleRevokeToken(t *testing.T) {
	h := freshTokens(t)
	raw, id, _ := h.MintToken(RoleViewer, "v", 0)

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/tokens/"+id, nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleRevokeToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Redeem after revoke should fail.
	redeemBody := `{"token":"` + raw + `"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/redeem", strings.NewReader(redeemBody))
	rec2 := httptest.NewRecorder()
	h.HandleRedeem(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("redeem after revoke: status = %d, want 401", rec2.Code)
	}
}

func TestHandleRevokeTokenNotFound(t *testing.T) {
	h := freshTokens(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/tokens/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rec := httptest.NewRecorder()
	h.HandleRevokeToken(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("revoke nonexistent: status = %d, want 404", rec.Code)
	}
}

// ── HandleRedeem ──────────────────────────────────────────────────────────────

func TestHandleRedeemViewer(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleViewer, "alice", 0)

	body := `{"token":"` + raw + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/redeem", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleRedeem(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("redeem viewer: status = %d, want 200", rec.Code)
	}
	var out map[string]string
	json.NewDecoder(rec.Result().Body).Decode(&out)
	if out["role"] != "viewer" {
		t.Errorf("role = %q, want viewer", out["role"])
	}
	if out["username"] != "alice" {
		t.Errorf("username = %q, want alice", out["username"])
	}
	if out["token"] == "" {
		t.Error("expected session token in response")
	}
	// The redeemed session must carry the correct role.
	if got := h.Role(out["token"]); got != RoleViewer {
		t.Errorf("session role = %q, want viewer", got)
	}
}

func TestHandleRedeemEditor(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleEditor, "bob", 0)

	body := `{"token":"` + raw + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/redeem", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleRedeem(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("redeem editor: status = %d", rec.Code)
	}
	var out map[string]string
	json.NewDecoder(rec.Result().Body).Decode(&out)
	if out["role"] != "editor" {
		t.Errorf("role = %q, want editor", out["role"])
	}
	if got := h.Role(out["token"]); got != RoleEditor {
		t.Errorf("session role = %q, want editor", got)
	}
}

func TestHandleRedeemBadToken(t *testing.T) {
	h := freshTokens(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/redeem", strings.NewReader(`{"token":"not-a-real-token"}`))
	rec := httptest.NewRecorder()
	h.HandleRedeem(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("bad redeem: status = %d, want 401", rec.Code)
	}
}

func TestHandleRedeemEmptyToken(t *testing.T) {
	h := freshTokens(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/redeem", strings.NewReader(`{"token":""}`))
	rec := httptest.NewRecorder()
	h.HandleRedeem(rec, req)

	// Empty body.Token causes Decode to fail validation.
	if rec.Code == http.StatusOK {
		t.Error("empty token should not return 200")
	}
}

// ── Check includes role ───────────────────────────────────────────────────────

func TestCheckIncludesOwnerRole(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "owner")
	tok := out["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	req.Header.Set("Authorization", tok)
	rec := httptest.NewRecorder()
	h.Check(rec, req)

	var chk map[string]any
	json.NewDecoder(rec.Result().Body).Decode(&chk)
	if chk["role"] != "owner" {
		t.Errorf("check role = %v, want owner", chk["role"])
	}
}

func TestCheckIncludesViewerRole(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleViewer, "v", 0)
	sess, _, _, _ := h.RedeemToken(raw)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	req.Header.Set("Authorization", sess)
	rec := httptest.NewRecorder()
	h.Check(rec, req)

	var chk map[string]any
	json.NewDecoder(rec.Result().Body).Decode(&chk)
	if chk["role"] != "viewer" {
		t.Errorf("check role = %v, want viewer", chk["role"])
	}
}

// ── RequireEditor middleware ───────────────────────────────────────────────────

func TestRequireEditorBlocks403Viewer(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleViewer, "v", 0)
	sess, _, _, _ := h.RedeemToken(raw)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(h.RequireEditor(inner))

	req := httptest.NewRequest(http.MethodPut, "/api/files/write", nil)
	req.Header.Set("Authorization", sess)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer through RequireEditor: status = %d, want 403", rec.Code)
	}
}

func TestRequireEditorPassesEditor(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleEditor, "e", 0)
	sess, _, _, _ := h.RedeemToken(raw)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(h.RequireEditor(inner))

	req := httptest.NewRequest(http.MethodPut, "/api/files/write", nil)
	req.Header.Set("Authorization", sess)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("editor through RequireEditor: status = %d, want 200", rec.Code)
	}
}

func TestRequireEditorPassesOwner(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "owner")
	tok := out["token"].(string)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(h.RequireEditor(inner))

	req := httptest.NewRequest(http.MethodPut, "/api/files/write", nil)
	req.Header.Set("Authorization", tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("owner through RequireEditor: status = %d, want 200", rec.Code)
	}
}

// ── RequireOwner middleware ───────────────────────────────────────────────────

func TestRequireOwnerBlocksEditor(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleEditor, "e", 0)
	sess, _, _, _ := h.RedeemToken(raw)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(h.RequireOwner(inner))

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/tokens/x", nil)
	req.Header.Set("Authorization", sess)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("editor through RequireOwner: status = %d, want 403", rec.Code)
	}
}

func TestRequireOwnerBlocksViewer(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleViewer, "v", 0)
	sess, _, _, _ := h.RedeemToken(raw)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(h.RequireOwner(inner))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/tokens", nil)
	req.Header.Set("Authorization", sess)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer through RequireOwner: status = %d, want 403", rec.Code)
	}
}

func TestRequireOwnerPassesOwner(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "owner")
	tok := out["token"].(string)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(h.RequireOwner(inner))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/tokens", nil)
	req.Header.Set("Authorization", tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("owner through RequireOwner: status = %d, want 200", rec.Code)
	}
}

// ── GetRole helper ────────────────────────────────────────────────────────────

func TestGetRoleFromContext(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "owner")
	tok := out["token"].(string)

	var gotRole Role
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRole = GetRole(r)
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Authorization", tok)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotRole != RoleOwner {
		t.Errorf("GetRole = %q, want owner", gotRole)
	}
}

func TestGetRoleViewerFromContext(t *testing.T) {
	h := freshTokens(t)
	raw, _, _ := h.MintToken(RoleViewer, "v", 0)
	sess, _, _, _ := h.RedeemToken(raw)

	var gotRole Role
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRole = GetRole(r)
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Authorization", sess)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotRole != RoleViewer {
		t.Errorf("GetRole = %q, want viewer", gotRole)
	}
}
