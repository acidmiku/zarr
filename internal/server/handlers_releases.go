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
			var anime bool
			s.db.QueryRow(`SELECT title, COALESCE(imdb_id,''), COALESCE(year,0), COALESCE(quality_profile_id,0), anime
				FROM media_items WHERE id = ?`, mediaItemID).Scan(&title, &imdbID, &year, &profileID, &anime)

			allIndexers := s.loadIndexers()

			newznabIdxs := indexer.MovieIndexers(allIndexers, "newznab", anime)
			releases = s.newznab.SearchMovie(newznabIdxs, imdbID, title, year)
			for i := range releases {
				releases[i].DownloadType = "nzb"
			}

			rtIndexers := indexer.MovieIndexers(allIndexers, "rutracker", anime)
			if len(rtIndexers) > 0 {
				query := title
				if year > 0 {
					query = fmt.Sprintf("%s %d", title, year)
				}
				rtContentType := "movie"
				if anime {
					rtContentType = "anime"
				}
				rtReleases := indexer.FilterMovieReleases(s.searchRutracker(rtIndexers, query, rtContentType), title, year)
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

	if req.MediaItemID <= 0 || req.EpisodeID < 0 {
		writeError(w, 400, "invalid library item")
		return
	}
	nzoID, err := s.enqueueRelease(req.MediaItemID, req.EpisodeID, 0, indexer.Release{Title: req.Title, NZBURL: req.ReleaseURL, DownloadType: req.DownloadType, TopicID: req.TopicID})
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "grabbed", "nzo_id": nzoID})
}
