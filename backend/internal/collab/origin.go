package collab

import (
	"log"
	"net/http"
	"strings"
)

// parseOrigins splits a space-separated frame_ancestors value into a set of
// allowed origin strings (e.g. "https://vulos.org"). Mirrors the terminal/lsp
// handlers so the collab socket honours the same embedding policy.
func parseOrigins(frameAncestors string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, o := range strings.Fields(frameAncestors) {
		set[o] = struct{}{}
	}
	return set
}

// checkOrigin enforces WebSocket origin validation: no Origin header (non-browser)
// or same-origin is allowed; otherwise the origin must be in the allowed set.
func checkOrigin(r *http.Request, allowedOrigins map[string]struct{}) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	if origin == scheme+"://"+r.Host {
		return true
	}
	if _, ok := allowedOrigins[origin]; ok {
		return true
	}
	log.Printf("[collab] rejected WebSocket upgrade from origin %q (host=%s)", origin, r.Host)
	return false
}
