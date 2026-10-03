package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"mediaforge/internal/indexer"
)

func (s *Server) handleProfilePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, indexer.ScoringPresets())
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, qualities, tags, language, reject_patterns, upgrade_allowed, COALESCE(profile_type, 'video'), scoring_config
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
		ScoringConfig  json.RawMessage `json:"scoring_config"`
	}

	var profiles []profileEntry
	for rows.Next() {
		var p profileEntry
		var qualities, tags, reject, scoring string
		if err := rows.Scan(&p.ID, &p.Name, &qualities, &tags, &p.Language, &reject, &p.UpgradeAllowed, &p.ProfileType, &scoring); err != nil {
			writeError(w, 500, "failed to read profile")
			return
		}
		resolved, err := normalizedScoringConfig(json.RawMessage(scoring))
		if err != nil {
			writeError(w, 500, "invalid saved scoring configuration")
			return
		}
		p.ScoringConfig = resolved
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
	if err := rows.Err(); err != nil {
		writeError(w, 500, "failed to read profiles")
		return
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

	result, err := s.db.Exec(`INSERT INTO quality_profiles (name, qualities, tags, language, reject_patterns, upgrade_allowed, profile_type, scoring_config)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, qualities, tags, req.Language, reject, req.UpgradeAllowed, req.ProfileType, string(req.ScoringConfig))
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
	// Older clients do not know about scoring_config. An unrelated edit must
	// not silently turn a TRaSH profile back into a legacy profile.
	var existingScoring string
	if err := s.db.QueryRow(`SELECT scoring_config FROM quality_profiles WHERE id=?`, id).Scan(&existingScoring); err != nil {
		writeError(w, 404, "profile not found")
		return
	}
	if len(req.ScoringConfig) == 0 {
		req.ScoringConfig = json.RawMessage(existingScoring)
	}

	if err := req.validate(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	profileType := req.ProfileType

	result, err := s.db.Exec(`UPDATE quality_profiles SET name = ?, qualities = ?, tags = ?, language = ?,
		reject_patterns = ?, upgrade_allowed = ?, profile_type = ?, scoring_config = ? WHERE id = ?`,
		req.Name, string(req.Qualities), string(req.Tags), req.Language,
		string(req.RejectPatterns), req.UpgradeAllowed, profileType, string(req.ScoringConfig), id)
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
	ScoringConfig  json.RawMessage `json:"scoring_config"`
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
	var err error
	p.ScoringConfig, err = normalizedScoringConfig(p.ScoringConfig)
	if err != nil {
		return err
	}
	if string(p.ScoringConfig) != "{}" {
		if p.ProfileType != "video" {
			return fmt.Errorf("anime scoring requires a video profile")
		}
		// The grouped configuration is authoritative for advanced profiles;
		// keep the legacy flat summary in sync for older clients.
		var config struct {
			QualityGroups [][]string `json:"quality_groups"`
		}
		if err := json.Unmarshal(p.ScoringConfig, &config); err != nil {
			return err
		}
		var flat []string
		for _, group := range config.QualityGroups {
			flat = append(flat, group...)
		}
		p.Qualities, _ = json.Marshal(flat)
	}
	if p.Language == "" {
		p.Language = "en"
		if p.ProfileType == "music" || string(p.ScoringConfig) != "{}" {
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

func normalizedScoringConfig(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("scoring_config must be an object")
	}
	if len(fields) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if err := indexer.ValidateScoringConfig(string(raw)); err != nil {
		return nil, err
	}
	resolved, err := indexer.ResolveScoringConfig(string(raw))
	if err != nil {
		return nil, err
	}
	return json.Marshal(resolved)
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

func (s *Server) resolveMediaProfile(id int, mediaType string, anime bool) (int, error) {
	if id == 0 && anime {
		preset := "sonarr-anime"
		if mediaType == "movie" {
			preset = "radarr-anime"
		}
		// Only new additions without an explicit choice use the new defaults.
		// Existing assignments and explicitly selected legacy profiles are kept.
		_ = s.db.QueryRow(`SELECT id FROM quality_profiles WHERE profile_type='video'
			AND json_extract(CASE WHEN json_valid(scoring_config) THEN scoring_config ELSE '{}' END,'$.preset')=? ORDER BY id LIMIT 1`, preset).Scan(&id)
	}
	return s.resolveProfile(id, "video")
}
