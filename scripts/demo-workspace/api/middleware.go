package main

import (
	"net/http"
	"strings"
)

// authMiddleware validates the Bearer JWT token in the Authorization header.
// In production this would verify the HMAC signature; here we just check presence.
func (a *App) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
			return
		}
		token := parts[1]
		if token == "" {
			http.Error(w, "empty token", http.StatusUnauthorized)
			return
		}
		// TODO: verify JWT signature using a.jwt secret
		next(w, r)
	}
}
