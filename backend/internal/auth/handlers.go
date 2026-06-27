package auth

// ── Routes the integrator must wire in main.go ────────────────────────────────
//
// PUBLIC (no auth middleware):
//   POST /api/auth/redeem
//       → h.HandleRedeem
//
// OWNER-ONLY (wrap with h.Middleware then h.RequireOwner):
//   POST   /api/auth/tokens          → h.HandleMintToken
//   GET    /api/auth/tokens          → h.HandleListTokens
//   DELETE /api/auth/tokens/{id}     → h.HandleRevokeToken
//
// EDITOR+ (wrap mutating/terminal/collab-doc routes with h.Middleware then
// h.RequireEditor so that viewer-role sessions are rejected with 403):
//   PUT    /api/files/write          (and all write/delete/create/rename/copy)
//   POST   /api/files/create
//   DELETE /api/files/delete
//   POST   /api/files/rename
//   POST   /api/files/copy
//   POST   /api/files/format
//   POST   /api/git/stage            (and all git mutation routes)
//   POST   /api/git/unstage
//   POST   /api/git/commit
//   POST   /api/git/checkout
//   POST   /api/git/branch
//   POST   /api/git/branch/delete
//   POST   /api/git/fetch
//   POST   /api/git/pull
//   POST   /api/git/push
//   POST   /api/git/discard
//   POST   /api/git/stash
//   POST   /api/git/stash/pop
//   POST   /api/git/stash/drop
//   POST   /api/git/conflict/resolve
//   POST   /api/git/remotes/add
//   POST   /api/git/remotes/remove
//   POST   /api/git/stage-hunk
//   GET    /api/terminal             (terminal WebSocket)
//   GET    /api/workspaces/{id}/terminal
//   GET    /api/workspaces/{id}/doc/{room...}  (CRDT collaborative-doc WebSocket)
//
// READ-ONLY routes need only h.Middleware (all roles including viewer pass):
//   GET /api/files, /api/files/read, /api/files/tree, /api/git/status, etc.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"encoding/json"
	"net"
	"net/http"
	"time"
)

// Invite-redemption rate limit: at most redeemMaxPerWindow attempts per client IP
// per redeemWindow. This throttles brute-force / abuse of the public redeem
// endpoint without affecting legitimate one-shot redemptions.
const (
	redeemMaxPerWindow = 10
	redeemWindow       = time.Minute
)

type redeemBucket struct {
	count int
	reset time.Time
}

// allowRedeem reports whether a redeem attempt from ip is within the rate limit,
// counting the attempt when allowed.
func (h *Handler) allowRedeem(ip string) bool {
	h.redeemMu.Lock()
	defer h.redeemMu.Unlock()
	now := time.Now()
	b, ok := h.redeemHits[ip]
	if !ok || now.After(b.reset) {
		h.redeemHits[ip] = &redeemBucket{count: 1, reset: now.Add(redeemWindow)}
		return true
	}
	if b.count >= redeemMaxPerWindow {
		return false
	}
	b.count++
	return true
}

// clientIP extracts the remote IP (without port) from the request.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// HandleMintToken creates a share token for a viewer or editor role.
//
// POST /api/auth/tokens  (RequireOwner)
// Request body: {"role":"viewer"|"editor", "username":"...", "ttlHours":0}
//
//	ttlHours is optional; 0 or absent means the token never expires.
//
// Response 200: {"raw":"<token>", "id":"<id>", "inviteUrl":"?invite=<token>"}
// Response 400: {"error":"..."} on invalid role.
func (h *Handler) HandleMintToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Role     Role    `json:"role"`
		Username string  `json:"username"`
		TTLHours float64 `json:"ttlHours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if !MintableRole(body.Role) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "role must be viewer or editor"})
		return
	}

	var ttl time.Duration
	if body.TTLHours > 0 {
		ttl = time.Duration(body.TTLHours * float64(time.Hour))
	}

	raw, id, err := h.MintToken(body.Role, body.Username, ttl)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"raw":       raw,
		"id":        id,
		"inviteUrl": "?invite=" + raw,
	})
}

// HandleListTokens returns the non-secret view of all live share tokens.
//
// GET /api/auth/tokens  (RequireOwner)
// Response 200: {"tokens": [{"id":"...","role":"...","username":"...","createdAt":"...","expiresAt":"..."},...]}
func (h *Handler) HandleListTokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tokens": h.ListTokens(),
	})
}

// HandleRevokeToken deletes a share token by ID.
//
// DELETE /api/auth/tokens/{id}  (RequireOwner)
// Response 200: {"status":"ok"}
// Response 404: {"error":"not found"} if no such token.
func (h *Handler) HandleRevokeToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := r.PathValue("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing id path parameter"})
		return
	}

	if !h.RevokeToken(id) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleRedeem exchanges a raw invite token for a new authenticated session.
//
// POST /api/auth/redeem  (public — no auth middleware required)
// Request body: {"token":"<raw-invite-token>"}
// Response 200: {"token":"<session-token>","role":"viewer"|"editor","username":"..."}
// Response 401: {"error":"invalid or expired token"}
func (h *Handler) HandleRedeem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.allowRedeem(clientIP(r)) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many attempts; try again later"})
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	sessionToken, role, username, ok := h.RedeemToken(body.Token)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token":    sessionToken,
		"role":     string(role),
		"username": username,
	})
}
