package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestApp(t *testing.T) *App {
	t.Helper()
	db, err := openDB(":memory:")
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &App{db: db, jwt: "test-secret"}
}

func TestCreateAndListTasks(t *testing.T) {
	app := setupTestApp(t)

	// Create a task
	body, _ := json.Marshal(map[string]string{"title": "Write tests", "priority": "high"})
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.handleCreateTask(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List tasks
	req2 := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w2 := httptest.NewRecorder()
	app.handleListTasks(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var tasks []Task
	if err := json.NewDecoder(w2.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Write tests" {
		t.Errorf("unexpected title: %q", tasks[0].Title)
	}
	if tasks[0].Priority != "high" {
		t.Errorf("unexpected priority: %q", tasks[0].Priority)
	}
}

func TestCreateTaskMissingTitle(t *testing.T) {
	app := setupTestApp(t)
	body, _ := json.Marshal(map[string]string{"priority": "low"})
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.handleCreateTask(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestUpdateTask(t *testing.T) {
	app := setupTestApp(t)

	// Seed a task directly
	res, _ := app.db.Exec(`INSERT INTO tasks (title, priority) VALUES ('Fix bug', 'medium')`)
	id, _ := res.LastInsertId()

	done := true
	body, _ := json.Marshal(UpdateTaskRequest{Done: &done})
	req := httptest.NewRequest(http.MethodPatch, "/tasks/"+fmt.Sprint(id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", fmt.Sprint(id))
	w := httptest.NewRecorder()
	app.handleUpdateTask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
