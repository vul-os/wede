package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestHandler builds a Handler whose state writes to an isolated temp dir, so
// tests never touch the real ~/.wede.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h := New("secret")
	h.dataDir = t.TempDir()
	h.sessions = make(map[string]sessionEntry) // discard anything loaded from home
	h.tokens = make(map[string]shareToken)     // isolate from real ~/.wede/tokens.json
	h.attempts = 0
	h.locked = false
	return h
}

func login(t *testing.T, h *Handler, password, username string) map[string]any {
	t.Helper()
	body := `{"password":"` + password + `","username":"` + username + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	var out map[string]any
	json.NewDecoder(rec.Result().Body).Decode(&out)
	return out
}

func TestLoginStoresAndReturnsUsername(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "alice")

	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("no token returned: %+v", out)
	}
	if out["username"] != "alice" {
		t.Errorf("login username = %v, want alice", out["username"])
	}

	// Check echoes the username for the valid session.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	h.Check(rec, req)
	var chk map[string]any
	json.NewDecoder(rec.Result().Body).Decode(&chk)
	if chk["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", chk["authenticated"])
	}
	if chk["username"] != "alice" {
		t.Errorf("check username = %v, want alice", chk["username"])
	}

	// Username() helper resolves the same.
	if got := h.Username(token); got != "alice" {
		t.Errorf("Username() = %q, want alice", got)
	}
}

func TestLoginWithoutUsernameIsEmpty(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "")
	if out["username"] != "" {
		t.Errorf("username = %v, want empty", out["username"])
	}
	if out["token"] == "" {
		t.Error("expected a token even without username")
	}
}

func TestUsernameIsTrimmedAndCapped(t *testing.T) {
	h := newTestHandler(t)
	long := strings.Repeat("x", 50)
	out := login(t, h, "secret", "  "+long+"  ")
	got, _ := out["username"].(string)
	if len(got) != maxUsernameLen {
		t.Errorf("username len = %d, want %d (capped)", len(got), maxUsernameLen)
	}
}

func TestSetUsernameUpdatesSession(t *testing.T) {
	h := newTestHandler(t)
	out := login(t, h, "secret", "alice")
	token := out["token"].(string)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/username", strings.NewReader(`{"username":"alice2"}`))
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	h.SetUsername(rec, req)

	if h.Username(token) != "alice2" {
		t.Errorf("after SetUsername, Username() = %q, want alice2", h.Username(token))
	}
}

func TestSetUsernameRejectsUnknownToken(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/username", strings.NewReader(`{"username":"x"}`))
	req.Header.Set("Authorization", "bogus")
	rec := httptest.NewRecorder()
	h.SetUsername(rec, req)
	if rec.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Result().StatusCode)
	}
}
