package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"mediaforge/internal/httpclient"
	"mediaforge/internal/metadata"
	"mediaforge/internal/qbt"
	"mediaforge/internal/sabnzbd"
)

var secretSettings = map[string]string{
	"tmdb_api_key": "tmdb_configured", "sabnzbd_api_key": "sabnzbd_configured",
	"lastfm_api_key": "lastfm_configured", "qbittorrent_password": "qbittorrent_configured",
	"openrouter_api_key": "openrouter_configured", "brave_api_key": "brave_configured",
}
var settingsDefaults = map[string]string{
	"proxy": "", "media_root": "/data/media", "tmdb_api_key": "", "sabnzbd_url": "http://sabnzbd:8080", "sabnzbd_api_key": "",
	"setup_complete": "false", "lastfm_api_key": "", "qbittorrent_enabled": "false", "qbittorrent_url": "http://qbittorrent:9090",
	"qbittorrent_username": "admin", "qbittorrent_password": "", "torrent_seed_time_hours": "24", "torrent_remove_after_seed": "true",
	"openrouter_api_key": "", "openrouter_model": "moonshotai/kimi-k3", "openrouter_reasoning_effort": "high", "brave_api_key": "",
	"ai_personality_preset": "default", "ai_personality_custom": "",
}
var boolSettings = map[string]bool{"setup_complete": true, "qbittorrent_enabled": true, "torrent_remove_after_seed": true}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{}
	for key, fallback := range settingsDefaults {
		value, err := s.db.GetSetting(key)
		if err != nil {
			writeError(w, 500, "read settings failed")
			return
		}
		if value == "" {
			value = fallback
		}
		if flag, secret := secretSettings[key]; secret {
			response[key] = ""
			response[flag] = value != ""
		} else if boolSettings[key] {
			response[key] = value == "true"
		} else {
			response[key] = value
		}
	}
	writeJSON(w, 200, response)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var request map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&request); err != nil || request == nil {
		writeError(w, 400, "invalid JSON object")
		return
	}
	changes := map[string]string{}
	for key, raw := range request {
		if strings.HasPrefix(key, "clear_") {
			target := strings.TrimPrefix(key, "clear_")
			if _, ok := secretSettings[target]; !ok {
				writeError(w, 400, "unknown setting: "+key)
				return
			}
			var clear bool
			if string(raw) == "null" || json.Unmarshal(raw, &clear) != nil {
				writeError(w, 400, key+" must be boolean")
				return
			}
			if clear {
				changes[target] = ""
			}
			continue
		}
		if _, ok := settingsDefaults[key]; !ok {
			writeError(w, 400, "unknown setting: "+key)
			return
		}
		var value string
		if boolSettings[key] {
			var b bool
			if string(raw) == "null" || json.Unmarshal(raw, &b) != nil {
				writeError(w, 400, key+" must be boolean")
				return
			}
			value = strconv.FormatBool(b)
		} else {
			if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
				writeError(w, 400, key+" must be a string")
				return
			}
		}
		if _, secret := secretSettings[key]; secret {
			if value == "" || value == "********" || strings.Contains(value, "...") {
				continue
			}
		} else {
			value = strings.TrimSpace(value)
		}
		if key == "sabnzbd_url" || key == "qbittorrent_url" {
			if err := validateHTTPURL(value); err != nil {
				writeError(w, 400, key+": "+err.Error())
				return
			}
			value = strings.TrimRight(value, "/")
		}
		if key == "media_root" && (value == "" || !filepath.IsAbs(value)) {
			writeError(w, 400, "media_root must be an absolute path")
			return
		}
		if key == "torrent_seed_time_hours" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 87600 {
				writeError(w, 400, "seed time must be between 0 and 87600 hours")
				return
			}
		}
		if key == "openrouter_reasoning_effort" && value != "high" && value != "medium" && value != "low" && value != "none" {
			writeError(w, 400, "reasoning effort must be high, medium, low, or none")
			return
		}
		if key == "openrouter_model" && (value == "" || !strings.Contains(value, "/")) {
			writeError(w, 400, "model must be a provider/model ID")
			return
		}
		if key == "proxy" {
			if _, err := httpclient.New(value); err != nil {
				writeError(w, 400, "invalid proxy URL")
				return
			}
		}
		changes[key] = value
	}
	// Explicit clears win regardless of map iteration order.
	for key, raw := range request {
		if strings.HasPrefix(key, "clear_") && string(raw) == "true" {
			changes[strings.TrimPrefix(key, "clear_")] = ""
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, 500, "save settings failed")
		return
	}
	defer tx.Rollback()
	for key, value := range changes {
		if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, key, value); err != nil {
			writeError(w, 500, "save settings failed")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "save settings failed")
		return
	}
	s.cfg.Reload(s.db)
	if _, ok := changes["proxy"]; ok {
		if err := httpclient.UpdateProxy(s.proxyClient, s.cfg.Proxy); err != nil {
			writeError(w, 500, "apply proxy failed")
			return
		}
	}
	if s.grabber != nil {
		s.grabber.UpdateConfig(s.cfg.SABnzbdURL, s.cfg.SABnzbdKey)
	}
	if s.processor != nil {
		s.processor.UpdateMediaRoot(s.cfg.MediaRoot)
	}
	if s.scanner != nil {
		s.scanner.UpdateMediaRoot(s.cfg.MediaRoot)
	}
	if s.qbt != nil {
		s.qbt.UpdateConfig(s.cfg.QBTURL, s.cfg.QBTUsername, s.cfg.QBTPassword)
	}
	if _, ok := changes["tmdb_api_key"]; ok {
		s.tmdb = nil
		if s.cfg.TMDBApiKey != "" {
			s.tmdb = metadata.NewTMDBClient(s.proxyClient, s.cfg.TMDBApiKey)
		}
		if s.metadataUpdater != nil {
			s.metadataUpdater(s.tmdb)
		}
	}
	if _, ok := changes["lastfm_api_key"]; ok {
		s.lastfm = nil
		if s.cfg.LastFMApiKey != "" {
			s.lastfm = metadata.NewLastFMClient(s.proxyClient, s.cfg.LastFMApiKey, s.db, s.cfg.ConfigDir)
		}
	}
	if _, ok := changes["openrouter_api_key"]; ok {
		s.initAI()
	} else if _, ok := changes["brave_api_key"]; ok {
		s.initAI()
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func validateHTTPURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("use an http(s) URL without credentials, query, or fragment")
	}
	return nil
}

type connectionTest struct {
	APIKey          string `json:"api_key"`
	Key             string `json:"key"`
	URL             string `json:"url"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
}

func decodeConnection(w http.ResponseWriter, r *http.Request) (connectionTest, bool) {
	var req connectionTest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil && err != io.EOF {
		writeError(w, 400, "invalid JSON")
		return req, false
	}
	if req.APIKey == "" {
		req.APIKey = req.Key
	}
	return req, true
}
func testResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"success": false, "error": err.Error()})
	} else {
		writeJSON(w, 200, map[string]bool{"success": true})
	}
}
func savedOr(value, fallback string) string {
	if value == "" || value == "********" || strings.Contains(value, "...") {
		return fallback
	}
	return value
}

func (s *Server) handleTestQBittorrent(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConnection(w, r)
	if !ok {
		return
	}
	endpoint := savedOr(req.URL, s.cfg.QBTURL)
	if err := validateHTTPURL(endpoint); err != nil {
		testResult(w, err)
		return
	}
	client := qbt.New(s.directClient, endpoint, savedOr(req.Username, s.cfg.QBTUsername), savedOr(req.Password, s.cfg.QBTPassword))
	testResult(w, client.TestConnection())
}
func (s *Server) handleTestSABnzbd(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConnection(w, r)
	if !ok {
		return
	}
	endpoint := savedOr(req.URL, s.cfg.SABnzbdURL)
	if err := validateHTTPURL(endpoint); err != nil {
		testResult(w, err)
		return
	}
	client := sabnzbd.New(s.directClient, endpoint, savedOr(req.APIKey, s.cfg.SABnzbdKey))
	testResult(w, client.Test(r.Context()))
}
func (s *Server) handleTestTMDB(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConnection(w, r)
	if !ok {
		return
	}
	key := savedOr(req.APIKey, s.cfg.TMDBApiKey)
	if key == "" {
		testResult(w, fmt.Errorf("enter a TMDB API key or access token"))
		return
	}
	request, _ := http.NewRequestWithContext(r.Context(), "GET", "https://api.themoviedb.org/3/configuration", nil)
	if len(key) == 32 {
		q := request.URL.Query()
		q.Set("api_key", key)
		request.URL.RawQuery = q.Encode()
	} else {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := s.proxyClient.Do(request)
	if err != nil {
		testResult(w, fmt.Errorf("TMDB connection failed; check network or proxy"))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		testResult(w, fmt.Errorf("TMDB returned HTTP %d", resp.StatusCode))
		return
	}
	testResult(w, nil)
}
func (s *Server) handleTestOpenRouter(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConnection(w, r)
	if !ok {
		return
	}
	stored, _ := s.db.GetSetting("openrouter_api_key")
	key := savedOr(req.APIKey, stored)
	if key == "" {
		testResult(w, fmt.Errorf("enter an OpenRouter API key"))
		return
	}
	// The authenticated key endpoint validates credentials without billing a chat.
	request, _ := http.NewRequestWithContext(r.Context(), "GET", "https://openrouter.ai/api/v1/key", nil)
	request.Header.Set("Authorization", "Bearer "+key)
	resp, err := s.proxyClient.Do(request)
	if err != nil {
		testResult(w, fmt.Errorf("OpenRouter connection failed; check network or proxy"))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		testResult(w, fmt.Errorf("OpenRouter returned HTTP %d", resp.StatusCode))
		return
	}
	testResult(w, nil)
}
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	key, _ := s.db.GetSetting("openrouter_api_key")
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM indexers WHERE enabled=1`).Scan(&count)
	writeJSON(w, 200, map[string]interface{}{"setup_complete": s.db.IsSetupComplete(), "tmdb_configured": s.cfg.TMDBApiKey != "", "openrouter_configured": key != "", "sabnzbd_configured": s.cfg.SABnzbdKey != "", "qbittorrent_configured": s.cfg.QBTPassword != "", "indexer_count": count, "media_root": s.cfg.MediaRoot})
}
func maskKey(key string) string {
	if key == "" {
		return ""
	}
	return "********"
}
