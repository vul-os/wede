package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"wede/backend/internal/auth"
	"wede/backend/internal/config"
	"wede/backend/internal/files"
	"wede/backend/internal/git"
	"wede/backend/internal/workspace"
	"wede/backend/internal/search"
	"wede/backend/internal/tunnel"
	"wede/backend/internal/folder"
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

	rootFolder := folder.New(defaultPath)

	// Workspace registry: the multi-project backbone. The boot workspace is adopted
	// as the default workspace so the solo-user case works with zero setup; additional
	// projects can be opened as further workspaces via /api/workspaces.
	wsMgr := workspace.NewManager(cfg.FrameAncestors)
	defaultWorkspace := wsMgr.Register("default", rootFolder)

	authHandler := auth.New(cfg.Password)
	tunnelMgr := tunnel.New(cfg.Port) // optional frp public tunnel (owner-only)
	fileHandler := files.New(rootFolder)
	gitHandler := git.New(rootFolder)
	searchHandler := search.New(rootFolder)

	mux := http.NewServeMux()

	// Public auth routes
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/auth/check", authHandler.Check)
	mux.HandleFunc("POST /api/auth/redeem", authHandler.HandleRedeem) // public: exchange an invite token for a session
	mux.Handle("DELETE /api/auth/logout", authHandler.Middleware(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("POST /api/auth/username", authHandler.Middleware(http.HandlerFunc(authHandler.SetUsername)))

	// Protected API routes
	protected := http.NewServeMux()

	// Owner-only share-token management (Middleware runs first via the /api/ mount,
	// so RequireOwner can read the role from context).
	re := authHandler.RequireEditor // wrap mutating handlers: viewers get 403
	ro := authHandler.RequireOwner
	protected.Handle("POST /api/auth/tokens", ro(http.HandlerFunc(authHandler.HandleMintToken)))
	protected.Handle("GET /api/auth/tokens", ro(http.HandlerFunc(authHandler.HandleListTokens)))
	protected.Handle("DELETE /api/auth/tokens/{id}", ro(http.HandlerFunc(authHandler.HandleRevokeToken)))

	// Public-tunnel (frp) management — owner-only. Lets an owner expose a
	// loopback-bound wede via their own frps relay; wede auto-detects frpc,
	// generates its config, runs it, and reports the live public URL.
	protected.Handle("GET /api/tunnel", ro(http.HandlerFunc(tunnelMgr.HandleGet)))
	protected.Handle("PUT /api/tunnel/config", ro(http.HandlerFunc(tunnelMgr.HandleSetConfig)))
	protected.Handle("POST /api/tunnel/start", ro(http.HandlerFunc(tunnelMgr.HandleStart)))
	protected.Handle("POST /api/tunnel/stop", ro(http.HandlerFunc(tunnelMgr.HandleStop)))

	protected.HandleFunc("GET /api/folder", rootFolder.HandleGet)
	protected.HandleFunc("POST /api/folder/open", rootFolder.HandleOpen)
	protected.HandleFunc("GET /api/folder/browse", rootFolder.HandleBrowse)

	// Workspace registry endpoints (multi-project backbone). Per-workspace scoping of the
	// file/git/etc. routes under /api/workspaces/{id}/... is layered on in later slices.
	protected.HandleFunc("GET /api/workspaces", wsMgr.HandleList)
	protected.HandleFunc("POST /api/workspaces", wsMgr.HandleCreate)
	protected.HandleFunc("GET /api/workspaces/{id}", wsMgr.HandleGet)
	protected.HandleFunc("DELETE /api/workspaces/{id}", wsMgr.HandleClose)

	// Per-workspace service routes. Each resolves {id} -> Workspace and dispatches to a
	// handler bound to that workspace's isolated workspace. The legacy /api/files,
	// /api/git, /api/search routes below remain (default workspace) until the frontend
	// is migrated to workspace-scoped paths.
	rs := wsMgr.Scoped
	// files
	protected.HandleFunc("GET /api/workspaces/{id}/files", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().List }))
	protected.HandleFunc("GET /api/workspaces/{id}/files/tree", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Tree }))
	protected.HandleFunc("GET /api/workspaces/{id}/files/read", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Read }))
	protected.HandleFunc("PUT /api/workspaces/{id}/files/write", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Write }))
	protected.HandleFunc("POST /api/workspaces/{id}/files/create", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Create }))
	protected.HandleFunc("DELETE /api/workspaces/{id}/files/delete", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Delete }))
	protected.HandleFunc("POST /api/workspaces/{id}/files/rename", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Rename }))
	protected.HandleFunc("POST /api/workspaces/{id}/files/copy", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Copy }))
	protected.HandleFunc("POST /api/workspaces/{id}/files/format", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Format }))
	// git
	protected.HandleFunc("GET /api/workspaces/{id}/git/status", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Status }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/log", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Log }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/diff", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Diff }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/stage", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Stage }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/unstage", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Unstage }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/commit", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Commit }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/branches", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Branches }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/checkout", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Checkout }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/branch", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().CreateBranch }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/branch/delete", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().DeleteBranch }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/fetch", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Fetch }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/pull", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Pull }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/push", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Push }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/remotes", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Remotes }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/discard", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Discard }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/stash", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashList }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/stash", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashPush }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/stash/pop", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashPop }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/stash/drop", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashDrop }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/commit-diff", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().CommitDiff }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/conflict", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().ConflictRegions }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/conflict/resolve", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().ConflictResolve }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/remotes/add", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().RemoteAdd }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/remotes/remove", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().RemoteRemove }))
	protected.HandleFunc("POST /api/workspaces/{id}/git/stage-hunk", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StageHunk }))
	// search
	protected.HandleFunc("GET /api/workspaces/{id}/search", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().Search }))
	protected.HandleFunc("GET /api/workspaces/{id}/search/replace-preview", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().ReplacePreview }))
	protected.HandleFunc("POST /api/workspaces/{id}/search/replace", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().ReplaceApply }))
	// file-watch SSE (one fsnotify watcher per workspace)
	protected.HandleFunc("GET /api/workspaces/{id}/watch", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Watcher().HandleSSE }))
	// terminal (shared PTY sessions per workspace) + lsp (language servers per workspace)
	protected.HandleFunc("GET /api/workspaces/{id}/terminal/sessions", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Terminal().ListSessions }))
	protected.HandleFunc("GET /api/workspaces/{id}/terminal", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Terminal().HandleWS }))
	protected.HandleFunc("GET /api/workspaces/{id}/lsp/available", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.LSP().HandleAvailable }))
	protected.HandleFunc("GET /api/workspaces/{id}/lsp", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.LSP().HandleWS }))
	// collaboration socket (presence: roster + cursors)
	protected.HandleFunc("GET /api/workspaces/{id}/collab", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Collab().HandleWS }))
	// CRDT document sync+awareness (ygo provider). {room...} is the file's
	// workspace-relative path; the provider reads it via r.PathValue("workspace").
	protected.HandleFunc("GET /api/workspaces/{id}/doc/{room...}", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.DocServer().ServeHTTP }))
	// git tools (blame/tags read-only; cherry-pick/revert/reset/merge/tag mutate -> RequireEditor)
	protected.HandleFunc("GET /api/workspaces/{id}/git/blame", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Blame }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/tags", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Tags }))
	protected.Handle("POST /api/workspaces/{id}/git/cherry-pick", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().CherryPick })))
	protected.Handle("POST /api/workspaces/{id}/git/revert", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Revert })))
	protected.Handle("POST /api/workspaces/{id}/git/reset", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Reset })))
	protected.Handle("POST /api/workspaces/{id}/git/merge", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Merge })))
	protected.Handle("POST /api/workspaces/{id}/git/tag", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().TagCreate })))
	protected.Handle("POST /api/workspaces/{id}/git/tag/delete", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().TagDelete })))
	// comprehensive search: filename mode
	protected.HandleFunc("GET /api/workspaces/{id}/search/files", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().SearchFiles }))
	// workspace chat (live + .wede/chat.md + git activity); ?channel=public|private
	protected.HandleFunc("GET /api/workspaces/{id}/chat", rs(func(ws *workspace.Workspace) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { ws.Chat(r.URL.Query().Get("channel")).HandleWS(w, r) }
	}))

	protected.HandleFunc("GET /api/files", fileHandler.List)
	protected.HandleFunc("GET /api/files/tree", fileHandler.Tree)
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
	protected.HandleFunc("POST /api/git/branch/delete", gitHandler.DeleteBranch)
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
	protected.HandleFunc("GET /api/search/files", searchHandler.SearchFiles)
	protected.HandleFunc("GET /api/search/replace-preview", searchHandler.ReplacePreview)
	protected.HandleFunc("POST /api/search/replace", searchHandler.ReplaceApply)

	protected.HandleFunc("GET /api/git/conflict", gitHandler.ConflictRegions)
	protected.HandleFunc("POST /api/git/conflict/resolve", gitHandler.ConflictResolve)
	protected.HandleFunc("POST /api/git/remotes/add", gitHandler.RemoteAdd)
	protected.HandleFunc("POST /api/git/remotes/remove", gitHandler.RemoteRemove)
	protected.HandleFunc("POST /api/git/stage-hunk", gitHandler.StageHunk)
	// git tools (legacy default-workspace routes used by the current frontend GitPanel)
	protected.HandleFunc("GET /api/git/blame", gitHandler.Blame)
	protected.HandleFunc("GET /api/git/tags", gitHandler.Tags)
	protected.Handle("POST /api/git/cherry-pick", re(http.HandlerFunc(gitHandler.CherryPick)))
	protected.Handle("POST /api/git/revert", re(http.HandlerFunc(gitHandler.Revert)))
	protected.Handle("POST /api/git/reset", re(http.HandlerFunc(gitHandler.Reset)))
	protected.Handle("POST /api/git/merge", re(http.HandlerFunc(gitHandler.Merge)))
	protected.Handle("POST /api/git/tag", re(http.HandlerFunc(gitHandler.TagCreate)))
	protected.Handle("POST /api/git/tag/delete", re(http.HandlerFunc(gitHandler.TagDelete)))

	protected.HandleFunc("GET /api/watch", defaultWorkspace.Watcher().HandleSSE)

	protected.HandleFunc("GET /api/terminal/sessions", defaultWorkspace.Terminal().ListSessions)
	protected.HandleFunc("GET /api/terminal", defaultWorkspace.Terminal().HandleWS)

	protected.HandleFunc("GET /api/lsp/available", defaultWorkspace.LSP().HandleAvailable)
	protected.HandleFunc("GET /api/lsp", defaultWorkspace.LSP().HandleWS)

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
	if rootFolder.HasWorkspace() {
		log.Printf("workspace: %s", rootFolder.Current())
	} else {
		log.Printf("no default workspace - open a folder from the UI")
	}
	if cfg.FrameAncestors != "" {
		log.Printf("embed mode: frame-ancestors %s", cfg.FrameAncestors)
	}
	if len(os.Args) == 1 {
		log.Printf("tip: run with a path to open directly: ./wede /path/to/project")
	}

	// Stop the frp tunnel on shutdown so we don't leave a public tunnel open to
	// a dead local port.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		tunnelMgr.Close()
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, securityHeaders(cfg, mux)); err != nil {
		log.Fatal(err)
	}
}
