package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMiddlewareReadsSubprotocolToken guards the terminal WS auth path: clients
// that can't set headers pass the token as an "auth.<token>" subprotocol in
// Sec-WebSocket-Protocol. Regression test for the empty-terminal bug.
func TestMiddlewareReadsSubprotocolToken(t *testing.T) {
	h := newTestHandler(t)
	h.mu.Lock()
	tok := h.newSession("alice", RoleOwner)
	h.mu.Unlock()

	var called bool
	mw := h.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	// token only in the subprotocol header
	req := httptest.NewRequest(http.MethodGet, "/api/terminal", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "auth."+tok)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if !called || rec.Code == http.StatusUnauthorized {
		t.Fatalf("subprotocol token should authenticate: code=%d called=%v", rec.Code, called)
	}

	// comma-separated subprotocol list is also accepted
	called = false
	req2 := httptest.NewRequest(http.MethodGet, "/api/terminal", nil)
	req2.Header.Set("Sec-WebSocket-Protocol", "auth."+tok+", other")
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req2)
	if !called {
		t.Fatalf("comma-separated subprotocol token should authenticate: code=%d", rec2.Code)
	}

	// no token anywhere → 401
	called = false
	req3 := httptest.NewRequest(http.MethodGet, "/api/terminal", nil)
	rec3 := httptest.NewRecorder()
	mw.ServeHTTP(rec3, req3)
	if called || rec3.Code != http.StatusUnauthorized {
		t.Fatalf("missing token should 401: code=%d called=%v", rec3.Code, called)
	}

	// a bogus subprotocol token → 401
	called = false
	req4 := httptest.NewRequest(http.MethodGet, "/api/terminal", nil)
	req4.Header.Set("Sec-WebSocket-Protocol", "auth.not-a-real-token")
	rec4 := httptest.NewRecorder()
	mw.ServeHTTP(rec4, req4)
	if called || rec4.Code != http.StatusUnauthorized {
		t.Fatalf("bogus subprotocol token should 401: code=%d called=%v", rec4.Code, called)
	}
}
