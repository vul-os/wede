package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Context key for session role ──────────────────────────────────────────────

type contextKey int

const (
	roleCtxKey contextKey = iota
	usernameCtxKey
)

// GetRole returns the resolved Role that Middleware attached to the request
// context. Returns "" if Middleware has not run (shouldn't happen in normal
// operation — treat it as an unauthenticated call).
func GetRole(r *http.Request) Role {
	if v, ok := r.Context().Value(roleCtxKey).(Role); ok {
		return v
	}
	return ""
}

// GetUsername returns the authenticated session's display username that
// Middleware attached to the request context. Handlers should trust this value
// rather than a client-supplied query parameter, so a peer cannot impersonate
// another user. Returns "" if Middleware has not run or the session had no name.
func GetUsername(r *http.Request) string {
	if v, ok := r.Context().Value(usernameCtxKey).(string); ok {
		return v
	}
	return ""
}

// sessionEntry holds a token's creation timestamp for idle-TTL enforcement, the
// display username chosen at (or after) login, and the session's role.
type sessionEntry struct {
	CreatedAt time.Time `json:"created_at"`
	Username  string    `json:"username,omitempty"`
	Role      Role      `json:"role,omitempty"` // empty (legacy) → owner
}

// maxUsernameLen bounds a stored username.
const maxUsernameLen = 32

// sanitizeUsername trims and length-caps a chosen username. An empty result is
// allowed; the presence layer substitutes a default ("anon").
func sanitizeUsername(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxUsernameLen {
		s = s[:maxUsernameLen]
	}
	return s
}

// SessionTTL is the idle lifetime of a session token (24 hours).
const SessionTTL = 24 * time.Hour

// lockoutFile is the basename of the file used to persist lockout state.
const lockoutFile = "lockout.json"

// lockoutState is persisted so brute-force lockout survives server restarts.
type lockoutState struct {
	Attempts int  `json:"attempts"`
	Locked   bool `json:"locked"`
}

type Handler struct {
	password    string
	mu          sync.Mutex
	attempts    int
	locked      bool
	maxAttempts int
	sessions    map[string]sessionEntry
	tokens      map[string]shareToken
	dataDir     string

	redeemMu   sync.Mutex
	redeemHits map[string]*redeemBucket // per-IP rate limiting for invite redemption
}

func New(password string) *Handler {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".wede")
	os.MkdirAll(dataDir, 0700)

	h := &Handler{
		password:    password,
		maxAttempts: 3,
		sessions:    make(map[string]sessionEntry),
		tokens:      make(map[string]shareToken),
		dataDir:     dataDir,
		redeemHits:  make(map[string]*redeemBucket),
	}
	h.loadSessions()
	h.loadLockout()
	h.loadTokens()
	return h
}

func (h *Handler) sessionsFile() string {
	return filepath.Join(h.dataDir, "sessions.json")
}

func (h *Handler) lockoutFilePath() string {
	return filepath.Join(h.dataDir, lockoutFile)
}

// loadSessions reads persisted sessions and prunes expired ones. Sessions are
// keyed by the SHA-256 hash of the token, so the stored map keys are hashes and
// can be loaded directly.
func (h *Handler) loadSessions() {
	data, err := os.ReadFile(h.sessionsFile())
	if err != nil {
		return
	}
	var stored map[string]sessionEntry
	if json.Unmarshal(data, &stored) == nil {
		now := time.Now()
		for k, e := range stored {
			if now.Sub(e.CreatedAt) < SessionTTL {
				h.sessions[k] = e
			}
		}
	}
}

// saveSessions writes the current session map to disk (must be called with mu held or
// after acquiring it if the caller doesn't hold it). The map is keyed by token
// hash, so no raw session token is ever persisted: an attacker who reads
// ~/.wede/sessions.json cannot recover a usable token.
func (h *Handler) saveSessions() {
	data, _ := json.Marshal(h.sessions)
	os.WriteFile(h.sessionsFile(), data, 0600)
}

// loadLockout restores attempt count and locked state from disk.
func (h *Handler) loadLockout() {
	data, err := os.ReadFile(h.lockoutFilePath())
	if err != nil {
		return
	}
	var s lockoutState
	if json.Unmarshal(data, &s) == nil {
		h.attempts = s.Attempts
		h.locked = s.Locked
	}
}

// saveLockout persists the current lockout state (must be called with mu held or
// immediately after the critical section that changed the state).
func (h *Handler) saveLockout() {
	data, _ := json.Marshal(lockoutState{Attempts: h.attempts, Locked: h.locked})
	os.WriteFile(h.lockoutFilePath(), data, 0600)
}

// pruneExpired removes sessions whose idle TTL has elapsed. mu must be held.
func (h *Handler) pruneExpired() {
	now := time.Now()
	for k, e := range h.sessions {
		if now.Sub(e.CreatedAt) >= SessionTTL {
			delete(h.sessions, k)
		}
	}
}

// validSession checks whether token is present and not expired. mu must be held.
// Lookups are keyed by the token's hash so the raw token is never used as a map
// key (and thus never persisted).
func (h *Handler) validSession(token string) bool {
	key := hashToken(token)
	e, ok := h.sessions[key]
	if !ok {
		return false
	}
	if time.Since(e.CreatedAt) >= SessionTTL {
		delete(h.sessions, key)
		return false
	}
	return true
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	h.mu.Lock()
	if h.locked {
		h.mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":   "locked",
			"message": "Too many failed attempts. Delete ~/.wede/lockout.json to unlock.",
		})
		return
	}
	h.mu.Unlock()

	var body struct {
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(h.password)) != 1 {
		h.attempts++
		remaining := h.maxAttempts - h.attempts
		if remaining <= 0 {
			h.locked = true
			h.saveLockout()
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{
				"error":   "locked",
				"message": "Too many failed attempts. Delete ~/.wede/lockout.json to unlock.",
			})
			return
		}
		h.saveLockout()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error":     "wrong_password",
			"remaining": remaining,
		})
		return
	}

	h.attempts = 0
	h.saveLockout()

	// Owner-password login → owner role.
	username := sanitizeUsername(body.Username)
	sessionToken := h.newSession(username, RoleOwner)

	http.SetCookie(w, &http.Cookie{
		Name:     "wede_session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   int(SessionTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	json.NewEncoder(w).Encode(map[string]string{
		"token":    sessionToken,
		"username": username,
		"role":     string(RoleOwner),
	})
}

// newSession creates, stores, and persists a session, returning its token. The
// caller must hold h.mu.
func (h *Handler) newSession(username string, role Role) string {
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	h.sessions[hashToken(token)] = sessionEntry{
		CreatedAt: time.Now(),
		Username:  sanitizeUsername(username),
		Role:      role,
	}
	h.pruneExpired()
	h.saveSessions()
	return token
}

// Role returns the role for a valid session token, or "" if unknown/expired.
// A session with no stored role (created before Wave 9) is treated as owner.
func (h *Handler) Role(token string) Role {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.sessions[hashToken(token)]; ok && time.Since(e.CreatedAt) < SessionTTL {
		return normalizeRole(e.Role)
	}
	return ""
}

// SetUsername updates the display username on the caller's session. Mounted
// behind the auth middleware (POST /api/auth/username).
func (h *Handler) SetUsername(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	var body struct {
		Username string `json:"username"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck — empty body => empty name
	name := sanitizeUsername(body.Username)

	key := hashToken(token)
	h.mu.Lock()
	e, ok := h.sessions[key]
	if ok {
		e.Username = name
		h.sessions[key] = e
		h.saveSessions()
	}
	h.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"username": name})
}

// IsAuthenticated reports whether r carries a valid session credential,
// checking the Authorization header, ?token= query param, and the
// wede_session cookie (set on browser login).
func (h *Handler) IsAuthenticated(r *http.Request) bool {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		if c, err := r.Cookie("wede_session"); err == nil {
			token = c.Value
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.validSession(token)
}

// Username returns the username for a valid session token, or "" if the token is
// unknown/expired. Used by the collab layer to attribute presence.
func (h *Handler) Username(token string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.sessions[hashToken(token)]; ok && time.Since(e.CreatedAt) < SessionTTL {
		return e.Username
	}
	return ""
}

// Logout revokes the caller's session token server-side.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	h.mu.Lock()
	delete(h.sessions, hashToken(token))
	h.saveSessions()
	h.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "wede_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	key := hashToken(token)
	h.mu.Lock()
	valid := h.validSession(token)
	locked := h.locked
	username := ""
	role := Role("")
	if valid {
		username = h.sessions[key].Username
		role = normalizeRole(h.sessions[key].Role)
	}
	h.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{
		"authenticated": valid,
		"locked":        locked,
		"username":      username,
		"role":          string(role),
	})
}

func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		// WebSocket clients (terminal) can't set headers, so they pass the token
		// as an "auth.<token>" subprotocol in Sec-WebSocket-Protocol. Read it here
		// so the token never appears in URLs or access logs.
		if token == "" {
			for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
				if p = strings.TrimSpace(p); strings.HasPrefix(p, "auth.") {
					token = strings.TrimPrefix(p, "auth.")
					break
				}
			}
		}

		key := hashToken(token)
		h.mu.Lock()
		valid := h.validSession(token)
		role := Role("")
		username := ""
		if valid {
			role = normalizeRole(h.sessions[key].Role)
			username = h.sessions[key].Username
		}
		h.mu.Unlock()

		if !valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		ctx := context.WithValue(r.Context(), roleCtxKey, role)
		ctx = context.WithValue(ctx, usernameCtxKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireEditor rejects requests from viewer-role sessions with 403 Forbidden.
// Owner and editor sessions pass through. Must be composed after Middleware so
// that the role is already in the request context.
func (h *Handler) RequireEditor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetRole(r)
		if !role.CanMutate() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: editor or owner role required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireOwner rejects requests from non-owner sessions with 403 Forbidden.
// Must be composed after Middleware so that the role is already in the request
// context.
func (h *Handler) RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetRole(r)
		if role != RoleOwner {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: owner role required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
