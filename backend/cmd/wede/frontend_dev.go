//go:build !embed_frontend

package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func findDist() string {
	cwd, _ := os.Getwd()
	// Check cwd and walk up to find dist/ (handles running from backend/)
	dir := cwd
	for {
		candidate := filepath.Join(dir, "dist")
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

func newFrontendHandler() http.HandlerFunc {
	distDir := findDist()

	if distDir == "" {
		log.Printf("[dev] no dist/ found - use Vite dev server (localhost:5173) or run 'npm run build'")
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!doctype html><html><body style="background:#111827;color:#f1f5f9;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
				<div style="text-align:center">
					<h2>wede dev mode</h2>
					<p>Frontend is served by Vite on <a href="http://localhost:5173" style="color:#60a5fa">localhost:5173</a></p>
					<p style="color:#94a3b8;font-size:14px">Run <code style="color:#60a5fa">npm run dev</code> in another terminal</p>
				</div>
			</body></html>`))
		}
	}

	log.Printf("[dev] serving frontend from %s", distDir)
	fileServer := http.FileServer(http.Dir(distDir))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		if _, err := os.Stat(filepath.Join(distDir, filepath.Clean(path))); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}
}
