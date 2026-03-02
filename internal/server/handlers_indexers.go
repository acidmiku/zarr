package server

import (
	"encoding/json"
	"net/http"

	"mediaforge/internal/indexer"
)

func (s *Server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, url, api_key, priority, enabled, COALESCE(type,'newznab'), COALESCE(username,''), COALESCE(password,''), COALESCE(content_types,'["movie","series","anime","music"]') FROM indexers ORDER BY priority DESC`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type indexerEntry struct {
		ID           int      `json:"id"`
		Name         string   `json:"name"`
		URL          string   `json:"url"`
		APIKey       string   `json:"api_key"`
		Priority     int      `json:"priority"`
		Enabled      bool     `json:"enabled"`
		Type         string   `json:"type"`
		Username     string   `json:"username"`
		Password     string   `json:"password"`
		ContentTypes []string `json:"content_types"`
	}

	var indexers []indexerEntry
	for rows.Next() {
		var idx indexerEntry
		var contentTypesJSON string
		if err := rows.Scan(&idx.ID, &idx.Name, &idx.URL, &idx.APIKey, &idx.Priority, &idx.Enabled, &idx.Type, &idx.Username, &idx.Password, &contentTypesJSON); err != nil {
			continue
		}
		json.Unmarshal([]byte(contentTypesJSON), &idx.ContentTypes)
		if idx.ContentTypes == nil {
			idx.ContentTypes = []string{"movie", "series", "anime", "music"}
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
		Name         string   `json:"name"`
		URL          string   `json:"url"`
		APIKey       string   `json:"api_key"`
		Priority     int      `json:"priority"`
		Enabled      bool     `json:"enabled"`
		Type         string   `json:"type"`
		Username     string   `json:"username"`
		Password     string   `json:"password"`
		ContentTypes []string `json:"content_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Name == "" || req.URL == "" {
		writeError(w, 400, "name and url required")
		return
	}

	if req.Type == "" {
		req.Type = "newznab"
	}

	// Newznab requires API key; Rutracker requires username/password
	if req.Type == "newznab" && req.APIKey == "" {
		writeError(w, 400, "api_key required for Newznab indexers")
		return
	}
	if req.Type == "rutracker" && (req.Username == "" || req.Password == "") {
		writeError(w, 400, "username and password required for Rutracker indexers")
		return
	}

	if req.ContentTypes == nil {
		req.ContentTypes = []string{"movie", "series", "anime", "music"}
	}
	contentTypesJSON, _ := json.Marshal(req.ContentTypes)

	result, err := s.db.Exec(`INSERT INTO indexers (name, url, api_key, priority, enabled, type, username, password, content_types)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.URL, req.APIKey, req.Priority, req.Enabled, req.Type, req.Username, req.Password, string(contentTypesJSON))
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
		Name         string   `json:"name"`
		URL          string   `json:"url"`
		APIKey       string   `json:"api_key"`
		Priority     int      `json:"priority"`
		Enabled      bool     `json:"enabled"`
		Type         string   `json:"type"`
		Username     string   `json:"username"`
		Password     string   `json:"password"`
		ContentTypes []string `json:"content_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Type == "" {
		req.Type = "newznab"
	}
	if req.ContentTypes == nil {
		req.ContentTypes = []string{"movie", "series", "anime", "music"}
	}
	contentTypesJSON, _ := json.Marshal(req.ContentTypes)

	_, err = s.db.Exec(`UPDATE indexers SET name = ?, url = ?, api_key = ?, priority = ?, enabled = ?, type = ?, username = ?, password = ?, content_types = ? WHERE id = ?`,
		req.Name, req.URL, req.APIKey, req.Priority, req.Enabled, req.Type, req.Username, req.Password, string(contentTypesJSON), id)
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
	var idxType string
	err = s.db.QueryRow(`SELECT id, name, url, api_key, priority, enabled, COALESCE(type,'newznab'), COALESCE(username,''), COALESCE(password,'') FROM indexers WHERE id = ?`, id).
		Scan(&idx.ID, &idx.Name, &idx.URL, &idx.APIKey, &idx.Priority, &idx.Enabled, &idxType, &idx.Username, &idx.Password)
	if err != nil {
		writeError(w, 404, "indexer not found")
		return
	}
	idx.Type = idxType

	if idx.Type == "rutracker" {
		// Test Rutracker connection
		if s.rutracker == nil {
			writeJSON(w, 200, map[string]interface{}{"success": false, "error": "Rutracker client not initialized"})
			return
		}
		if err := s.rutracker.TestConnection(idx.Username, idx.Password); err != nil {
			writeJSON(w, 200, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]interface{}{"success": true})
		return
	}

	// Default: test Newznab
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
