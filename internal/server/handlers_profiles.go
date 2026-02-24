package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, qualities, tags, language, reject_patterns, upgrade_allowed
		FROM quality_profiles ORDER BY id`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type profileEntry struct {
		ID             int             `json:"id"`
		Name           string          `json:"name"`
		Qualities      json.RawMessage `json:"qualities"`
		Tags           json.RawMessage `json:"tags"`
		Language       string          `json:"language"`
		RejectPatterns json.RawMessage `json:"reject_patterns"`
		UpgradeAllowed bool            `json:"upgrade_allowed"`
	}

	var profiles []profileEntry
	for rows.Next() {
		var p profileEntry
		var qualities, tags, reject string
		if err := rows.Scan(&p.ID, &p.Name, &qualities, &tags, &p.Language, &reject, &p.UpgradeAllowed); err != nil {
			continue
		}
		p.Qualities = json.RawMessage(qualities)
		if tags != "" {
			p.Tags = json.RawMessage(tags)
		} else {
			p.Tags = json.RawMessage("{}")
		}
		if reject != "" {
			p.RejectPatterns = json.RawMessage(reject)
		} else {
			p.RejectPatterns = json.RawMessage("[]")
		}
		profiles = append(profiles, p)
	}

	if profiles == nil {
		profiles = []profileEntry{}
	}

	writeJSON(w, 200, profiles)
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string          `json:"name"`
		Qualities      json.RawMessage `json:"qualities"`
		Tags           json.RawMessage `json:"tags"`
		Language       string          `json:"language"`
		RejectPatterns json.RawMessage `json:"reject_patterns"`
		UpgradeAllowed bool            `json:"upgrade_allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Name == "" {
		writeError(w, 400, "name required")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}

	qualities := string(req.Qualities)
	tags := string(req.Tags)
	reject := string(req.RejectPatterns)
	if tags == "" { tags = "{}" }
	if reject == "" { reject = "[]" }

	result, err := s.db.Exec(`INSERT INTO quality_profiles (name, qualities, tags, language, reject_patterns, upgrade_allowed)
		VALUES (?, ?, ?, ?, ?, ?)`,
		req.Name, qualities, tags, req.Language, reject, req.UpgradeAllowed)
	if err != nil {
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var req struct {
		Name           string          `json:"name"`
		Qualities      json.RawMessage `json:"qualities"`
		Tags           json.RawMessage `json:"tags"`
		Language       string          `json:"language"`
		RejectPatterns json.RawMessage `json:"reject_patterns"`
		UpgradeAllowed bool            `json:"upgrade_allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	_, err = s.db.Exec(`UPDATE quality_profiles SET name = ?, qualities = ?, tags = ?, language = ?,
		reject_patterns = ?, upgrade_allowed = ? WHERE id = ?`,
		req.Name, string(req.Qualities), string(req.Tags), req.Language,
		string(req.RejectPatterns), req.UpgradeAllowed, id)
	if err != nil {
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	s.db.Exec(`DELETE FROM quality_profiles WHERE id = ?`, id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
