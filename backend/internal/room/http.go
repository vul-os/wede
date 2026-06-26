package room

import (
	"encoding/json"
	"net/http"
)

// dto is the JSON shape returned for a room.
type dto struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root string `json:"root"`
}

func toDTO(r *Room) dto {
	return dto{ID: r.ID, Name: r.Name, Root: r.Root()}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// HandleList serves GET /api/rooms.
func (m *Manager) HandleList(w http.ResponseWriter, r *http.Request) {
	rooms := m.List()
	out := make([]dto, 0, len(rooms))
	for _, rm := range rooms {
		out = append(out, toDTO(rm))
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

// HandleCreate serves POST /api/rooms with body {name?, path}.
func (m *Manager) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}
	rm, err := m.Create(body.Name, body.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, toDTO(rm))
}

// HandleGet serves GET /api/rooms/{id}.
func (m *Manager) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rm, ok := m.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "room not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDTO(rm))
}

// HandleClose serves DELETE /api/rooms/{id}.
func (m *Manager) HandleClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !m.Close(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "room not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}
