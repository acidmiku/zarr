package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"mediaforge/internal/indexer"
)

func (s *Server) handleSearchReleases(w http.ResponseWriter, r *http.Request) {
	mediaItemID := queryInt(r, "media_item_id", 0)
	episodeID := queryInt(r, "episode_id", 0)

	if mediaItemID == 0 {
		writeError(w, 400, "media_item_id required")
		return
	}

	var releases []indexer.Release

	if episodeID > 0 {
		var err error
		releases, err = s.findReleasesForEpisode(mediaItemID, episodeID)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
	} else {
		// Movie or whole-series search
		var mediaType string
		s.db.QueryRow(`SELECT type FROM media_items WHERE id = ?`, mediaItemID).Scan(&mediaType)

		if mediaType == "series" {
			// Series-wide: search for the first wanted episode as a representative
			var epID int
			err := s.db.QueryRow(`SELECT e.id FROM episodes e
				JOIN seasons s ON e.season_id = s.id
				WHERE e.media_item_id = ? AND e.episode_type = 'standard'
				ORDER BY s.number, e.number LIMIT 1`, mediaItemID).Scan(&epID)
			if err == nil {
				releases, _ = s.findReleasesForEpisode(mediaItemID, epID)
			}
		} else {
			// Movie search
			var title, imdbID string
			var year, profileID int
			s.db.QueryRow(`SELECT title, COALESCE(imdb_id,''), COALESCE(year,0), COALESCE(quality_profile_id,0)
				FROM media_items WHERE id = ?`, mediaItemID).Scan(&title, &imdbID, &year, &profileID)

			allIndexers := s.loadIndexers()

			newznabIdxs := filterIndexersByType(allIndexers, "newznab", "movie")
			releases = s.newznab.SearchMovie(newznabIdxs, imdbID, title, year)
			for i := range releases {
				releases[i].DownloadType = "nzb"
			}

			rtIndexers := filterIndexersByType(allIndexers, "rutracker", "movie")
			if len(rtIndexers) > 0 {
				query := title
				if year > 0 {
					query = fmt.Sprintf("%s %d", title, year)
				}
				rtReleases := s.searchRutracker(rtIndexers, query, "movie")
				releases = append(releases, rtReleases...)
			}

			var profile indexer.QualityProfile
			s.db.QueryRow(`SELECT id, name, qualities, COALESCE(tags,'{}'), language, COALESCE(reject_patterns,'[]'), upgrade_allowed
				FROM quality_profiles WHERE id = ?`, profileID).Scan(
				&profile.ID, &profile.Name, &profile.Qualities, &profile.Tags,
				&profile.Language, &profile.RejectPatterns, &profile.UpgradeAllowed)

			for i := range releases {
				indexer.ScoreRelease(&releases[i], &profile)
			}
		}
	}

	// Mark blacklisted releases as rejected (but keep visible for browsing)
	blacklist := s.loadBlacklist(mediaItemID, episodeID)
	releases = indexer.MarkBlacklisted(releases, blacklist)

	if releases == nil {
		releases = []indexer.Release{}
	}

	writeJSON(w, 200, releases)
}

// loadBlacklist returns blacklisted release titles for the server handlers.
func (s *Server) loadBlacklist(mediaItemID, episodeID int) []string {
	var query string
	var arg int
	if episodeID > 0 {
		query = `SELECT release_title FROM release_blacklist WHERE episode_id = ?`
		arg = episodeID
	} else if mediaItemID > 0 {
		query = `SELECT release_title FROM release_blacklist WHERE media_item_id = ? AND episode_id IS NULL`
		arg = mediaItemID
	} else {
		return nil
	}
	rows, err := s.db.Query(query, arg)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var t string
		rows.Scan(&t)
		titles = append(titles, t)
	}
	return titles
}

func (s *Server) handleGrabRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReleaseURL   string `json:"release_url"`
		Title        string `json:"title"`
		MediaItemID  int    `json:"media_item_id"`
		EpisodeID    int    `json:"episode_id"`
		DownloadType string `json:"download_type"`
		TopicID      int    `json:"topic_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.MediaItemID == 0 {
		writeError(w, 400, "media_item_id required")
		return
	}

	grabTitle := req.Title
	if grabTitle == "" {
		grabTitle = "manual-grab"
	}

	var epID interface{}
	if req.EpisodeID > 0 {
		epID = req.EpisodeID
		s.db.Exec(`UPDATE episodes SET status = 'downloading' WHERE id = ?`, req.EpisodeID)
	} else {
		s.db.Exec(`UPDATE media_items SET status = 'downloading' WHERE id = ?`, req.MediaItemID)
	}

	if req.DownloadType == "torrent" && req.TopicID > 0 {
		rtIndexers := filterIndexersByType(s.loadIndexers(), "rutracker", "")
		if len(rtIndexers) == 0 {
			writeError(w, 400, "no rutracker indexers configured")
			return
		}
		idx := rtIndexers[0]

		torrentData, err := s.rutracker.DownloadTorrent(req.TopicID, idx.Username, idx.Password)
		if err != nil {
			writeError(w, 502, "torrent download failed: "+err.Error())
			return
		}

		result, err := s.db.Exec(`INSERT INTO downloads (media_item_id, episode_id, nzb_title, download_type) VALUES (?, ?, ?, 'torrent')`,
			req.MediaItemID, epID, grabTitle)
		if err != nil {
			writeError(w, 500, "database error: "+err.Error())
			return
		}
		dlID, _ := result.LastInsertId()

		_, err = s.qbt.AddTorrent(torrentData, fmt.Sprintf("dl_%d.torrent", dlID), int(dlID))
		if err != nil {
			writeError(w, 502, "qBittorrent failed: "+err.Error())
			return
		}

		s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details)
			VALUES (?, ?, 'grabbed', ?)`, req.MediaItemID, epID, "Manual grab: "+grabTitle)

		writeJSON(w, 200, map[string]string{"status": "grabbed"})
		return
	}

	if req.ReleaseURL == "" {
		writeError(w, 400, "release_url required for NZB grab")
		return
	}

	nzoID, err := s.grabber.GrabNZB(req.ReleaseURL, grabTitle)
	if err != nil {
		writeError(w, 500, "grab failed: "+err.Error())
		return
	}

	s.db.Exec(`INSERT INTO downloads (media_item_id, episode_id, nzb_title, sabnzbd_nzo_id, download_type)
		VALUES (?, ?, ?, ?, 'nzb')`, req.MediaItemID, epID, grabTitle, nzoID)

	s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details)
		VALUES (?, ?, 'grabbed', ?)`, req.MediaItemID, epID, "Manual grab: "+grabTitle)

	writeJSON(w, 200, map[string]string{"status": "grabbed", "nzo_id": nzoID})
}
