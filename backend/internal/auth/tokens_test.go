package auth

import (
	"testing"
	"time"
)

func TestMintRedeemRole(t *testing.T) {
	h := newTestHandler(t)

	raw, id, err := h.MintToken(RoleEditor, "bob", 0)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	if raw == "" || id == "" {
		t.Fatal("expected non-empty raw token and id")
	}

	// The raw token is never stored — only its hash.
	if h.tokens[id].Hash == raw {
		t.Error("raw token must not be stored; only its hash")
	}

	sess, role, username, ok := h.RedeemToken(raw)
	if !ok {
		t.Fatal("RedeemToken failed for a valid token")
	}
	if role != RoleEditor {
		t.Errorf("role = %q, want editor", role)
	}
	if username != "bob" {
		t.Errorf("username = %q, want bob", username)
	}
	// The redeemed session carries the role.
	if got := h.Role(sess); got != RoleEditor {
		t.Errorf("session role = %q, want editor", got)
	}
}

func TestMintRejectsOwnerRole(t *testing.T) {
	h := newTestHandler(t)
	if _, _, err := h.MintToken(RoleOwner, "x", 0); err == nil {
		t.Error("minting an owner token should fail (owner is the config password)")
	}
	if _, _, err := h.MintToken("bogus", "x", 0); err == nil {
		t.Error("minting an invalid role should fail")
	}
}

func TestRedeemWrongTokenRejected(t *testing.T) {
	h := newTestHandler(t)
	h.MintToken(RoleViewer, "v", 0) //nolint:errcheck
	if _, _, _, ok := h.RedeemToken("not-a-real-token"); ok {
		t.Error("redeeming a wrong token should fail")
	}
	if _, _, _, ok := h.RedeemToken(""); ok {
		t.Error("redeeming an empty token should fail")
	}
}

func TestRevokeToken(t *testing.T) {
	h := newTestHandler(t)
	raw, id, _ := h.MintToken(RoleViewer, "v", 0)

	if !h.RevokeToken(id) {
		t.Fatal("RevokeToken returned false for an existing token")
	}
	if h.RevokeToken(id) {
		t.Error("double revoke should return false")
	}
	if _, _, _, ok := h.RedeemToken(raw); ok {
		t.Error("revoked token should no longer redeem")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	h := newTestHandler(t)
	raw, id, _ := h.MintToken(RoleViewer, "v", time.Millisecond)
	// Force expiry in the past.
	h.mu.Lock()
	tok := h.tokens[id]
	tok.ExpiresAt = time.Now().Add(-time.Hour)
	h.tokens[id] = tok
	h.mu.Unlock()

	if _, _, _, ok := h.RedeemToken(raw); ok {
		t.Error("expired token should not redeem")
	}
}

func TestListTokensOmitsHash(t *testing.T) {
	h := newTestHandler(t)
	h.MintToken(RoleEditor, "a", 0) //nolint:errcheck
	h.MintToken(RoleViewer, "b", 0) //nolint:errcheck

	list := h.ListTokens()
	if len(list) != 2 {
		t.Fatalf("ListTokens len = %d, want 2", len(list))
	}
	roles := map[Role]bool{}
	for _, ti := range list {
		roles[ti.Role] = true
	}
	if !roles[RoleEditor] || !roles[RoleViewer] {
		t.Errorf("expected both roles in list, got %+v", list)
	}
}

func TestRoleCapabilities(t *testing.T) {
	if !RoleEditor.CanMutate() || !RoleOwner.CanMutate() {
		t.Error("editor and owner must be able to mutate")
	}
	if RoleViewer.CanMutate() {
		t.Error("viewer must NOT be able to mutate")
	}
	if normalizeRole("") != RoleOwner {
		t.Error("legacy empty role should normalize to owner")
	}
}
