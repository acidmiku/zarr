package server

import (
	"context"
	"mediaforge/internal/qbt"
	"mediaforge/internal/sabnzbd"
	"net/http"
	"runtime"
	"time"
)

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"version":  "0.3.0",
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
		"go":       runtime.Version(),
	}

	// Database status
	if err := s.db.Ping(); err != nil {
		status["database"] = "error"
	} else {
		status["database"] = "ok"
	}

	// Unconfigured services do not delay the first-run UI. Configured clients
	// are checked concurrently with a short bound, independently of queue polling.
	type serviceStatus struct{ name, state string }
	checks := make(chan serviceStatus, 2)
	count := 0
	status["sabnzbd"] = "not_configured"
	status["qbittorrent"] = "disabled"
	if s.cfg.SABnzbdKey != "" {
		count++
		go func() {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			state := "connected"
			if sabnzbd.New(s.directClient, s.cfg.SABnzbdURL, s.cfg.SABnzbdKey).Test(ctx) != nil {
				state = "unreachable"
			}
			checks <- serviceStatus{"sabnzbd", state}
		}()
	}
	if s.cfg.QBTEnabled {
		count++
		go func() {
			client := *s.directClient
			client.Timeout = 3 * time.Second
			state := "connected"
			if qbt.New(&client, s.cfg.QBTURL, s.cfg.QBTUsername, s.cfg.QBTPassword).TestConnection() != nil {
				state = "unreachable"
			}
			checks <- serviceStatus{"qbittorrent", state}
		}()
	}
	for i := 0; i < count; i++ {
		result := <-checks
		status[result.name] = result.state
	}

	// Setup status
	status["setup_complete"] = s.db.IsSetupComplete()

	// Disk space
	status["disk"] = getDiskSpace(s.cfg.MediaRoot)

	// Library stats
	var movieCount, seriesCount, animeCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE type = 'movie'`).Scan(&movieCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE type = 'series' AND anime = FALSE`).Scan(&seriesCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE anime = TRUE`).Scan(&animeCount)

	status["library"] = map[string]int{
		"movies": movieCount,
		"series": seriesCount,
		"anime":  animeCount,
	}

	writeJSON(w, 200, status)
}

func (s *Server) handleTriggerScan(w http.ResponseWriter, r *http.Request) {
	result, err := s.scanner.Scan()
	if err != nil {
		writeError(w, 500, "scan failed: "+err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	lines := queryInt(r, "lines", 100)

	rows, err := s.db.Query(`SELECT action || ': ' || COALESCE(details, '') || ' [' || created_at || ']'
		FROM activity_log ORDER BY created_at DESC LIMIT ?`, lines)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	var logLines []string
	for rows.Next() {
		var line string
		rows.Scan(&line)
		logLines = append(logLines, line)
	}

	if logLines == nil {
		logLines = []string{}
	}

	writeJSON(w, 200, logLines)
}

type diskSpace struct {
	Total     uint64 `json:"total_gb"`
	Free      uint64 `json:"free_gb"`
	Available uint64 `json:"available_gb"`
}
