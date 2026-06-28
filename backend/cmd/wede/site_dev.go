//go:build !embed_frontend

package main

import (
	"net/http"
	"os"
	"path/filepath"
)

// newSiteHandler serves the marketing site from the repo-root site/ directory
// when present (dev / non-embedded builds). Returns nil if no site/ is found,
// in which case the /site/ route is simply not registered.
func newSiteHandler() http.Handler {
	dir := findSite()
	if dir == "" {
		return nil
	}
	return http.FileServer(http.Dir(dir))
}

// findSite walks up from the working directory looking for a site/ directory.
func findSite() string {
	cwd, _ := os.Getwd()
	dir := cwd
	for {
		candidate := filepath.Join(dir, "site")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// siteIndexHTML returns the raw bytes of site/index.html found on disk,
// or nil if unavailable. Used by the root handler to serve the landing page.
func siteIndexHTML() []byte {
	dir := findSite()
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return nil
	}
	return data
}
