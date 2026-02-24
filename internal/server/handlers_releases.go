package server

import (
	"encoding/json"
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
		// Movie search
		var title, imdbID string
		var year, profileID int
		s.db.QueryRow(`SELECT title, COALESCE(imdb_id,''), COALESCE(year,0), COALESCE(quality_profile_id,0)
			FROM media_items WHERE id = ?`, mediaItemID).Scan(&title, &imdbID, &year, &profileID)

		idxConfigs := s.loadIndexers()
		releases = s.newznab.SearchMovie(idxConfigs, imdbID, title, year)

		var profile indexer.QualityProfile
		s.db.QueryRow(`SELECT id, name, qualities, COALESCE(tags,'{}'), language, COALESCE(reject_patterns,'[]'), upgrade_allowed
			FROM quality_profiles WHERE id = ?`, profileID).Scan(
			&profile.ID, &profile.Name, &profile.Qualities, &profile.Tags,
			&profile.Language, &profile.RejectPatterns, &profile.UpgradeAllowed)

		for i := range releases {
			indexer.ScoreRelease(&releases[i], &profile)
		}
	}

	if releases == nil {
		releases = []indexer.Release{}
	}

	writeJSON(w, 200, releases)
}

func (s *Server) handleGrabRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReleaseURL  string `json:"release_url"`
		MediaItemID int    `json:"media_item_id"`
		EpisodeID   int    `json:"episode_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.ReleaseURL == "" || req.MediaItemID == 0 {
		writeError(w, 400, "release_url and media_item_id required")
		return
	}

	nzoID, err := s.grabber.GrabNZB(req.ReleaseURL, "manual-grab")
	if err != nil {
		writeError(w, 500, "grab failed: "+err.Error())
		return
	}

	var epID interface{}
	if req.EpisodeID > 0 {
		epID = req.EpisodeID
		s.db.Exec(`UPDATE episodes SET status = 'downloading' WHERE id = ?`, req.EpisodeID)
	} else {
		s.db.Exec(`UPDATE media_items SET status = 'downloading' WHERE id = ?`, req.MediaItemID)
	}

	s.db.Exec(`INSERT INTO downloads (media_item_id, episode_id, nzb_title, sabnzbd_nzo_id)
		VALUES (?, ?, 'manual-grab', ?)`, req.MediaItemID, epID, nzoID)

	s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details)
		VALUES (?, ?, 'grabbed', 'Manual grab')`, req.MediaItemID, epID)

	writeJSON(w, 200, map[string]string{"status": "grabbed", "nzo_id": nzoID})
}
