package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type Task struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	Priority  string    `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateTaskRequest struct {
	Title    string `json:"title"`
	Priority string `json:"priority"`
}

type UpdateTaskRequest struct {
	Title    *string `json:"title,omitempty"`
	Done     *bool   `json:"done,omitempty"`
	Priority *string `json:"priority,omitempty"`
}

func (a *App) handleListTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, title, done, priority, created_at FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Done, &t.Priority, &t.CreatedAt); err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			return
		}
		tasks = append(tasks, t)
	}

	writeJSON(w, http.StatusOK, tasks)
}

func (a *App) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusUnprocessableEntity)
		return
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}

	res, err := a.db.ExecContext(r.Context(),
		`INSERT INTO tasks (title, priority) VALUES (?, ?)`, req.Title, req.Priority)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (a *App) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Done != nil {
		if _, err := a.db.ExecContext(r.Context(),
			`UPDATE tasks SET done = ? WHERE id = ?`, *req.Done, id); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
	}
	if req.Title != nil {
		if _, err := a.db.ExecContext(r.Context(),
			`UPDATE tasks SET title = ? WHERE id = ?`, *req.Title, id); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM tasks WHERE id = ?`, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	// Simplified: accept any non-empty admin/admin in dev
	if creds.Username != "admin" || creds.Password != "admin" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.demo",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
