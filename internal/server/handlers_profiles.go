package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, qualities, tags, language, reject_patterns, upgrade_allowed, COALESCE(profile_type, 'video')
		FROM quality_profiles ORDER BY id`)
	if err != nil {
		slog.Error("load quality profiles", "error", err)
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
		ProfileType    string          `json:"profile_type"`
	}

	var profiles []profileEntry
	for rows.Next() {
		var p profileEntry
		var qualities, tags, reject string
		if err := rows.Scan(&p.ID, &p.Name, &qualities, &tags, &p.Language, &reject, &p.UpgradeAllowed, &p.ProfileType); err != nil {
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
	var req profileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if err := req.validate(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	qualities, tags, reject := string(req.Qualities), string(req.Tags), string(req.RejectPatterns)

	result, err := s.db.Exec(`INSERT INTO quality_profiles (name, qualities, tags, language, reject_patterns, upgrade_allowed, profile_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.Name, qualities, tags, req.Language, reject, req.UpgradeAllowed, req.ProfileType)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, 409, "a profile with this name already exists")
			return
		}
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

	var req profileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if err := req.validate(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	profileType := req.ProfileType

	result, err := s.db.Exec(`UPDATE quality_profiles SET name = ?, qualities = ?, tags = ?, language = ?,
		reject_patterns = ?, upgrade_allowed = ?, profile_type = ? WHERE id = ?`,
		req.Name, string(req.Qualities), string(req.Tags), req.Language,
		string(req.RejectPatterns), req.UpgradeAllowed, profileType, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, 409, "a profile with this name already exists")
			return
		}
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, 404, "profile not found")
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

	result, err := s.db.Exec(`DELETE FROM quality_profiles WHERE id = ?`, id)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			writeError(w, 409, "profile is in use by library items")
			return
		}
		writeError(w, 500, "failed to delete profile")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, 404, "profile not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

type profileRequest struct {
	Name           string          `json:"name"`
	Qualities      json.RawMessage `json:"qualities"`
	Tags           json.RawMessage `json:"tags"`
	Language       string          `json:"language"`
	RejectPatterns json.RawMessage `json:"reject_patterns"`
	UpgradeAllowed bool            `json:"upgrade_allowed"`
	ProfileType    string          `json:"profile_type"`
}

func (p *profileRequest) validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("name required")
	}
	if p.ProfileType == "" {
		p.ProfileType = "video"
	}
	if p.ProfileType != "video" && p.ProfileType != "music" {
		return fmt.Errorf("profile_type must be video or music")
	}
	if p.Language == "" {
		p.Language = "en"
		if p.ProfileType == "music" {
			p.Language = "any"
		}
	}
	var qualities []string
	if json.Unmarshal(p.Qualities, &qualities) != nil || len(qualities) == 0 {
		return fmt.Errorf("qualities must be a nonempty array of strings")
	}
	for _, quality := range qualities {
		if strings.TrimSpace(quality) == "" {
			return fmt.Errorf("quality cannot be empty")
		}
	}
	if len(p.Tags) == 0 {
		p.Tags = json.RawMessage(`{}`)
	}
	var tags map[string]int
	if json.Unmarshal(p.Tags, &tags) != nil || tags == nil {
		return fmt.Errorf("tags must be an object of integer scores")
	}
	if len(p.RejectPatterns) == 0 {
		p.RejectPatterns = json.RawMessage(`[]`)
	}
	var reject []string
	if json.Unmarshal(p.RejectPatterns, &reject) != nil || reject == nil {
		return fmt.Errorf("reject_patterns must be an array of strings")
	}
	for _, pattern := range reject {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("reject patterns cannot be empty")
		}
	}
	return nil
}

// resolveProfile chooses a seeded profile when the caller omits one and rejects
// nonexistent or mismatched profiles before any library side effects occur.
func (s *Server) resolveProfile(id int, profileType string) (int, error) {
	if id < 0 {
		return 0, fmt.Errorf("invalid quality_profile_id")
	}
	if id == 0 {
		err := s.db.QueryRow(`SELECT id FROM quality_profiles WHERE profile_type = ? ORDER BY id LIMIT 1`, profileType).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("no %s quality profile configured", profileType)
		}
		return id, nil
	}
	var kind string
	if err := s.db.QueryRow(`SELECT COALESCE(profile_type,'video') FROM quality_profiles WHERE id = ?`, id).Scan(&kind); err != nil || kind != profileType {
		return 0, fmt.Errorf("quality_profile_id must reference a %s profile", profileType)
	}
	return id, nil
}
