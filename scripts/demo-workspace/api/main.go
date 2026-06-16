package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const defaultAddr = ":8080"

type App struct {
	db  *sql.DB
	jwt string
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me-in-production"
		log.Println("warning: using default JWT secret — set JWT_SECRET in production")
	}

	db, err := openDB("taskboard.db")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	app := &App{db: db, jwt: secret}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", app.handleLogin)
	mux.HandleFunc("GET /tasks", app.authMiddleware(app.handleListTasks))
	mux.HandleFunc("POST /tasks", app.authMiddleware(app.handleCreateTask))
	mux.HandleFunc("PATCH /tasks/{id}", app.authMiddleware(app.handleUpdateTask))
	mux.HandleFunc("DELETE /tasks/{id}", app.authMiddleware(app.handleDeleteTask))

	srv := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("taskboard API listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			title     TEXT    NOT NULL,
			done      BOOLEAN NOT NULL DEFAULT 0,
			priority  TEXT    NOT NULL DEFAULT 'medium',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS users (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT    NOT NULL UNIQUE,
			password TEXT    NOT NULL
		);
	`)
	return db, err
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
