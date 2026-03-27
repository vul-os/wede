//go:build embed_frontend

package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed dist
var frontendFS embed.FS

func newFrontendHandler() http.HandlerFunc {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		log.Fatal("embedded frontend dist not found:", err)
	}
	fileServer := http.FileServer(http.FS(distFS))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		if _, err := fs.Stat(distFS, path[1:]); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}
}
