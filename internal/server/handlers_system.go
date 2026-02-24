package server

import (
	"net/http"
	"os"
	"runtime"
	"syscall"
)

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"version":  "0.1.0",
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
		"go":       runtime.Version(),
	}

	// Database status
	if err := s.db.Ping(); err != nil {
		status["database"] = "error"
	} else {
		status["database"] = "ok"
	}

	// SABnzbd status
	if err := s.grabber.TestConnection(); err != nil {
		status["sabnzbd"] = "unreachable"
	} else {
		status["sabnzbd"] = "connected"
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

func getDiskSpace(path string) diskSpace {
	if path == "" {
		return diskSpace{}
	}
	os.MkdirAll(path, 0755)

	ds := diskSpace{}

	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) == nil {
		ds.Total = stat.Blocks * uint64(stat.Bsize) / (1024 * 1024 * 1024)
		ds.Free = stat.Bfree * uint64(stat.Bsize) / (1024 * 1024 * 1024)
		ds.Available = stat.Bavail * uint64(stat.Bsize) / (1024 * 1024 * 1024)
	}

	return ds
}
