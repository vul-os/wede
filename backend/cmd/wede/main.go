package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"wede/backend/internal/auth"
	"wede/backend/internal/config"
	"wede/backend/internal/files"
	"wede/backend/internal/git"
	"wede/backend/internal/terminal"
	"wede/backend/internal/workspace"
)

func main() {
	portFlag := flag.String("port", "", "Override port (default: from config or 9090)")
	pFlag := flag.String("p", "", "Override port (shorthand)")
	flag.Parse()

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
	termHandler := terminal.New(ws)

	mux := http.NewServeMux()

	// Public auth routes
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/auth/check", authHandler.Check)

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

	protected.HandleFunc("GET /api/git/status", gitHandler.Status)
	protected.HandleFunc("GET /api/git/log", gitHandler.Log)
	protected.HandleFunc("GET /api/git/diff", gitHandler.Diff)
	protected.HandleFunc("POST /api/git/stage", gitHandler.Stage)
	protected.HandleFunc("POST /api/git/unstage", gitHandler.Unstage)
	protected.HandleFunc("POST /api/git/commit", gitHandler.Commit)
	protected.HandleFunc("GET /api/git/branches", gitHandler.Branches)
	protected.HandleFunc("POST /api/git/checkout", gitHandler.Checkout)

	protected.HandleFunc("GET /api/terminal/sessions", termHandler.ListSessions)
	protected.HandleFunc("GET /api/terminal", termHandler.HandleWS)

	mux.Handle("/api/", authHandler.Middleware(protected))

	// Frontend handler - provided by frontend_embed.go or frontend_dev.go
	frontendHandler := newFrontendHandler()
	mux.HandleFunc("/", frontendHandler)

	addr := ":" + cfg.Port
	log.Printf("wede IDE running on http://localhost%s", addr)
	if ws.HasWorkspace() {
		log.Printf("workspace: %s", ws.Current())
	} else {
		log.Printf("no default workspace - open a folder from the UI")
	}
	if len(os.Args) == 1 {
		log.Printf("tip: run with a path to open directly: ./wede /path/to/project")
	}

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
