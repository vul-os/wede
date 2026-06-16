package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"wede/backend/internal/auth"
	"wede/backend/internal/config"
	"wede/backend/internal/files"
	"wede/backend/internal/filewatcher"
	"wede/backend/internal/git"
	"wede/backend/internal/lsp"
	"wede/backend/internal/search"
	"wede/backend/internal/terminal"
	"wede/backend/internal/workspace"
)

// Version is injected at build time via -ldflags "-X main.Version=vX.Y.Z".
// Falls back to "dev" when running without ldflags (e.g. go run).
var Version = "dev"

// securityHeaders wraps a handler and injects security headers on every
// response.  Frame-embedding behaviour is controlled by cfg.FrameAncestors:
//
//   - Empty (default): emit X-Frame-Options: DENY and
//     Content-Security-Policy: frame-ancestors 'self'  — blocks all
//     cross-origin embedding (safe standalone default).
//   - Non-empty: emit Content-Security-Policy: frame-ancestors <value>
//     and omit X-Frame-Options so the CSP directive takes sole effect.
//     This allows the Vulos OS shell (or any other trusted origin) to
//     embed wede in an iframe.  Example value: "https://vulos.org".
func securityHeaders(cfg *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.FrameAncestors != "" {
			// Cross-origin embedding allowed for the listed origins.
			w.Header().Set("Content-Security-Policy", "frame-ancestors "+cfg.FrameAncestors)
		} else {
			// Default: deny all cross-origin framing.
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func main() {
	portFlag := flag.String("port", "", "Override port (default: from config or 9090)")
	pFlag := flag.String("p", "", "Override port (shorthand)")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("wede", Version)
		os.Exit(0)
	}

	cfg := config.Load()

	if *portFlag != "" {
		cfg.Port = *portFlag
	} else if *pFlag != "" {
		cfg.Port = *pFlag
	}

	var defaultPath string
	args := flag.Args()
	if len(args) > 0 {
		defaultPath = args[0]
	}

	ws := workspace.New(defaultPath)

	authHandler := auth.New(cfg.Password)
	fileHandler := files.New(ws)
	gitHandler := git.New(ws)
	termHandler := terminal.New(ws, cfg.FrameAncestors)
	searchHandler := search.New(ws)
	watchHandler := filewatcher.New(ws)
	lspHandler := lsp.New(ws, cfg.FrameAncestors)

	mux := http.NewServeMux()

	// Public auth routes
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/auth/check", authHandler.Check)
	mux.Handle("DELETE /api/auth/logout", authHandler.Middleware(http.HandlerFunc(authHandler.Logout)))

	// Protected API routes
	protected := http.NewServeMux()

	protected.HandleFunc("GET /api/workspace", ws.HandleGet)
	protected.HandleFunc("POST /api/workspace/open", ws.HandleOpen)
	protected.HandleFunc("GET /api/workspace/browse", ws.HandleBrowse)

	protected.HandleFunc("GET /api/files", fileHandler.List)
	protected.HandleFunc("GET /api/files/read", fileHandler.Read)
	protected.HandleFunc("PUT /api/files/write", fileHandler.Write)
	protected.HandleFunc("POST /api/files/create", fileHandler.Create)
	protected.HandleFunc("DELETE /api/files/delete", fileHandler.Delete)
	protected.HandleFunc("POST /api/files/rename", fileHandler.Rename)
	protected.HandleFunc("POST /api/files/copy", fileHandler.Copy)

	protected.HandleFunc("GET /api/git/status", gitHandler.Status)
	protected.HandleFunc("GET /api/git/log", gitHandler.Log)
	protected.HandleFunc("GET /api/git/diff", gitHandler.Diff)
	protected.HandleFunc("POST /api/git/stage", gitHandler.Stage)
	protected.HandleFunc("POST /api/git/unstage", gitHandler.Unstage)
	protected.HandleFunc("POST /api/git/commit", gitHandler.Commit)
	protected.HandleFunc("GET /api/git/branches", gitHandler.Branches)
	protected.HandleFunc("POST /api/git/checkout", gitHandler.Checkout)
	protected.HandleFunc("POST /api/git/branch", gitHandler.CreateBranch)
	protected.HandleFunc("POST /api/git/fetch", gitHandler.Fetch)
	protected.HandleFunc("POST /api/git/pull", gitHandler.Pull)
	protected.HandleFunc("POST /api/git/push", gitHandler.Push)
	protected.HandleFunc("GET /api/git/remotes", gitHandler.Remotes)
	protected.HandleFunc("POST /api/git/discard", gitHandler.Discard)
	protected.HandleFunc("GET /api/git/stash", gitHandler.StashList)
	protected.HandleFunc("POST /api/git/stash", gitHandler.StashPush)
	protected.HandleFunc("POST /api/git/stash/pop", gitHandler.StashPop)
	protected.HandleFunc("POST /api/git/stash/drop", gitHandler.StashDrop)
	protected.HandleFunc("GET /api/git/commit-diff", gitHandler.CommitDiff)
	protected.HandleFunc("POST /api/files/format", fileHandler.Format)

	protected.HandleFunc("GET /api/search", searchHandler.Search)
	protected.HandleFunc("GET /api/search/replace-preview", searchHandler.ReplacePreview)
	protected.HandleFunc("POST /api/search/replace", searchHandler.ReplaceApply)

	protected.HandleFunc("GET /api/git/conflict", gitHandler.ConflictRegions)
	protected.HandleFunc("POST /api/git/conflict/resolve", gitHandler.ConflictResolve)
	protected.HandleFunc("POST /api/git/remotes/add", gitHandler.RemoteAdd)
	protected.HandleFunc("POST /api/git/remotes/remove", gitHandler.RemoteRemove)
	protected.HandleFunc("POST /api/git/stage-hunk", gitHandler.StageHunk)

	protected.HandleFunc("GET /api/watch", watchHandler.HandleSSE)

	protected.HandleFunc("GET /api/terminal/sessions", termHandler.ListSessions)
	protected.HandleFunc("GET /api/terminal", termHandler.HandleWS)

	protected.HandleFunc("GET /api/lsp/available", lspHandler.HandleAvailable)
	protected.HandleFunc("GET /api/lsp", lspHandler.HandleWS)

	mux.Handle("/api/", authHandler.Middleware(protected))

	// Frontend handler - provided by frontend_embed.go or frontend_dev.go
	frontendHandler := newFrontendHandler()
	mux.HandleFunc("/", frontendHandler)

	host := cfg.Host
	if host == "" {
		host = "127.0.0.1" // safe default: loopback only
	}
	addr := host + ":" + cfg.Port
	log.Printf("wede %s running on http://%s", Version, addr)
	if ws.HasWorkspace() {
		log.Printf("workspace: %s", ws.Current())
	} else {
		log.Printf("no default workspace - open a folder from the UI")
	}
	if cfg.FrameAncestors != "" {
		log.Printf("embed mode: frame-ancestors %s", cfg.FrameAncestors)
	}
	if len(os.Args) == 1 {
		log.Printf("tip: run with a path to open directly: ./wede /path/to/project")
	}

	if err := http.ListenAndServe(addr, securityHeaders(cfg, mux)); err != nil {
		log.Fatal(err)
	}
}
