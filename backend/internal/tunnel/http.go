package tunnel

import (
	"encoding/json"
	"net/http"
)

// All tunnel routes are owner-only (mounted behind RequireOwner in main.go):
//   GET  /api/tunnel          -> HandleGet     (status + config, token redacted)
//   PUT  /api/tunnel/config   -> HandleSetConfig
//   POST /api/tunnel/start    -> HandleStart
//   POST /api/tunnel/stop     -> HandleStop

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// HandleGet returns the current tunnel state (frpc detected, status, public URL,
// config with the token redacted, recent log).
func (m *Manager) HandleGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, m.Snapshot())
}

// HandleSetConfig persists the relay config. An empty token preserves the stored
// one (so the redacted-on-read token isn't wiped on save).
func (m *Manager) HandleSetConfig(w http.ResponseWriter, r *http.Request) {
	var c Config
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if c.Token == "" {
		m.mu.Lock()
		c.Token = m.cfg.Token // keep existing token when client sends none
		m.mu.Unlock()
	}
	if err := m.SetConfig(c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, m.Snapshot())
}

// HandleStart launches the tunnel.
func (m *Manager) HandleStart(w http.ResponseWriter, r *http.Request) {
	if err := m.Start(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, m.Snapshot())
}

// HandleStop terminates the tunnel.
func (m *Manager) HandleStop(w http.ResponseWriter, r *http.Request) {
	_ = m.Stop()
	writeJSON(w, http.StatusOK, m.Snapshot())
}
