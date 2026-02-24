package server

import (
	"encoding/json"
	"net/http"

	"mediaforge/internal/indexer"
)

func (s *Server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, url, api_key, priority, enabled FROM indexers ORDER BY priority DESC`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type indexerEntry struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		URL      string `json:"url"`
		APIKey   string `json:"api_key"`
		Priority int    `json:"priority"`
		Enabled  bool   `json:"enabled"`
	}

	var indexers []indexerEntry
	for rows.Next() {
		var idx indexerEntry
		if err := rows.Scan(&idx.ID, &idx.Name, &idx.URL, &idx.APIKey, &idx.Priority, &idx.Enabled); err != nil {
			continue
		}
		indexers = append(indexers, idx)
	}

	if indexers == nil {
		indexers = []indexerEntry{}
	}

	writeJSON(w, 200, indexers)
}

func (s *Server) handleCreateIndexer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		APIKey   string `json:"api_key"`
		Priority int    `json:"priority"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Name == "" || req.URL == "" || req.APIKey == "" {
		writeError(w, 400, "name, url, and api_key required")
		return
	}

	result, err := s.db.Exec(`INSERT INTO indexers (name, url, api_key, priority, enabled)
		VALUES (?, ?, ?, ?, ?)`,
		req.Name, req.URL, req.APIKey, req.Priority, req.Enabled)
	if err != nil {
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (s *Server) handleUpdateIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		APIKey   string `json:"api_key"`
		Priority int    `json:"priority"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	_, err = s.db.Exec(`UPDATE indexers SET name = ?, url = ?, api_key = ?, priority = ?, enabled = ? WHERE id = ?`,
		req.Name, req.URL, req.APIKey, req.Priority, req.Enabled, id)
	if err != nil {
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	s.db.Exec(`DELETE FROM indexers WHERE id = ?`, id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleTestIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var idx indexer.IndexerConfig
	err = s.db.QueryRow(`SELECT id, name, url, api_key, priority, enabled FROM indexers WHERE id = ?`, id).
		Scan(&idx.ID, &idx.Name, &idx.URL, &idx.APIKey, &idx.Priority, &idx.Enabled)
	if err != nil {
		writeError(w, 404, "indexer not found")
		return
	}

	if err := s.newznab.TestConnection(idx); err != nil {
		writeJSON(w, 200, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"success": true,
	})
}
