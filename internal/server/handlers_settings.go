package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"mediaforge/internal/ai"
	"mediaforge/internal/metadata"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings := map[string]string{
		"proxy":          "",
		"media_root":     s.cfg.MediaRoot,
		"tmdb_api_key":   s.cfg.TMDBApiKey,
		"sabnzbd_url":    s.cfg.SABnzbdURL,
		"sabnzbd_api_key": s.cfg.SABnzbdKey,
		"setup_complete": "false",
	}

	for key := range settings {
		val, err := s.db.GetSetting(key)
		if err == nil && val != "" {
			settings[key] = val
		}
	}

	// Music settings
	lastfmKey, _ := s.db.GetSetting("lastfm_api_key")

	// AI settings
	orKey, _ := s.db.GetSetting("openrouter_api_key")
	orModel, _ := s.db.GetSetting("openrouter_model")
	braveKey, _ := s.db.GetSetting("brave_api_key")
	aiPreset, _ := s.db.GetSetting("ai_personality_preset")
	aiCustom, _ := s.db.GetSetting("ai_personality_custom")

	if orModel == "" {
		orModel = "anthropic/claude-sonnet-4-20250514"
	}
	if aiPreset == "" {
		aiPreset = "default"
	}

	// qBittorrent settings
	qbtEnabled, _ := s.db.GetSetting("qbittorrent_enabled")
	qbtURL, _ := s.db.GetSetting("qbittorrent_url")
	if qbtURL == "" {
		qbtURL = "http://qbittorrent:8080"
	}
	qbtUsername, _ := s.db.GetSetting("qbittorrent_username")
	if qbtUsername == "" {
		qbtUsername = "admin"
	}
	qbtPassword, _ := s.db.GetSetting("qbittorrent_password")
	seedTimeStr, _ := s.db.GetSetting("torrent_seed_time_hours")
	if seedTimeStr == "" {
		seedTimeStr = "24"
	}
	removeAfterStr, _ := s.db.GetSetting("torrent_remove_after_seed")
	removeAfterSeed := removeAfterStr != "false"

	// Mask API keys for display
	response := map[string]interface{}{
		"proxy":          settings["proxy"],
		"media_root":     settings["media_root"],
		"tmdb_api_key":   maskKey(settings["tmdb_api_key"]),
		"tmdb_configured": settings["tmdb_api_key"] != "",
		"sabnzbd_url":    settings["sabnzbd_url"],
		"sabnzbd_api_key": maskKey(settings["sabnzbd_api_key"]),
		"sabnzbd_configured": settings["sabnzbd_api_key"] != "",
		"setup_complete": settings["setup_complete"] == "true",

		// Music settings
		"lastfm_api_key":    maskKey(lastfmKey),
		"lastfm_configured": lastfmKey != "",

		// qBittorrent settings
		"qbittorrent_enabled":    qbtEnabled == "true",
		"qbittorrent_url":        qbtURL,
		"qbittorrent_username":   qbtUsername,
		"qbittorrent_password":   maskKey(qbtPassword),
		"qbittorrent_configured": qbtPassword != "",
		"torrent_seed_time_hours": seedTimeStr,
		"torrent_remove_after_seed": removeAfterSeed,

		// AI settings
		"openrouter_api_key":    maskKey(orKey),
		"openrouter_configured": orKey != "",
		"openrouter_model":      orModel,
		"brave_api_key":         maskKey(braveKey),
		"brave_configured":      braveKey != "",
		"ai_personality_preset": aiPreset,
		"ai_personality_custom": aiCustom,
	}

	writeJSON(w, 200, response)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Proxy      *string `json:"proxy"`
		MediaRoot  *string `json:"media_root"`
		TMDBKey    *string `json:"tmdb_api_key"`
		SabURL     *string `json:"sabnzbd_url"`
		SabKey     *string `json:"sabnzbd_api_key"`
		SetupDone  *bool   `json:"setup_complete"`

		// Music settings
		LastFMKey     *string `json:"lastfm_api_key"`

		// qBittorrent settings
		QBTEnabled       *bool   `json:"qbittorrent_enabled"`
		QBTURL           *string `json:"qbittorrent_url"`
		QBTUsername      *string `json:"qbittorrent_username"`
		QBTPassword      *string `json:"qbittorrent_password"`
		SeedTimeHours    *string `json:"torrent_seed_time_hours"`
		RemoveAfterSeed  *bool   `json:"torrent_remove_after_seed"`

		// AI settings
		OpenRouterKey    *string `json:"openrouter_api_key"`
		OpenRouterModel  *string `json:"openrouter_model"`
		BraveKey         *string `json:"brave_api_key"`
		AIPreset         *string `json:"ai_personality_preset"`
		AICustom         *string `json:"ai_personality_custom"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Proxy != nil {
		s.db.SetSetting("proxy", *req.Proxy)
	}
	if req.MediaRoot != nil {
		s.db.SetSetting("media_root", *req.MediaRoot)
	}
	if req.TMDBKey != nil {
		s.db.SetSetting("tmdb_api_key", *req.TMDBKey)
	}
	if req.SabURL != nil {
		s.db.SetSetting("sabnzbd_url", *req.SabURL)
	}
	if req.SabKey != nil {
		s.db.SetSetting("sabnzbd_api_key", *req.SabKey)
	}
	if req.SetupDone != nil && *req.SetupDone {
		s.db.SetSetting("setup_complete", "true")
	}

	// Music settings
	if req.LastFMKey != nil {
		s.db.SetSetting("lastfm_api_key", *req.LastFMKey)
	}

	// qBittorrent settings
	if req.QBTEnabled != nil {
		if *req.QBTEnabled {
			s.db.SetSetting("qbittorrent_enabled", "true")
		} else {
			s.db.SetSetting("qbittorrent_enabled", "false")
		}
	}
	if req.QBTURL != nil {
		s.db.SetSetting("qbittorrent_url", *req.QBTURL)
	}
	if req.QBTUsername != nil {
		s.db.SetSetting("qbittorrent_username", *req.QBTUsername)
	}
	if req.QBTPassword != nil {
		s.db.SetSetting("qbittorrent_password", *req.QBTPassword)
	}
	if req.SeedTimeHours != nil {
		s.db.SetSetting("torrent_seed_time_hours", *req.SeedTimeHours)
	}
	if req.RemoveAfterSeed != nil {
		if *req.RemoveAfterSeed {
			s.db.SetSetting("torrent_remove_after_seed", "true")
		} else {
			s.db.SetSetting("torrent_remove_after_seed", "false")
		}
	}

	// AI settings
	reinitAI := false
	if req.OpenRouterKey != nil {
		s.db.SetSetting("openrouter_api_key", *req.OpenRouterKey)
		reinitAI = true
	}
	if req.OpenRouterModel != nil {
		s.db.SetSetting("openrouter_model", *req.OpenRouterModel)
	}
	if req.BraveKey != nil {
		s.db.SetSetting("brave_api_key", *req.BraveKey)
		reinitAI = true
	}
	if req.AIPreset != nil {
		s.db.SetSetting("ai_personality_preset", *req.AIPreset)
	}
	if req.AICustom != nil {
		s.db.SetSetting("ai_personality_custom", *req.AICustom)
	}

	// Reload config
	s.cfg.Reload(s.db)

	// Update dependent services
	s.grabber.UpdateConfig(s.cfg.SABnzbdURL, s.cfg.SABnzbdKey)
	s.processor.UpdateMediaRoot(s.cfg.MediaRoot)
	s.scanner.UpdateMediaRoot(s.cfg.MediaRoot)

	// Update qBittorrent client
	if s.qbt != nil {
		s.qbt.UpdateConfig(s.cfg.QBTURL, s.cfg.QBTUsername, s.cfg.QBTPassword)
	}

	// Reinitialize TMDB client if the key was set/changed
	if s.cfg.TMDBApiKey != "" && (s.tmdb == nil || req.TMDBKey != nil) {
		s.tmdb = metadata.NewTMDBClient(s.proxyClient, s.cfg.TMDBApiKey)
		slog.Info("TMDB client initialized via settings update")
	}

	// Reinitialize Last.fm client if key was set/changed
	if s.cfg.LastFMApiKey != "" && (s.lastfm == nil || req.LastFMKey != nil) {
		s.lastfm = metadata.NewLastFMClient(s.proxyClient, s.cfg.LastFMApiKey, s.db, s.cfg.ConfigDir)
		slog.Info("Last.fm client initialized via settings update")
	}

	// Reinitialize AI clients if keys changed
	if reinitAI {
		s.initAI()
		slog.Info("AI clients reinitialized via settings update")
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// handleTestOpenRouter tests the OpenRouter API key with a minimal chat request.
func (s *Server) handleTestOpenRouter(w http.ResponseWriter, r *http.Request) {
	// Accept key from request body (unsaved input) or fall back to DB
	var req struct {
		Key string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	orKey := req.Key
	if orKey == "" {
		orKey, _ = s.db.GetSetting("openrouter_api_key")
	}
	if orKey == "" {
		writeJSON(w, 200, map[string]interface{}{"success": false, "error": "No API key configured"})
		return
	}

	// Use a minimal chat request to validate auth (models endpoint is public)
	client := ai.NewOpenRouterClient(s.proxyClient, orKey)
	_, err := client.Chat(ai.ChatRequest{
		Model:     "openai/gpt-3.5-turbo",
		Messages:  []ai.Message{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	})
	if err != nil {
		errStr := err.Error()
		// Auth failures typically contain 401 or "unauthorized"
		writeJSON(w, 200, map[string]interface{}{"success": false, "error": errStr})
		return
	}

	writeJSON(w, 200, map[string]interface{}{"success": true})
}

// handleTestQBittorrent tests the qBittorrent connection.
func (s *Server) handleTestQBittorrent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if s.qbt == nil {
		writeJSON(w, 200, map[string]interface{}{"success": false, "error": "qBittorrent client not initialized"})
		return
	}

	// Temporarily update config for the test if values provided
	if req.URL != "" || req.Username != "" || req.Password != "" {
		url := req.URL
		if url == "" {
			url = s.cfg.QBTURL
		}
		username := req.Username
		if username == "" {
			username = s.cfg.QBTUsername
		}
		password := req.Password
		if password == "" {
			password = s.cfg.QBTPassword
		}
		s.qbt.UpdateConfig(url, username, password)
	}

	if err := s.qbt.TestConnection(); err != nil {
		writeJSON(w, 200, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]interface{}{"success": true})
}

func maskKey(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[:4] + "..." + key[len(key)-2:]
}
