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

// licensesTxt returns the third-party notices. In dev builds it reads the
// generated file from the repo root (THIRD-PARTY-NOTICES.txt) so /licenses.txt
// works without a full embed build; falls back to site/licenses.txt. Returns
// nil if neither exists (run scripts/gen-notices.sh).
func licensesTxt() []byte {
	dir := findSite()
	if dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, "licenses.txt")); err == nil {
			return data
		}
		// site/ sits next to the repo root; the canonical file lives at the root.
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "THIRD-PARTY-NOTICES.txt")); err == nil {
			return data
		}
	}
	return nil
}
