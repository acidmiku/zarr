package server

import (
	"database/sql"
	"fmt"
	"log/slog"
	"mediaforge/internal/grabber"
	"mediaforge/internal/music"
	"net/http"
	"os"
)

func (s *Server) handleGetDownloads(w http.ResponseWriter, r *http.Request) {
	// Get active downloads from database — LEFT JOIN both media_items and albums
	rows, err := s.db.Query(`SELECT d.id, d.media_item_id, d.episode_id, d.nzb_title, d.sabnzbd_nzo_id,
		d.status, d.quality, d.score, d.started_at, d.completed_at,
		COALESCE(m.title, a.title, '') as media_title,
		COALESCE(m.type, CASE WHEN d.album_id IS NOT NULL THEN 'music' ELSE '' END) as media_type,
		d.download_type, d.qbt_hash, d.seed_ratio, d.album_id
		FROM downloads d
		LEFT JOIN media_items m ON d.media_item_id = m.id
		LEFT JOIN albums a ON d.album_id = a.id
		WHERE d.status NOT IN ('imported', 'cancelled')
		ORDER BY d.started_at DESC`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type downloadEntry struct {
		ID           int      `json:"id"`
		MediaItemID  *int     `json:"media_item_id,omitempty"`
		EpisodeID    *int     `json:"episode_id,omitempty"`
		AlbumID      *int     `json:"album_id,omitempty"`
		NZBTitle     string   `json:"nzb_title"`
		NzoID        string   `json:"nzo_id"`
		Status       string   `json:"status"`
		Quality      string   `json:"quality"`
		Score        int      `json:"score"`
		StartedAt    string   `json:"started_at"`
		CompletedAt  string   `json:"completed_at,omitempty"`
		MediaTitle   string   `json:"media_title"`
		MediaType    string   `json:"media_type"`
		DownloadType string   `json:"download_type"`
		QbtHash      string   `json:"qbt_hash,omitempty"`
		SeedRatio    *float64 `json:"seed_ratio,omitempty"`
		Percentage   string   `json:"percentage,omitempty"`
		Speed        string   `json:"speed,omitempty"`
		TimeLeft     string   `json:"time_left,omitempty"`
	}

	var downloads []downloadEntry

	// Index-based maps — pointers into downloads[] are unsafe while the slice grows.
	queueIdx := make(map[string]int)
	qbtIdx := make(map[string]int)

	for rows.Next() {
		var d downloadEntry
		var mediaItemID, epID, albumID sql.NullInt64
		var nzoID, quality, completedAt, qbtHash sql.NullString
		var score sql.NullInt64
		var seedRatio sql.NullFloat64

		if err := rows.Scan(&d.ID, &mediaItemID, &epID, &d.NZBTitle, &nzoID,
			&d.Status, &quality, &score, &d.StartedAt, &completedAt,
			&d.MediaTitle, &d.MediaType,
			&d.DownloadType, &qbtHash, &seedRatio, &albumID); err != nil {
			slog.Warn("downloads scan failed", "error", err)
			continue
		}

		if mediaItemID.Valid {
			v := int(mediaItemID.Int64)
			d.MediaItemID = &v
		}
		if epID.Valid {
			v := int(epID.Int64)
			d.EpisodeID = &v
		}
		if albumID.Valid {
			v := int(albumID.Int64)
			d.AlbumID = &v
		}
		if nzoID.Valid {
			d.NzoID = nzoID.String
		}
		if quality.Valid {
			d.Quality = quality.String
		}
		if score.Valid {
			d.Score = int(score.Int64)
		}
		if completedAt.Valid {
			d.CompletedAt = completedAt.String
		}
		if qbtHash.Valid {
			d.QbtHash = qbtHash.String
		}
		if seedRatio.Valid {
			d.SeedRatio = &seedRatio.Float64
		}

		downloads = append(downloads, d)
		idx := len(downloads) - 1
		if d.NzoID != "" && d.Status != "failed" && d.Status != "completed" {
			queueIdx[d.NzoID] = idx
		}
		if d.QbtHash != "" && d.Status != "failed" && d.Status != "completed" {
			qbtIdx[d.QbtHash] = idx
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "database error")
		return
	}

	rows.Close()
	var queue *grabber.SABQueue
	if len(queueIdx) > 0 && s.grabber != nil {
		queue, _ = s.grabber.GetQueue()
	}

	// Merge SABnzbd queue data
	if queue != nil {
		for _, slot := range queue.Slots {
			if idx, ok := queueIdx[slot.NzoID]; ok {
				downloads[idx].Percentage = slot.Percentage
				downloads[idx].Speed = queue.Speed
				downloads[idx].TimeLeft = slot.TimeLeft
			}
		}
	}

	// Merge qBittorrent torrent data
	if len(qbtIdx) > 0 && s.qbt != nil {
		torrents, err := s.qbt.GetTorrents()
		if err == nil {
			for _, t := range torrents {
				idx, ok := qbtIdx[t.Hash]
				if !ok {
					continue
				}
				d := &downloads[idx]
				pct := int(t.Progress * 100)
				d.Percentage = fmt.Sprintf("%d", pct)
				if t.DlSpeed > 0 {
					d.Speed = formatSpeed(t.DlSpeed)
				} else if t.UpSpeed > 0 {
					d.Speed = formatSpeed(t.UpSpeed) + " UP"
				}
				ratio := t.Ratio
				d.SeedRatio = &ratio
				if t.ETA > 0 && t.ETA < 8640000 {
					d.TimeLeft = formatETA(t.ETA)
				}
			}
		}
	}

	if downloads == nil {
		downloads = []downloadEntry{}
	}

	writeJSON(w, 200, downloads)
}

func (s *Server) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 50)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	rows, err := s.db.Query(`SELECT a.id, a.media_item_id, a.episode_id, a.action, a.details, a.created_at,
		COALESCE(m.title, al.title, '') as media_title,
		a.album_id
		FROM activity_log a
		LEFT JOIN media_items m ON a.media_item_id = m.id
		LEFT JOIN albums al ON a.album_id = al.id
		ORDER BY a.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type activityEntry struct {
		ID          int    `json:"id"`
		MediaItemID *int   `json:"media_item_id,omitempty"`
		EpisodeID   *int   `json:"episode_id,omitempty"`
		AlbumID     *int   `json:"album_id,omitempty"`
		Action      string `json:"action"`
		Details     string `json:"details"`
		CreatedAt   string `json:"created_at"`
		MediaTitle  string `json:"media_title"`
	}

	var activities []activityEntry
	for rows.Next() {
		var a activityEntry
		var mediaID, epID, albumID sql.NullInt64
		var details sql.NullString

		if err := rows.Scan(&a.ID, &mediaID, &epID, &a.Action, &details, &a.CreatedAt, &a.MediaTitle, &albumID); err != nil {
			slog.Warn("activity scan failed", "error", err)
			continue
		}
		if mediaID.Valid {
			v := int(mediaID.Int64)
			a.MediaItemID = &v
		}
		if epID.Valid {
			v := int(epID.Int64)
			a.EpisodeID = &v
		}
		if albumID.Valid {
			v := int(albumID.Int64)
			a.AlbumID = &v
		}
		if details.Valid {
			a.Details = details.String
		}

		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "database error")
		return
	}

	if activities == nil {
		activities = []activityEntry{}
	}

	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM activity_log`).Scan(&total)

	writeJSON(w, 200, map[string]interface{}{
		"items": activities,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func formatSpeed(bytesPerSec int64) string {
	if bytesPerSec > 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytesPerSec)/1024/1024)
	}
	return fmt.Sprintf("%.0f KB", float64(bytesPerSec)/1024)
}

func formatETA(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dh%dm", seconds/3600, (seconds%3600)/60)
}

func (s *Server) handleRetryDownload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var mediaItemID sql.NullInt64
	var epID, albumID sql.NullInt64
	var path, upgradeFrom sql.NullString
	var kind string
	err = s.db.QueryRow(`SELECT media_item_id, episode_id, album_id, download_path, download_type, upgrade_from_title FROM downloads WHERE id = ? AND status = 'failed'`,
		id).Scan(&mediaItemID, &epID, &albumID, &path, &kind, &upgradeFrom)
	if err != nil {
		writeError(w, 404, "failed download not found")
		return
	}
	// Retry an already downloaded file before requesting another release.
	if path.Valid && path.String != "" {
		if _, err := os.Stat(path.String); err == nil {
			if kind == "torrent" {
				err = s.processor.ProcessTorrent(id, path.String)
			} else {
				err = s.processor.Process(id, path.String)
			}
			if err != nil {
				s.db.Exec(`UPDATE downloads SET error_message=? WHERE id=?`, "Import failed: "+err.Error(), id)
				writeError(w, 422, "Import failed: "+err.Error())
				return
			}
			writeJSON(w, 200, map[string]string{"status": "imported"})
			return
		}
	}
	if upgradeFrom.Valid {
		// A fresh upgrade check must apply today's profile and current release,
		// rather than falling through to an unconditional wanted-item grab.
		if epID.Valid {
			_, err = s.db.Exec(`UPDATE episodes SET upgrade_checked_at=NULL WHERE id=?`, epID.Int64)
		} else {
			_, err = s.db.Exec(`UPDATE media_items SET upgrade_checked_at=NULL WHERE id=?`, mediaItemID.Int64)
		}
		if err != nil {
			writeError(w, 500, "could not schedule upgrade check")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "retrying", "message": "Upgrade is eligible for the next quality scan (runs every 30 minutes)."})
		return
	}
	if epID.Valid && mediaItemID.Valid {
		s.db.Exec(`UPDATE episodes SET status='wanted' WHERE id=? AND status!='available'`, epID.Int64)
		s.searchForEpisode(w, int(mediaItemID.Int64), int(epID.Int64))
		return
	} else if mediaItemID.Valid {
		s.db.Exec(`UPDATE media_items SET status='wanted' WHERE id=? AND status!='available'`, mediaItemID.Int64)
		s.searchForMovie(w, int(mediaItemID.Int64))
		return
	} else if albumID.Valid {
		s.db.Exec(`UPDATE albums SET status='wanted' WHERE id=? AND status!='available'`, albumID.Int64)
		acquisition, requestErr := music.Request(s.db, int(albumID.Int64), false)
		if requestErr != nil {
			writeError(w, 500, "could not save music retry")
			return
		}
		writeJSON(w, 202, map[string]any{"status": "retrying", "acquisition": acquisition})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "retrying"})
}

func (s *Server) handleCancelDownload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var nzoID, qbtHash sql.NullString
	var epID, mediaItemID, albumID sql.NullInt64
	var dlType string
	err = s.db.QueryRow(`SELECT sabnzbd_nzo_id, episode_id, media_item_id, album_id, download_type, qbt_hash FROM downloads WHERE id = ?`, id).
		Scan(&nzoID, &epID, &mediaItemID, &albumID, &dlType, &qbtHash)
	if err != nil {
		writeError(w, 404, "download not found")
		return
	}

	if err := s.cancelTrackedDownload(id, dlType, qbtHash, nzoID); err != nil {
		writeError(w, 502, "Cancellation failed: "+err.Error())
		return
	}
	if epID.Valid {
		s.db.Exec(`UPDATE episodes SET status='wanted' WHERE id=? AND status!='available' AND NOT EXISTS (SELECT 1 FROM downloads WHERE episode_id=? AND id!=? AND status IN ('queued','downloading','extracting'))`, epID.Int64, epID.Int64, id)
	} else if mediaItemID.Valid {
		s.db.Exec(`UPDATE media_items SET status='wanted' WHERE id=? AND status!='available' AND NOT EXISTS (SELECT 1 FROM downloads WHERE media_item_id=? AND id!=? AND status IN ('queued','downloading','extracting'))`, mediaItemID.Int64, mediaItemID.Int64, id)
	} else if albumID.Valid {
		music.Pause(s.db, int(albumID.Int64))
		s.db.Exec(`UPDATE albums SET status='wanted' WHERE id=? AND status!='available' AND NOT EXISTS (SELECT 1 FROM downloads WHERE album_id=? AND id!=? AND status IN ('queued','downloading','extracting'))`, albumID.Int64, albumID.Int64, id)
	}

	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

// handleClearFailedDownloads removes all failed downloads from the queue.
// Hides them from the activity view; doesn't restart anything.
func (s *Server) handleClearFailedDownloads(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.Exec(`DELETE FROM downloads WHERE status = 'failed'`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, 200, map[string]any{"cleared": n})
}
