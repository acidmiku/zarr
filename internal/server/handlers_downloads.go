package server

import (
	"database/sql"
	"net/http"
)

func (s *Server) handleGetDownloads(w http.ResponseWriter, r *http.Request) {
	// Get active downloads from database
	rows, err := s.db.Query(`SELECT d.id, d.media_item_id, d.episode_id, d.nzb_title, d.sabnzbd_nzo_id,
		d.status, d.quality, d.score, d.started_at, d.completed_at,
		m.title as media_title, m.type as media_type
		FROM downloads d
		JOIN media_items m ON d.media_item_id = m.id
		WHERE d.status NOT IN ('imported')
		ORDER BY d.started_at DESC`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type downloadEntry struct {
		ID          int    `json:"id"`
		MediaItemID int    `json:"media_item_id"`
		EpisodeID   *int   `json:"episode_id,omitempty"`
		NZBTitle    string `json:"nzb_title"`
		NzoID       string `json:"nzo_id"`
		Status      string `json:"status"`
		Quality     string `json:"quality"`
		Score       int    `json:"score"`
		StartedAt   string `json:"started_at"`
		CompletedAt string `json:"completed_at,omitempty"`
		MediaTitle  string `json:"media_title"`
		MediaType   string `json:"media_type"`
		Percentage  string `json:"percentage,omitempty"`
		Speed       string `json:"speed,omitempty"`
		TimeLeft    string `json:"time_left,omitempty"`
	}

	var downloads []downloadEntry

	// Get queue info from SABnzbd for progress data
	queue, _ := s.grabber.GetQueue()
	queueMap := make(map[string]*downloadEntry)

	for rows.Next() {
		var d downloadEntry
		var epID sql.NullInt64
		var nzoID, quality, completedAt sql.NullString
		var score sql.NullInt64

		if err := rows.Scan(&d.ID, &d.MediaItemID, &epID, &d.NZBTitle, &nzoID,
			&d.Status, &quality, &score, &d.StartedAt, &completedAt,
			&d.MediaTitle, &d.MediaType); err != nil {
			continue
		}

		if epID.Valid {
			v := int(epID.Int64)
			d.EpisodeID = &v
		}
		if nzoID.Valid { d.NzoID = nzoID.String }
		if quality.Valid { d.Quality = quality.String }
		if score.Valid { d.Score = int(score.Int64) }
		if completedAt.Valid { d.CompletedAt = completedAt.String }

		downloads = append(downloads, d)
		if d.NzoID != "" {
			queueMap[d.NzoID] = &downloads[len(downloads)-1]
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
		COALESCE(m.title, '') as media_title
		FROM activity_log a
		LEFT JOIN media_items m ON a.media_item_id = m.id
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
		Action      string `json:"action"`
		Details     string `json:"details"`
		CreatedAt   string `json:"created_at"`
		MediaTitle  string `json:"media_title"`
	}

	var activities []activityEntry
	for rows.Next() {
		var a activityEntry
		var mediaID, epID sql.NullInt64
		var details sql.NullString

		if err := rows.Scan(&a.ID, &mediaID, &epID, &a.Action, &details, &a.CreatedAt, &a.MediaTitle); err != nil {
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

func (s *Server) handleRetryDownload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var mediaItemID int
	var epID sql.NullInt64
	err = s.db.QueryRow(`SELECT media_item_id, episode_id FROM downloads WHERE id = ? AND status = 'failed'`,
		id).Scan(&mediaItemID, &epID)
	if err != nil {
		writeError(w, 404, "failed download not found")
		return
	}

	// Reset the episode status to wanted
	if epID.Valid {
		s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ?`, epID.Int64)
		s.searchForEpisodeSilent(mediaItemID, int(epID.Int64))
	} else {
		go func() {
			// Re-search for movie
			s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ?`, mediaItemID)
		}()
	}

	writeJSON(w, 200, map[string]string{"status": "retrying"})
}

func (s *Server) handleCancelDownload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var nzoID sql.NullString
	var epID sql.NullInt64
	var mediaItemID int
	err = s.db.QueryRow(`SELECT sabnzbd_nzo_id, episode_id, media_item_id FROM downloads WHERE id = ?`, id).Scan(&nzoID, &epID, &mediaItemID)
	if err != nil {
		writeError(w, 404, "download not found")
		return
	}

	// Remove from SABnzbd
	if nzoID.Valid && nzoID.String != "" {
		s.grabber.DeleteFromQueue(nzoID.String)
	}

	// Update database
	s.db.Exec(`UPDATE downloads SET status = 'failed' WHERE id = ?`, id)
	if epID.Valid {
		s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ?`, epID.Int64)
	} else {
		// Movie download — reset media item status if no other active downloads
		var activeCount int
		s.db.QueryRow(`SELECT COUNT(*) FROM downloads WHERE media_item_id = ? AND id != ? AND status NOT IN ('imported', 'failed')`,
			mediaItemID, id).Scan(&activeCount)
		if activeCount == 0 {
			s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ?`, mediaItemID)
		}
	}

	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}
