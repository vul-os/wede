package workspace

import (
	"encoding/json"
	"net/http"
)

// dto is the JSON shape returned for a workspace.
type dto struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root string `json:"root"`
}

func toDTO(r *Workspace) dto {
	return dto{ID: r.ID, Name: r.Name, Root: r.Root()}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// HandleList serves GET /api/workspaces.
func (m *Manager) HandleList(w http.ResponseWriter, r *http.Request) {
	workspaces := m.List()
	out := make([]dto, 0, len(workspaces))
	for _, rm := range workspaces {
		out = append(out, toDTO(rm))
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
}

// HandleCreate serves POST /api/workspaces with body {name?, path}.
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

// Scoped adapts a per-workspace handler into an http.HandlerFunc registered under a
// path containing the {id} wildcard. It resolves the workspace from the path, returns
// 404 if the workspace does not exist, and otherwise invokes the selected handler.
//
//	mux.HandleFunc("GET /api/workspaces/{id}/files",
//	    roomMgr.Scoped(func(rm *workspace.Workspace) http.HandlerFunc { return rm.Files().List }))
func (m *Manager) Scoped(pick func(*Workspace) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rm, ok := m.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
			return
		}
		pick(rm)(w, r)
	}
}

// HandleGet serves GET /api/workspaces/{id}.
func (m *Manager) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rm, ok := m.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDTO(rm))
}

// HandleClose serves DELETE /api/workspaces/{id}.
func (m *Manager) HandleClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !m.Close(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}
