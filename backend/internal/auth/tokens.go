package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// errInvalidRole is returned when minting a token for a non-mintable role.
var errInvalidRole = errors.New("auth: role must be viewer or editor")

// shareToken is a per-user invite credential. Only its SHA-256 hash is persisted;
// the raw token is returned exactly once, at mint time, and lives only in the
// invite URL the owner shares.
type shareToken struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"` // sha256 hex of the raw token
	Role      Role      `json:"role"`
	Username  string    `json:"username,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"` // zero = no expiry
}

// TokenInfo is the non-secret view of a token for the owner's management UI.
type TokenInfo struct {
	ID        string    `json:"id"`
	Role      Role      `json:"role"`
	Username  string    `json:"username,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) tokensFile() string {
	return filepath.Join(h.dataDir, "tokens.json")
}

func (h *Handler) loadTokens() {
	data, err := os.ReadFile(h.tokensFile())
	if err != nil {
		return
	}
	var stored map[string]shareToken
	if json.Unmarshal(data, &stored) == nil {
		now := time.Now()
		for id, t := range stored {
			if t.ExpiresAt.IsZero() || t.ExpiresAt.After(now) {
				h.tokens[id] = t
			}
		}
	}
}

// saveTokens writes the token map to disk (0600). Caller holds h.mu.
func (h *Handler) saveTokens() {
	data, _ := json.MarshalIndent(h.tokens, "", "  ")
	os.WriteFile(h.tokensFile(), data, 0600)
}

// MintToken creates a share token for a viewer/editor role and returns the raw
// token (shown to the owner once) plus its id. ttl == 0 means no expiry.
func (h *Handler) MintToken(role Role, username string, ttl time.Duration) (raw, id string, err error) {
	if !MintableRole(role) {
		return "", "", errInvalidRole
	}
	rawBytes := make([]byte, 32)
	rand.Read(rawBytes)
	raw = hex.EncodeToString(rawBytes)
	idBytes := make([]byte, 8)
	rand.Read(idBytes)
	id = hex.EncodeToString(idBytes)

	t := shareToken{
		ID:        id,
		Hash:      hashToken(raw),
		Role:      role,
		Username:  sanitizeUsername(username),
		CreatedAt: time.Now(),
	}
	if ttl > 0 {
		t.ExpiresAt = time.Now().Add(ttl)
	}

	h.mu.Lock()
	h.tokens[id] = t
	h.saveTokens()
	h.mu.Unlock()
	return raw, id, nil
}

// RedeemToken exchanges a raw invite token for a new session carrying the
// token's role + username. Returns ok=false for unknown/expired tokens. The
// hash comparison is constant-time.
func (h *Handler) RedeemToken(raw string) (sessionToken string, role Role, username string, ok bool) {
	if raw == "" {
		return "", "", "", false
	}
	candidate := hashToken(raw)

	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for id, t := range h.tokens {
		if !t.ExpiresAt.IsZero() && t.ExpiresAt.Before(now) {
			delete(h.tokens, id) // prune expired lazily
			continue
		}
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(candidate)) == 1 {
			st := h.newSession(t.Username, t.Role)
			return st, t.Role, t.Username, true
		}
	}
	return "", "", "", false
}

// RevokeToken deletes a token by id. Returns true if it existed.
func (h *Handler) RevokeToken(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.tokens[id]; !ok {
		return false
	}
	delete(h.tokens, id)
	h.saveTokens()
	return true
}

// ListTokens returns the non-secret view of all live tokens.
func (h *Handler) ListTokens() []TokenInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]TokenInfo, 0, len(h.tokens))
	for _, t := range h.tokens {
		out = append(out, TokenInfo{
			ID:        t.ID,
			Role:      t.Role,
			Username:  t.Username,
			CreatedAt: t.CreatedAt,
			ExpiresAt: t.ExpiresAt,
		})
	}
	return out
}
