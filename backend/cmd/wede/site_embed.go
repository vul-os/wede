//go:build embed_frontend

package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

// siteFS holds the standalone marketing site (landing + docs viewer) served for
// wede.vulos.org. `npm run build:all` copies the repo-root site/ into this
// directory before the embedded build, mirroring how the frontend dist is
// embedded. Plain `go build ./...` uses site_dev.go (no embed) instead.
//
//go:embed site
var siteFS embed.FS

// newSiteHandler serves the embedded marketing site, or nil if unavailable.
func newSiteHandler() http.Handler {
	sub, err := fs.Sub(siteFS, "site")
	if err != nil {
		log.Printf("embedded marketing site not found: %v", err)
		return nil
	}
	return http.FileServer(http.FS(sub))
}

// siteIndexHTML returns the raw bytes of site/index.html from the embedded FS,
// or nil if unavailable. Used by the root handler to serve the landing page.
func siteIndexHTML() []byte {
	data, err := fs.ReadFile(siteFS, "site/index.html")
	if err != nil {
		return nil
	}
	return data
}
