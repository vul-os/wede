package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"wede/backend/internal/auth"
	"wede/backend/internal/config"
	"wede/backend/internal/folder"
	"wede/backend/internal/lsp"
	"wede/backend/internal/tasks"
	"wede/backend/internal/trust"
	"wede/backend/internal/tunnel"
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

	// Optional user-defined language servers (~/.wede/lsp.json) — add LSP support
	// for any language without recompiling. Missing file is fine.
	if home, err := os.UserHomeDir(); err == nil {
		if err := lsp.LoadConfig(filepath.Join(home, ".wede", "lsp.json")); err != nil {
			log.Printf("lsp config: %v", err)
		}
	}

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
	// Confine runtime folder-open / workspace-create requests to the configured
	// allowed base (default: $HOME). The boot path above is the owner's explicit
	// launch choice and is intentionally exempt from this restriction.
	rootFolder.SetAllowedRoot(cfg.WorkspaceRoot)

	// Workspace registry: the multi-project backbone. The boot workspace is adopted
	// as the default workspace so the solo-user case works with zero setup; additional
	// projects can be opened as further workspaces via /api/workspaces.
	wsMgr := workspace.NewManager(cfg.FrameAncestors, cfg.WorkspaceRoot)
	// The default workspace must exist so scoped /api/workspaces/{id}/... routes
	// (and the frontend's auto-selected "default") resolve.
	wsMgr.Register("default", rootFolder)

	authHandler := auth.New(cfg.Password)
	tunnelMgr := tunnel.New(cfg.Port) // optional frp public tunnel (owner-only)

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
	// Opening a folder adopts a new workspace root and grants a shell/edit surface
	// over it, so it is gated to editor+ (viewers are rejected) and the target path
	// is restricted to the allowed base inside the handler.
	protected.Handle("POST /api/folder/open", re(http.HandlerFunc(rootFolder.HandleOpen)))
	protected.HandleFunc("GET /api/folder/browse", rootFolder.HandleBrowse)

	// Workspace registry endpoints (multi-project backbone). Per-workspace scoping of the
	// file/git/etc. routes under /api/workspaces/{id}/... is layered on in later slices.
	protected.HandleFunc("GET /api/workspaces", wsMgr.HandleList)

	// Creating a workspace is editor+ only (same rationale as /api/folder/open);
	// the requested root is validated against the allowed base in HandleCreate.
	protected.Handle("POST /api/workspaces", re(http.HandlerFunc(wsMgr.HandleCreate)))
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
	protected.Handle("PUT /api/workspaces/{id}/files/write", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Write })))
	protected.Handle("POST /api/workspaces/{id}/files/create", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Create })))
	protected.Handle("DELETE /api/workspaces/{id}/files/delete", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Delete })))
	protected.Handle("POST /api/workspaces/{id}/files/rename", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Rename })))
	protected.Handle("POST /api/workspaces/{id}/files/copy", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Copy })))
	protected.Handle("POST /api/workspaces/{id}/files/format", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Files().Format })))
	// git
	protected.HandleFunc("GET /api/workspaces/{id}/git/status", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Status }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/log", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Log }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/diff", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Diff }))
	protected.Handle("POST /api/workspaces/{id}/git/stage", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Stage })))
	protected.Handle("POST /api/workspaces/{id}/git/unstage", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Unstage })))
	protected.Handle("POST /api/workspaces/{id}/git/commit", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Commit })))
	protected.HandleFunc("GET /api/workspaces/{id}/git/branches", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Branches }))
	protected.Handle("POST /api/workspaces/{id}/git/checkout", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Checkout })))
	protected.Handle("POST /api/workspaces/{id}/git/branch", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().CreateBranch })))
	protected.Handle("POST /api/workspaces/{id}/git/branch/delete", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().DeleteBranch })))
	protected.Handle("POST /api/workspaces/{id}/git/fetch", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Fetch })))
	protected.Handle("POST /api/workspaces/{id}/git/pull", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Pull })))
	protected.Handle("POST /api/workspaces/{id}/git/push", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Push })))
	protected.HandleFunc("GET /api/workspaces/{id}/git/remotes", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Remotes }))
	protected.Handle("POST /api/workspaces/{id}/git/discard", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().Discard })))
	protected.HandleFunc("GET /api/workspaces/{id}/git/stash", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashList }))
	protected.Handle("POST /api/workspaces/{id}/git/stash", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashPush })))
	protected.Handle("POST /api/workspaces/{id}/git/stash/pop", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashPop })))
	protected.Handle("POST /api/workspaces/{id}/git/stash/drop", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StashDrop })))
	protected.HandleFunc("GET /api/workspaces/{id}/git/commit-diff", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().CommitDiff }))
	protected.HandleFunc("GET /api/workspaces/{id}/git/conflict", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().ConflictRegions }))
	protected.Handle("POST /api/workspaces/{id}/git/conflict/resolve", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().ConflictResolve })))
	protected.Handle("POST /api/workspaces/{id}/git/remotes/add", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().RemoteAdd })))
	protected.Handle("POST /api/workspaces/{id}/git/remotes/remove", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().RemoteRemove })))
	protected.Handle("POST /api/workspaces/{id}/git/stage-hunk", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Git().StageHunk })))
	// search
	protected.HandleFunc("GET /api/workspaces/{id}/search", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().Search }))
	protected.HandleFunc("GET /api/workspaces/{id}/search/replace-preview", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().ReplacePreview }))
	protected.Handle("POST /api/workspaces/{id}/search/replace", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Search().ReplaceApply })))
	// file-watch SSE (one fsnotify watcher per workspace)
	protected.HandleFunc("GET /api/workspaces/{id}/watch", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Watcher().HandleSSE }))
	// terminal (shared PTY sessions per workspace) + lsp (language servers per workspace)
	protected.HandleFunc("GET /api/workspaces/{id}/terminal/sessions", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Terminal().ListSessions }))
	protected.Handle("GET /api/workspaces/{id}/terminal", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Terminal().HandleWS }))) // viewers get no shell
	// Tasks (~/.wede/tasks.json + trusted project .wede/tasks.json) — run client-side
	// in a terminal, which is itself editor-gated.
	protected.HandleFunc("GET /api/workspaces/{id}/tasks", rs(func(ws *workspace.Workspace) http.HandlerFunc { return tasks.Handler(ws.Root()) }))
	// Workspace trust (owner-only) — gates running commands from a workspace's
	// committed .wede/ tooling config.
	trustFn := func(ws *workspace.Workspace) http.HandlerFunc { return trust.Handler(ws.Root()) }
	protected.Handle("GET /api/workspaces/{id}/trust", ro(rs(trustFn)))
	protected.Handle("POST /api/workspaces/{id}/trust", ro(rs(trustFn)))
	protected.Handle("DELETE /api/workspaces/{id}/trust", ro(rs(trustFn)))
	protected.HandleFunc("GET /api/workspaces/{id}/lsp/available", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.LSP().HandleAvailable }))
	// Debug Adapter Protocol — listing is read-only; the debug session WS runs code,
	// so it is editor-gated.
	protected.HandleFunc("GET /api/workspaces/{id}/dap/available", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.DAP().HandleAvailable }))
	protected.Handle("GET /api/workspaces/{id}/dap", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.DAP().HandleWS })))
	protected.HandleFunc("GET /api/workspaces/{id}/lsp", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.LSP().HandleWS }))
	// collaboration socket (presence: roster + cursors)
	protected.HandleFunc("GET /api/workspaces/{id}/collab", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.Collab().HandleWS }))
	// CRDT document sync+awareness (ygo provider). {room...} is the file's
	// workspace-relative path; the provider reads it via r.PathValue("workspace").
	// Editor+ only: the doc socket drives DiskPersistence write-back to files, so
	// viewers must not be able to connect and overwrite workspace files.
	protected.Handle("GET /api/workspaces/{id}/doc/{room...}", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.DocServer().ServeHTTP })))
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
	// .wede location — which workspace folder hosts .wede (owner-only; PUT moves it)
	protected.Handle("GET /api/workspaces/{id}/wede-location", ro(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.HandleWedeLocationGet })))
	protected.Handle("PUT /api/workspaces/{id}/wede-location", ro(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.HandleWedeLocationSet })))
	// built-in API client (Postman-style); requests saved under <wede>/requests/
	protected.HandleFunc("GET /api/workspaces/{id}/apiclient", rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.APIClient().Tree }))
	protected.Handle("POST /api/workspaces/{id}/apiclient/send", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.APIClient().Send })))
	protected.Handle("PUT /api/workspaces/{id}/apiclient/item", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.APIClient().SaveItem })))
	protected.Handle("DELETE /api/workspaces/{id}/apiclient/item", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.APIClient().DeleteItem })))
	protected.Handle("PUT /api/workspaces/{id}/apiclient/environment", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.APIClient().SaveEnvironment })))
	protected.Handle("DELETE /api/workspaces/{id}/apiclient/environment", re(rs(func(ws *workspace.Workspace) http.HandlerFunc { return ws.APIClient().DeleteEnvironment })))

	// Legacy default-workspace routes (/api/files, /api/git, /api/search,
	// /api/watch, /api/terminal, /api/lsp) have been removed: the frontend's
	// authFetch now rewrites them to /api/workspaces/{activeId}/... so every
	// request follows the focused workspace.

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
