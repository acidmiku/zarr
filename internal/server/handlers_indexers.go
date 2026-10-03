package server

import (
	"database/sql"
	"encoding/json"
	"mediaforge/internal/indexer"
	"net/http"
	"strings"
)

type indexerEntry struct {
	ID                 int      `json:"id"`
	Name               string   `json:"name"`
	URL                string   `json:"url"`
	APIKey             string   `json:"api_key"`
	APIKeyConfigured   bool     `json:"api_key_configured"`
	Priority           int      `json:"priority"`
	Enabled            bool     `json:"enabled"`
	Type               string   `json:"type"`
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	PasswordConfigured bool     `json:"password_configured"`
	ContentTypes       []string `json:"content_types"`
}

const indexerColumns = `id,name,url,api_key,priority,enabled,COALESCE(type,'newznab'),COALESCE(username,''),COALESCE(password,''),COALESCE(content_types,'["movie","series","anime","music"]')`

func readIndexer(row interface{ Scan(...interface{}) error }) (indexerEntry, error) {
	var item indexerEntry
	var types string
	err := row.Scan(&item.ID, &item.Name, &item.URL, &item.APIKey, &item.Priority, &item.Enabled, &item.Type, &item.Username, &item.Password, &types)
	if err == nil {
		err = json.Unmarshal([]byte(types), &item.ContentTypes)
	}
	return item, err
}
func (s *Server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT ` + indexerColumns + ` FROM indexers ORDER BY priority DESC,id`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()
	items := []indexerEntry{}
	for rows.Next() {
		item, err := readIndexer(rows)
		if err != nil {
			writeError(w, 500, "read indexers failed")
			return
		}
		item.APIKeyConfigured = item.APIKey != ""
		item.PasswordConfigured = item.Password != ""
		item.APIKey = ""
		item.Password = ""
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, 500, "read indexers failed")
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleCreateIndexer(w http.ResponseWriter, r *http.Request) {
	s.saveIndexer(w, r, true)
}
func (s *Server) handleUpdateIndexer(w http.ResponseWriter, r *http.Request) {
	s.saveIndexer(w, r, false)
}
func (s *Server) saveIndexer(w http.ResponseWriter, r *http.Request, create bool) {
	item := indexerEntry{Type: "newznab", Enabled: true, Priority: 50, ContentTypes: []string{"movie", "series", "anime", "music"}}
	if !create {
		id, err := parseID(r, "id")
		if err != nil {
			writeError(w, 400, "invalid id")
			return
		}
		item, err = readIndexer(s.db.QueryRow(`SELECT `+indexerColumns+` FROM indexers WHERE id=?`, id))
		if err == sql.ErrNoRows {
			writeError(w, 404, "indexer not found")
			return
		}
		if err != nil {
			writeError(w, 500, "read indexer failed")
			return
		}
	}
	var patch struct {
		Name          *string   `json:"name"`
		URL           *string   `json:"url"`
		APIKey        *string   `json:"api_key"`
		Priority      *int      `json:"priority"`
		Enabled       *bool     `json:"enabled"`
		Type          *string   `json:"type"`
		Username      *string   `json:"username"`
		Password      *string   `json:"password"`
		ContentTypes  *[]string `json:"content_types"`
		ClearAPIKey   bool      `json:"clear_api_key"`
		ClearPassword bool      `json:"clear_password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&patch); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	if patch.Name != nil {
		item.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.URL != nil {
		item.URL = strings.TrimRight(strings.TrimSpace(*patch.URL), "/")
	}
	if patch.APIKey != nil && *patch.APIKey != "" {
		item.APIKey = *patch.APIKey
	}
	if patch.Password != nil && *patch.Password != "" {
		item.Password = *patch.Password
	}
	if patch.Priority != nil {
		item.Priority = *patch.Priority
	}
	if patch.Enabled != nil {
		item.Enabled = *patch.Enabled
	}
	if patch.Type != nil {
		item.Type = *patch.Type
	}
	if patch.Username != nil {
		item.Username = strings.TrimSpace(*patch.Username)
	}
	if patch.ContentTypes != nil {
		item.ContentTypes = *patch.ContentTypes
	}
	if patch.ClearAPIKey {
		item.APIKey = ""
	}
	if patch.ClearPassword {
		item.Password = ""
	}
	if item.Name == "" {
		writeError(w, 400, "name is required")
		return
	}
	if err := validateHTTPURL(item.URL); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if item.Type != "newznab" && item.Type != "rutracker" {
		writeError(w, 400, "unsupported indexer type")
		return
	}
	if item.Enabled && item.Type == "newznab" && item.APIKey == "" {
		writeError(w, 400, "api_key required for Newznab indexers")
		return
	}
	if item.Enabled && item.Type == "rutracker" && (item.Username == "" || item.Password == "") {
		writeError(w, 400, "username and password required for Rutracker")
		return
	}
	if len(item.ContentTypes) == 0 {
		writeError(w, 400, "choose at least one content type")
		return
	}
	for _, ct := range item.ContentTypes {
		if ct != "movie" && ct != "series" && ct != "anime" && ct != "music" {
			writeError(w, 400, "invalid content type")
			return
		}
	}
	types, _ := json.Marshal(item.ContentTypes)
	if create {
		res, err := s.db.Exec(`INSERT INTO indexers(name,url,api_key,priority,enabled,type,username,password,content_types) VALUES(?,?,?,?,?,?,?,?,?)`, item.Name, item.URL, item.APIKey, item.Priority, item.Enabled, item.Type, item.Username, item.Password, string(types))
		if err != nil {
			writeError(w, 500, "save indexer failed")
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, 201, map[string]int64{"id": id})
	} else {
		_, err := s.db.Exec(`UPDATE indexers SET name=?,url=?,api_key=?,priority=?,enabled=?,type=?,username=?,password=?,content_types=? WHERE id=?`, item.Name, item.URL, item.APIKey, item.Priority, item.Enabled, item.Type, item.Username, item.Password, string(types), item.ID)
		if err != nil {
			writeError(w, 500, "save indexer failed")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "updated"})
	}
}
func (s *Server) handleDeleteIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	res, err := s.db.Exec(`DELETE FROM indexers WHERE id=?`, id)
	if err != nil {
		writeError(w, 500, "delete indexer failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "indexer not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
func (s *Server) handleTestIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	item, err := readIndexer(s.db.QueryRow(`SELECT `+indexerColumns+` FROM indexers WHERE id=?`, id))
	if err != nil {
		writeError(w, 404, "indexer not found")
		return
	}
	idx := indexer.IndexerConfig{ID: item.ID, Name: item.Name, URL: item.URL, APIKey: item.APIKey, Priority: item.Priority, Enabled: item.Enabled, Type: item.Type, Username: item.Username, Password: item.Password}
	if item.Type == "rutracker" {
		if s.rutracker == nil {
			writeError(w, 503, "Rutracker is unavailable")
			return
		}
		err = s.rutracker.TestConnection(item.Username, item.Password)
	} else {
		err = s.newznab.TestConnection(idx)
	}
	testResult(w, err)
}
