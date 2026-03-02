package server

import (
	"database/sql"
	"fmt"
	"net/http"
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
		WHERE d.status NOT IN ('imported')
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

	// Get queue info from SABnzbd for progress data
	queue, _ := s.grabber.GetQueue()
	queueMap := make(map[string]*downloadEntry)
	// Collect qbt hashes for torrent progress
	qbtHashMap := make(map[string]*downloadEntry)

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
		if nzoID.Valid { d.NzoID = nzoID.String }
		if quality.Valid { d.Quality = quality.String }
		if score.Valid { d.Score = int(score.Int64) }
		if completedAt.Valid { d.CompletedAt = completedAt.String }
		if qbtHash.Valid { d.QbtHash = qbtHash.String }
		if seedRatio.Valid { d.SeedRatio = &seedRatio.Float64 }

		downloads = append(downloads, d)
		if d.NzoID != "" {
			queueMap[d.NzoID] = &downloads[len(downloads)-1]
		}
		if d.QbtHash != "" {
			qbtHashMap[d.QbtHash] = &downloads[len(downloads)-1]
		}
	}

	// Merge SABnzbd queue data
	if queue != nil {
		for _, slot := range queue.Slots {
			if d, ok := queueMap[slot.NzoID]; ok {
				d.Percentage = slot.Percentage
				d.Speed = queue.Speed
				d.TimeLeft = slot.TimeLeft
			}
		}
	}

	// Merge qBittorrent torrent data
	if len(qbtHashMap) > 0 && s.qbt != nil {
		torrents, err := s.qbt.GetTorrents()
		if err == nil {
			for _, t := range torrents {
				if d, ok := qbtHashMap[t.Hash]; ok {
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
	}

	if downloads == nil {
		downloads = []downloadEntry{}
	}

	writeJSON(w, 200, downloads)
}

func (s *Server) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 50)
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
		if details.Valid { a.Details = details.String }

		activities = append(activities, a)
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
	err = s.db.QueryRow(`SELECT media_item_id, episode_id, album_id FROM downloads WHERE id = ? AND status = 'failed'`,
		id).Scan(&mediaItemID, &epID, &albumID)
	if err != nil {
		writeError(w, 404, "failed download not found")
		return
	}

	if epID.Valid && mediaItemID.Valid {
		s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ?`, epID.Int64)
		s.searchForEpisodeSilent(int(mediaItemID.Int64), int(epID.Int64))
	} else if mediaItemID.Valid {
		go func() {
			s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ?`, mediaItemID.Int64)
		}()
	} else if albumID.Valid {
		s.db.Exec(`UPDATE albums SET status = 'wanted' WHERE id = ?`, albumID.Int64)
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

	// Remove from download client
	if dlType == "torrent" && qbtHash.Valid && qbtHash.String != "" && s.qbt != nil {
		s.qbt.DeleteTorrent(qbtHash.String, true)
	} else if nzoID.Valid && nzoID.String != "" {
		s.grabber.DeleteFromQueue(nzoID.String)
	}

	// Update database
	s.db.Exec(`UPDATE downloads SET status = 'failed' WHERE id = ?`, id)
	if epID.Valid {
		s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ?`, epID.Int64)
	} else if mediaItemID.Valid {
		var activeCount int
		s.db.QueryRow(`SELECT COUNT(*) FROM downloads WHERE media_item_id = ? AND id != ? AND status NOT IN ('imported', 'failed')`,
			mediaItemID.Int64, id).Scan(&activeCount)
		if activeCount == 0 {
			s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ?`, mediaItemID.Int64)
		}
	} else if albumID.Valid {
		s.db.Exec(`UPDATE albums SET status = 'wanted' WHERE id = ?`, albumID.Int64)
	}

	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}
