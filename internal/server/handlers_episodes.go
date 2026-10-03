package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mediaforge/internal/indexer"
	"mediaforge/internal/rutracker"
)

func (s *Server) handleGetEpisodes(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	// Get seasons
	seasonRows, err := s.db.Query(`SELECT id, number, title, overview, poster_url
		FROM seasons WHERE media_item_id = ? ORDER BY number`, mediaID)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer seasonRows.Close()

	type episodeEntry struct {
		ID             int    `json:"id"`
		Number         int    `json:"number"`
		AbsoluteNumber *int   `json:"absolute_number,omitempty"`
		EpisodeType    string `json:"episode_type"`
		Title          string `json:"title"`
		Overview       string `json:"overview"`
		AirDate        string `json:"air_date"`
		Status         string `json:"status"`
		FilePath       string `json:"file_path,omitempty"`
	}

	type seasonEntry struct {
		ID        int            `json:"id"`
		Number    int            `json:"number"`
		Title     string         `json:"title"`
		Overview  string         `json:"overview"`
		PosterURL string         `json:"poster_url"`
		Episodes  []episodeEntry `json:"episodes"`
	}

	var seasons []seasonEntry
	for seasonRows.Next() {
		var se seasonEntry
		var nullTitle, nullOverview, nullPoster sql.NullString
		if err := seasonRows.Scan(&se.ID, &se.Number, &nullTitle, &nullOverview, &nullPoster); err != nil {
			continue
		}
		if nullTitle.Valid {
			se.Title = nullTitle.String
		}
		if nullOverview.Valid {
			se.Overview = nullOverview.String
		}
		if nullPoster.Valid {
			se.PosterURL = nullPoster.String
		}

		// Get episodes for this season
		epRows, err := s.db.Query(`SELECT id, number, absolute_number, episode_type, title, overview, air_date, status, file_path
			FROM episodes WHERE season_id = ? ORDER BY number`, se.ID)
		if err != nil {
			continue
		}

		for epRows.Next() {
			var ep episodeEntry
			var absNum sql.NullInt64
			var nullEpTitle, nullOverview, nullAirDate, nullFilePath sql.NullString
			if err := epRows.Scan(&ep.ID, &ep.Number, &absNum, &ep.EpisodeType,
				&nullEpTitle, &nullOverview, &nullAirDate, &ep.Status, &nullFilePath); err != nil {
				continue
			}
			if absNum.Valid {
				v := int(absNum.Int64)
				ep.AbsoluteNumber = &v
			}
			if nullEpTitle.Valid {
				ep.Title = nullEpTitle.String
			}
			if nullOverview.Valid {
				ep.Overview = nullOverview.String
			}
			if nullAirDate.Valid {
				ep.AirDate = nullAirDate.String
			}
			if nullFilePath.Valid {
				ep.FilePath = nullFilePath.String
			}

			se.Episodes = append(se.Episodes, ep)
		}
		epRows.Close()

		if se.Episodes == nil {
			se.Episodes = []episodeEntry{}
		}
		seasons = append(seasons, se)
	}

	if seasons == nil {
		seasons = []seasonEntry{}
	}

	writeJSON(w, 200, seasons)
}

func (s *Server) handleSearchEpisode(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	episodeID, err := parseID(r, "episodeID")
	if err != nil {
		writeError(w, 400, "invalid episode_id")
		return
	}

	s.searchForEpisode(w, mediaID, episodeID)
}

func (s *Server) handleSearchAll(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	// Check if this is a movie
	var mediaType string
	s.db.QueryRow(`SELECT type FROM media_items WHERE id = ?`, mediaID).Scan(&mediaType)

	if mediaType == "movie" {
		s.searchForMovie(w, mediaID)
		return
	}

	// Search for all wanted standard episodes
	rows, err := s.db.Query(`SELECT id FROM episodes
		WHERE media_item_id = ? AND status = 'wanted' AND episode_type = 'standard'`, mediaID)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	go func() {
		for _, id := range ids {
			s.runtimeMu.RLock()
			s.searchForEpisodeSilent(mediaID, id)
			s.runtimeMu.RUnlock()
		}
	}()
	count := len(ids)

	writeJSON(w, 200, map[string]interface{}{
		"status":          "searching",
		"episodes_queued": count,
	})
}

func (s *Server) handleDeleteEpisodeFile(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	episodeID, err := parseID(r, "episodeID")
	if err != nil {
		writeError(w, 400, "invalid episode_id")
		return
	}
	var path sql.NullString
	if err := s.db.QueryRow(`SELECT file_path FROM episodes WHERE id=? AND media_item_id=?`, episodeID, mediaID).Scan(&path); err != nil {
		writeError(w, 404, "episode not found")
		return
	}
	if path.Valid && path.String != "" {
		root := s.cfg.Snapshot().MediaRoot
		resolved, err := filepath.EvalSymlinks(path.String)
		if err != nil && !os.IsNotExist(err) {
			writeError(w, 500, "cannot resolve episode file")
			return
		}
		if err == nil {
			actualRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				writeError(w, 500, "cannot resolve media root")
				return
			}
			rel, err := filepath.Rel(actualRoot, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				writeError(w, 400, "episode path is outside media root")
				return
			}
			if err := os.Remove(path.String); err != nil {
				writeError(w, 500, "could not delete episode file")
				return
			}
		}
	}
	if _, err := s.db.Exec(`UPDATE episodes SET file_path=NULL,status='wanted' WHERE id=? AND media_item_id=?`, episodeID, mediaID); err != nil {
		writeError(w, 500, "database error")
		return
	}
	s.db.Exec(`UPDATE media_items SET status='wanted' WHERE id=?`, mediaID)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleResetEpisode(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	episodeID, err := parseID(r, "episodeID")
	if err != nil {
		writeError(w, 400, "invalid episode_id")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT id FROM episodes WHERE id=? AND media_item_id=?`, episodeID, mediaID).Scan(&exists); err != nil {
		writeError(w, 404, "episode not found")
		return
	}
	rows, err := s.db.Query(`SELECT id,download_type,qbt_hash,sabnzbd_nzo_id FROM downloads WHERE episode_id=? AND status NOT IN ('imported','failed','cancelled')`, episodeID)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	type download struct {
		id        int
		kind      string
		hash, nzo sql.NullString
	}
	var downloads []download
	for rows.Next() {
		var d download
		if err := rows.Scan(&d.id, &d.kind, &d.hash, &d.nzo); err != nil {
			rows.Close()
			writeError(w, 500, "database error")
			return
		}
		downloads = append(downloads, d)
	}
	rows.Close()
	for _, d := range downloads {
		if err := s.cancelClientDownload(d.id, d.kind, d.hash, d.nzo); err != nil {
			writeError(w, 502, "Cancellation failed: "+err.Error())
			return
		}
		s.db.Exec(`UPDATE downloads SET status='cancelled' WHERE id=?`, d.id)
	}
	s.db.Exec(`UPDATE episodes SET status=CASE WHEN file_path IS NOT NULL AND file_path!='' THEN 'available' ELSE 'wanted' END WHERE id=?`, episodeID)
	writeJSON(w, 200, map[string]string{"status": "reset"})
}

func (s *Server) searchForEpisode(w http.ResponseWriter, mediaID, episodeID int) {
	releases, err := s.findReleasesForEpisode(mediaID, episodeID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	// Filter blacklisted releases
	blacklist := s.loadBlacklist(mediaID, episodeID)
	releases = indexer.FilterBlacklisted(releases, blacklist)

	best := indexer.BestRelease(releases)
	if best == nil {
		writeJSON(w, 200, map[string]interface{}{
			"status":   "no_results",
			"releases": releases,
		})
		return
	}

	nzoID, err := s.enqueueRelease(mediaID, episodeID, 0, *best)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "grabbed", "release": best, "nzo_id": nzoID})
}

func (s *Server) searchForEpisodeSilent(mediaID, episodeID int) {
	releases, err := s.findReleasesForEpisode(mediaID, episodeID)
	if err != nil {
		return
	}

	// Filter blacklisted releases
	blacklist := s.loadBlacklist(mediaID, episodeID)
	releases = indexer.FilterBlacklisted(releases, blacklist)

	best := indexer.BestRelease(releases)
	if best == nil {
		return
	}

	nzoID, err := s.enqueueRelease(mediaID, episodeID, 0, *best)
	_ = nzoID
	if err != nil {
		slog.Warn("episode grab failed", "error", err)
	}
}

func (s *Server) searchForMovie(w http.ResponseWriter, mediaID int) {
	var title, imdbID string
	var year int
	var profileID int
	s.db.QueryRow(`SELECT title, COALESCE(imdb_id,''), COALESCE(year,0), COALESCE(quality_profile_id,0)
		FROM media_items WHERE id = ?`, mediaID).Scan(&title, &imdbID, &year, &profileID)

	allIndexers := s.loadIndexers()

	// Newznab search
	newznabIdxs := filterIndexersByType(allIndexers, "newznab", "movie")
	releases := s.newznab.SearchMovie(newznabIdxs, imdbID, title, year)
	for i := range releases {
		releases[i].DownloadType = "nzb"
	}

	// Rutracker search
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

	// Filter blacklisted releases
	movieBlacklist := s.loadBlacklist(mediaID, 0)
	releases = indexer.FilterBlacklisted(releases, movieBlacklist)

	for i := range releases {
		indexer.ScoreRelease(&releases[i], &profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		writeJSON(w, 200, map[string]interface{}{
			"status":   "no_results",
			"releases": releases,
		})
		return
	}

	nzoID, err := s.enqueueRelease(mediaID, 0, 0, *best)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "grabbed", "release": best, "nzo_id": nzoID})
}

func (s *Server) findReleasesForEpisode(mediaID, episodeID int) ([]indexer.Release, error) {
	var title string
	var tvdbID, profileID int
	var anime bool
	err := s.db.QueryRow(`SELECT title, COALESCE(tvdb_id,0), anime, COALESCE(quality_profile_id,0)
		FROM media_items WHERE id = ?`, mediaID).Scan(&title, &tvdbID, &anime, &profileID)
	if err != nil {
		return nil, fmt.Errorf("media item not found")
	}

	var seasonNum, epNum int
	var absNum sql.NullInt64
	err = s.db.QueryRow(`SELECT s.number, e.number, e.absolute_number
		FROM episodes e JOIN seasons s ON e.season_id = s.id WHERE e.id = ? AND e.media_item_id = ?`,
		episodeID, mediaID).Scan(&seasonNum, &epNum, &absNum)
	if err != nil {
		return nil, fmt.Errorf("episode does not belong to this series")
	}

	allIndexers := s.loadIndexers()

	// Newznab search
	contentType := "series"
	if anime {
		contentType = "anime"
	}
	newznabIdxs := filterIndexersByType(allIndexers, "newznab", contentType)

	var releases []indexer.Release
	if anime && absNum.Valid {
		titles := []string{title}
		releases = s.newznab.SearchAnimeEpisode(newznabIdxs, titles, int(absNum.Int64))
	} else {
		releases = s.newznab.SearchEpisode(newznabIdxs, tvdbID, seasonNum, epNum, title)
	}
	// Mark NZB releases
	for i := range releases {
		releases[i].DownloadType = "nzb"
	}

	// Rutracker search
	rtIndexers := filterIndexersByType(allIndexers, "rutracker", contentType)
	if len(rtIndexers) > 0 {
		query := fmt.Sprintf("%s S%02dE%02d", title, seasonNum, epNum)
		if anime && absNum.Valid {
			query = fmt.Sprintf("%s %d", title, absNum.Int64)
		}
		rtReleases := s.searchRutracker(rtIndexers, query, contentType)
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

	return releases, nil
}

// filterIndexersByType returns indexers of a specific type that support a content type.
func filterIndexersByType(indexers []indexer.IndexerConfig, idxType, contentType string) []indexer.IndexerConfig {
	var filtered []indexer.IndexerConfig
	for _, idx := range indexers {
		if idx.Type != idxType {
			continue
		}
		if contentType != "" && !containsStr(idx.ContentTypes, contentType) {
			continue
		}
		filtered = append(filtered, idx)
	}
	return filtered
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// searchRutracker searches Rutracker indexers and returns releases.
func (s *Server) searchRutracker(indexers []indexer.IndexerConfig, query, contentType string) []indexer.Release {
	if s.rutracker == nil {
		return nil
	}

	var allReleases []indexer.Release
	forumIDs := rutracker.DefaultForumIDs[contentType]
	if len(forumIDs) == 0 {
		forumIDs = rutracker.DefaultForumIDs["movie"]
	}

	for _, idx := range indexers {
		results, err := s.rutracker.Search(query, forumIDs, idx.Username, idx.Password)
		if err != nil {
			slog.Warn("rutracker search failed", "indexer", idx.Name, "error", err)
			continue
		}

		for _, r := range results {
			parsed := indexer.ParseReleaseName(r.Title)
			allReleases = append(allReleases, indexer.Release{
				Title:        r.Title,
				Size:         r.Size,
				Quality:      parsed.Quality,
				Tags:         parsed.Tags,
				Indexer:      idx.Name,
				DownloadType: "torrent",
				Seeders:      r.Seeders,
				Leechers:     r.Leechers,
				TopicID:      r.TopicID,
			})
		}
	}
	return allReleases
}

func (s *Server) loadIndexers() []indexer.IndexerConfig {
	rows, err := s.db.Query(`SELECT id, name, url, api_key, priority, enabled, COALESCE(type,'newznab'), COALESCE(username,''), COALESCE(password,''), COALESCE(content_types,'["movie","series","anime","music"]') FROM indexers WHERE enabled = TRUE ORDER BY priority DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var configs []indexer.IndexerConfig
	for rows.Next() {
		var c indexer.IndexerConfig
		var contentTypesJSON string
		if err := rows.Scan(&c.ID, &c.Name, &c.URL, &c.APIKey, &c.Priority, &c.Enabled, &c.Type, &c.Username, &c.Password, &contentTypesJSON); err != nil {
			continue
		}
		json.Unmarshal([]byte(contentTypesJSON), &c.ContentTypes)
		if c.Type == "" {
			c.Type = "newznab"
		}
		configs = append(configs, c)
	}
	return configs
}
