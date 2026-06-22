package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"mediaforge/internal/metadata"
)

type addToLibraryRequest struct {
	TMDBID           int    `json:"tmdb_id"`
	AniListID        int    `json:"anilist_id"`
	Type             string `json:"type"` // movie or series
	Anime            bool   `json:"anime"`
	QualityProfileID int    `json:"quality_profile_id"`
	Seasons          []int  `json:"seasons,omitempty"` // which season numbers to monitor
}

func (s *Server) handleAddToLibrary(w http.ResponseWriter, r *http.Request) {
	var req addToLibraryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.AniListID > 0 {
		s.addAnimeToLibrary(w, req)
		return
	}

	if req.TMDBID > 0 {
		s.addTMDBToLibrary(w, req)
		return
	}

	writeError(w, 400, "must provide tmdb_id or anilist_id")
}

func (s *Server) addTMDBToLibrary(w http.ResponseWriter, req addToLibraryRequest) {
	if s.tmdb == nil {
		writeError(w, 500, "TMDB not configured")
		return
	}

	if req.Type == "movie" {
		movie, err := s.tmdb.GetMovie(req.TMDBID)
		if err != nil {
			writeError(w, 500, "TMDB fetch failed: "+err.Error())
			return
		}

		year := 0
		if len(movie.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(movie.ReleaseDate[:4])
		}

		genres := genreNames(movie.Genres)
		genresJSON, _ := json.Marshal(genres)

		result, err := s.db.Exec(`INSERT INTO media_items
			(type, title, year, tmdb_id, imdb_id, overview, poster_url, backdrop_url, genres, rating, rating_source, quality_profile_id, root_path)
			VALUES ('movie', ?, ?, ?, ?, ?, ?, ?, ?, ?, 'tmdb', ?, ?)`,
			movie.Title, year, movie.ID, movie.IMDbID,
			movie.Overview,
			metadata.PosterURL(movie.PosterPath),
			metadata.BackdropURL(movie.BackdropPath),
			string(genresJSON),
			movie.VoteAverage,
			req.QualityProfileID,
			fmt.Sprintf("%s/movies/%s (%d)", s.cfg.MediaRoot, movie.Title, year),
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeError(w, 409, "movie already in library")
				return
			}
			writeError(w, 500, "database error: "+err.Error())
			return
		}

		id, _ := result.LastInsertId()
		s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'added', ?)`,
			id, fmt.Sprintf("Added %s (%d)", movie.Title, year))

		writeJSON(w, 201, map[string]int64{"id": id})

	} else {
		// TV Series
		tv, err := s.tmdb.GetTV(req.TMDBID)
		if err != nil {
			writeError(w, 500, "TMDB fetch failed: "+err.Error())
			return
		}

		year := 0
		if len(tv.FirstAirDate) >= 4 {
			year, _ = strconv.Atoi(tv.FirstAirDate[:4])
		}

		genres := genreNames(tv.Genres)
		genresJSON, _ := json.Marshal(genres)

		anime := req.Anime || s.tmdb.IsAnime(tv)

		result, err := s.db.Exec(`INSERT INTO media_items
			(type, title, year, anime, tmdb_id, imdb_id, tvdb_id, overview, poster_url, backdrop_url, genres, rating, rating_source, quality_profile_id)
			VALUES ('series', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'tmdb', ?)`,
			tv.Name, year, anime, tv.ID,
			tv.ExternalIDs.IMDBID, tv.ExternalIDs.TVDBID,
			tv.Overview,
			metadata.PosterURL(tv.PosterPath),
			metadata.BackdropURL(tv.BackdropPath),
			string(genresJSON),
			tv.VoteAverage,
			req.QualityProfileID,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeError(w, 409, "series already in library")
				return
			}
			writeError(w, 500, "database error: "+err.Error())
			return
		}

		mediaID, _ := result.LastInsertId()

		// Build set of selected seasons (if specified)
		selectedSeasons := make(map[int]bool)
		for _, sn := range req.Seasons {
			selectedSeasons[sn] = true
		}

		// Fetch seasons and episodes
		for _, season := range tv.Seasons {
			if season.SeasonNumber == 0 && !anime {
				continue // skip specials for non-anime unless explicitly requested
			}
			// If user selected specific seasons, skip unselected ones
			if len(selectedSeasons) > 0 && !selectedSeasons[season.SeasonNumber] {
				continue
			}

			s.db.Exec(`INSERT INTO seasons (media_item_id, number, title, overview, poster_url)
				VALUES (?, ?, ?, ?, ?)`,
				mediaID, season.SeasonNumber, season.Name, season.Overview,
				metadata.PosterURL(season.PosterPath))

			var seasonID int64
			s.db.QueryRow(`SELECT id FROM seasons WHERE media_item_id = ? AND number = ?`,
				mediaID, season.SeasonNumber).Scan(&seasonID)

			// Fetch episode details
			seasonDetail, err := s.tmdb.GetSeason(tv.ID, season.SeasonNumber)
			if err != nil {
				continue
			}

			for _, ep := range seasonDetail.Episodes {
				epType := "standard"
				if season.SeasonNumber == 0 {
					epType = "special"
				}

				s.db.Exec(`INSERT INTO episodes
					(season_id, media_item_id, number, episode_type, title, overview, air_date)
					VALUES (?, ?, ?, ?, ?, ?, ?)`,
					seasonID, mediaID, ep.EpisodeNumber, epType,
					ep.Name, ep.Overview, ep.AirDate)
			}
		}

		s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'added', ?)`,
			mediaID, fmt.Sprintf("Added %s (%d)", tv.Name, year))

		writeJSON(w, 201, map[string]int64{"id": mediaID})
	}
}

func (s *Server) addAnimeToLibrary(w http.ResponseWriter, req addToLibraryRequest) {
	if s.anilist == nil {
		writeError(w, 500, "AniList client not configured")
		return
	}

	media, err := s.anilist.GetAnime(req.AniListID)
	if err != nil {
		writeError(w, 500, "AniList fetch failed: "+err.Error())
		return
	}

	title := media.DisplayTitle()

	genres, _ := json.Marshal(media.Genres)
	var tagNames []string
	for _, t := range media.Tags {
		if t.Rank >= 60 {
			tagNames = append(tagNames, t.Name)
		}
	}
	tagsJSON, _ := json.Marshal(tagNames)

	result, err := s.db.Exec(`INSERT INTO media_items
		(type, title, year, anime, anilist_id, overview, poster_url, backdrop_url, genres, tags, rating, rating_source, quality_profile_id)
		VALUES ('series', ?, ?, TRUE, ?, ?, ?, ?, ?, ?, ?, 'anilist', ?)`,
		title, media.StartDate.Year,
		media.ID, media.Description,
		media.CoverImage.ExtraLarge, media.BannerImage,
		string(genres), string(tagsJSON),
		float64(media.AverageScore)/10.0,
		req.QualityProfileID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, 409, "anime already in library")
			return
		}
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	mediaID, _ := result.LastInsertId()

	// Create a single season with episodes based on episode count
	if media.Episodes > 0 {
		s.db.Exec(`INSERT INTO seasons (media_item_id, number, title) VALUES (?, 1, 'Season 1')`, mediaID)
		var seasonID int64
		s.db.QueryRow(`SELECT id FROM seasons WHERE media_item_id = ? AND number = 1`, mediaID).Scan(&seasonID)

		for i := 1; i <= media.Episodes; i++ {
			s.db.Exec(`INSERT INTO episodes (season_id, media_item_id, number, absolute_number, episode_type)
				VALUES (?, ?, ?, ?, 'standard')`,
				seasonID, mediaID, i, i)
		}
	}

	s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'added', ?)`,
		mediaID, fmt.Sprintf("Added %s", title))

	writeJSON(w, 201, map[string]int64{"id": mediaID})
}

func (s *Server) handleListLibrary(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 50)
	offset := (page - 1) * limit

	query := `SELECT id, type, title, year, anime, tmdb_id, anilist_id, poster_url, status, rating, genres, added_at
		FROM media_items WHERE 1=1`
	var args []interface{}

	if mediaType == "anime" {
		query += " AND anime = TRUE"
	} else if mediaType == "movie" {
		query += " AND type = 'movie'"
	} else if mediaType == "series" {
		query += " AND type = 'series' AND anime = FALSE"
	}

	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY added_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type libraryEntry struct {
		ID        int            `json:"id"`
		Type      string         `json:"type"`
		Title     string         `json:"title"`
		Year      sql.NullInt64  `json:"-"`
		YearVal   int            `json:"year"`
		Anime     bool           `json:"anime"`
		TMDBID    sql.NullInt64  `json:"-"`
		TMDBIDVal int            `json:"tmdb_id,omitempty"`
		AniListID sql.NullInt64  `json:"-"`
		AniListVal int           `json:"anilist_id,omitempty"`
		PosterURL sql.NullString `json:"-"`
		PosterVal string         `json:"poster_url"`
		Status    string         `json:"status"`
		Rating    sql.NullFloat64 `json:"-"`
		RatingVal float64        `json:"rating"`
		Genres    sql.NullString `json:"-"`
		GenresVal json.RawMessage `json:"genres"`
		AddedAt   string         `json:"added_at"`
	}

	var items []libraryEntry
	for rows.Next() {
		var e libraryEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Title, &e.Year, &e.Anime, &e.TMDBID, &e.AniListID,
			&e.PosterURL, &e.Status, &e.Rating, &e.Genres, &e.AddedAt); err != nil {
			slog.Warn("library scan failed", "error", err)
			continue
		}
		if e.Year.Valid {
			e.YearVal = int(e.Year.Int64)
		}
		if e.TMDBID.Valid {
			e.TMDBIDVal = int(e.TMDBID.Int64)
		}
		if e.AniListID.Valid {
			e.AniListVal = int(e.AniListID.Int64)
		}
		if e.PosterURL.Valid {
			e.PosterVal = e.PosterURL.String
		}
		if e.Rating.Valid {
			e.RatingVal = e.Rating.Float64
		}
		if e.Genres.Valid {
			e.GenresVal = json.RawMessage(e.Genres.String)
		} else {
			e.GenresVal = json.RawMessage("[]")
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "database error")
		return
	}

	if items == nil {
		items = []libraryEntry{}
	}

	// Count total
	var total int
	countQuery := `SELECT COUNT(*) FROM media_items WHERE 1=1`
	var countArgs []interface{}
	if mediaType == "anime" {
		countQuery += " AND anime = TRUE"
	} else if mediaType == "movie" {
		countQuery += " AND type = 'movie'"
	} else if mediaType == "series" {
		countQuery += " AND type = 'series' AND anime = FALSE"
	}
	if status != "" && status != "all" {
		countQuery += " AND status = ?"
		countArgs = append(countArgs, status)
	}
	s.db.QueryRow(countQuery, countArgs...).Scan(&total)

	writeJSON(w, 200, map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (s *Server) handleGetLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var item struct {
		ID               int     `json:"id"`
		Type             string  `json:"type"`
		Title            string  `json:"title"`
		Year             int     `json:"year"`
		Anime            bool    `json:"anime"`
		TMDBID           int     `json:"tmdb_id"`
		IMDBID           string  `json:"imdb_id"`
		AniListID        int     `json:"anilist_id"`
		TVDBID           int     `json:"tvdb_id"`
		Overview         string  `json:"overview"`
		PosterURL        string  `json:"poster_url"`
		BackdropURL      string  `json:"backdrop_url"`
		Genres           string  `json:"genres"`
		Tags             string  `json:"tags"`
		Rating           float64 `json:"rating"`
		Status           string  `json:"status"`
		QualityProfileID int     `json:"quality_profile_id"`
		RootPath         string  `json:"root_path"`
		HasFiles         bool    `json:"has_files"`
		AddedAt          string  `json:"added_at"`
	}

	var (
		nullYear, nullTMDB, nullAniList, nullTVDB, nullProfileID sql.NullInt64
		nullIMDB, nullOverview, nullPoster, nullBackdrop         sql.NullString
		nullGenres, nullTags, nullRootPath                       sql.NullString
		nullRating                                               sql.NullFloat64
	)

	err = s.db.QueryRow(`SELECT id, type, title, year, anime, tmdb_id, imdb_id, anilist_id, tvdb_id,
		overview, poster_url, backdrop_url, genres, tags, rating, status, quality_profile_id, root_path, added_at
		FROM media_items WHERE id = ?`, id).Scan(
		&item.ID, &item.Type, &item.Title, &nullYear, &item.Anime,
		&nullTMDB, &nullIMDB, &nullAniList, &nullTVDB,
		&nullOverview, &nullPoster, &nullBackdrop,
		&nullGenres, &nullTags, &nullRating,
		&item.Status, &nullProfileID, &nullRootPath, &item.AddedAt)
	if err != nil {
		writeError(w, 404, "not found")
		return
	}

	if nullYear.Valid { item.Year = int(nullYear.Int64) }
	if nullTMDB.Valid { item.TMDBID = int(nullTMDB.Int64) }
	if nullIMDB.Valid { item.IMDBID = nullIMDB.String }
	if nullAniList.Valid { item.AniListID = int(nullAniList.Int64) }
	if nullTVDB.Valid { item.TVDBID = int(nullTVDB.Int64) }
	if nullOverview.Valid { item.Overview = nullOverview.String }
	if nullPoster.Valid { item.PosterURL = nullPoster.String }
	if nullBackdrop.Valid { item.BackdropURL = nullBackdrop.String }
	if nullGenres.Valid { item.Genres = nullGenres.String }
	if nullTags.Valid { item.Tags = nullTags.String }
	if nullRating.Valid { item.Rating = nullRating.Float64 }
	if nullProfileID.Valid { item.QualityProfileID = int(nullProfileID.Int64) }
	if nullRootPath.Valid { item.RootPath = nullRootPath.String }

	// Check if files actually exist on disk
	if item.Type == "movie" && item.RootPath != "" {
		if info, err := os.Stat(item.RootPath); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(item.RootPath)
			item.HasFiles = len(entries) > 0
		}
	} else if item.Type == "series" {
		var fileCount int
		s.db.QueryRow(`SELECT COUNT(*) FROM episodes WHERE media_item_id = ? AND file_path IS NOT NULL AND file_path != ''`, id).Scan(&fileCount)
		item.HasFiles = fileCount > 0
	}

	writeJSON(w, 200, item)
}

func (s *Server) handleUpdateLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var req struct {
		QualityProfileID *int  `json:"quality_profile_id"`
		Anime            *bool `json:"anime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.QualityProfileID != nil {
		s.db.Exec(`UPDATE media_items SET quality_profile_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			*req.QualityProfileID, id)
	}
	if req.Anime != nil {
		s.db.Exec(`UPDATE media_items SET anime = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			*req.Anime, id)
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	deleteFiles := r.URL.Query().Get("delete_files") == "true"

	// 1. Cancel any active SABnzbd downloads for this media item
	dlRows, err := s.db.Query(`SELECT id, sabnzbd_nzo_id FROM downloads
		WHERE media_item_id = ? AND status NOT IN ('imported', 'failed')`, id)
	if err == nil {
		for dlRows.Next() {
			var dlID int
			var nzoID sql.NullString
			if dlRows.Scan(&dlID, &nzoID) == nil {
				if nzoID.Valid && nzoID.String != "" {
					s.grabber.DeleteFromQueue(nzoID.String)
				}
			}
		}
		dlRows.Close()
	}

	// 2. Delete episode files from disk if requested
	if deleteFiles {
		epRows, _ := s.db.Query(`SELECT file_path FROM episodes WHERE media_item_id = ? AND file_path IS NOT NULL`, id)
		if epRows != nil {
			for epRows.Next() {
				var fp string
				if epRows.Scan(&fp) == nil && fp != "" {
					os.Remove(fp)
				}
			}
			epRows.Close()
		}

		var rootPath sql.NullString
		s.db.QueryRow(`SELECT root_path FROM media_items WHERE id = ?`, id).Scan(&rootPath)
		if rootPath.Valid && rootPath.String != "" {
			os.RemoveAll(rootPath.String)
		}
	}

	// 3. Clean up records without CASCADE (downloads, activity_log)
	s.db.Exec(`DELETE FROM downloads WHERE media_item_id = ?`, id)
	s.db.Exec(`DELETE FROM activity_log WHERE media_item_id = ?`, id)

	// 4. Delete the media item (seasons/episodes cascade automatically)
	s.db.Exec(`DELETE FROM media_items WHERE id = ?`, id)

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleAddSeasons(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var req struct {
		Seasons []int `json:"seasons"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	if len(req.Seasons) == 0 {
		writeError(w, 400, "must provide at least one season number")
		return
	}

	// Look up media item
	var tmdbID int
	var mediaType string
	var anime bool
	err = s.db.QueryRow(`SELECT tmdb_id, type, anime FROM media_items WHERE id = ?`, mediaID).Scan(&tmdbID, &mediaType, &anime)
	if err != nil {
		writeError(w, 404, "media item not found")
		return
	}
	if mediaType != "series" || tmdbID == 0 {
		writeError(w, 400, "can only add seasons to TMDB-backed series")
		return
	}

	// Get existing season numbers
	existing := map[int]bool{}
	rows, err := s.db.Query(`SELECT number FROM seasons WHERE media_item_id = ?`, mediaID)
	if err == nil {
		for rows.Next() {
			var n int
			rows.Scan(&n)
			existing[n] = true
		}
		rows.Close()
	}

	// Filter to new seasons only
	var newSeasons []int
	for _, sn := range req.Seasons {
		if !existing[sn] {
			newSeasons = append(newSeasons, sn)
		}
	}
	if len(newSeasons) == 0 {
		writeError(w, 409, "all requested seasons already exist")
		return
	}

	// Fetch TMDB data
	if s.tmdb == nil {
		writeError(w, 500, "TMDB not configured")
		return
	}
	tv, err := s.tmdb.GetTV(tmdbID)
	if err != nil {
		writeError(w, 500, "TMDB fetch failed: "+err.Error())
		return
	}

	tmdbSeasonMap := map[int]metadata.TMDBSeason{}
	for _, s := range tv.Seasons {
		tmdbSeasonMap[s.SeasonNumber] = s
	}

	added := 0
	for _, sn := range newSeasons {
		season, ok := tmdbSeasonMap[sn]
		if !ok {
			continue
		}

		s.db.Exec(`INSERT INTO seasons (media_item_id, number, title, overview, poster_url)
			VALUES (?, ?, ?, ?, ?)`,
			mediaID, season.SeasonNumber, season.Name, season.Overview,
			metadata.PosterURL(season.PosterPath))

		var seasonID int64
		s.db.QueryRow(`SELECT id FROM seasons WHERE media_item_id = ? AND number = ?`,
			mediaID, season.SeasonNumber).Scan(&seasonID)

		seasonDetail, err := s.tmdb.GetSeason(tmdbID, season.SeasonNumber)
		if err != nil {
			continue
		}

		for _, ep := range seasonDetail.Episodes {
			epType := "standard"
			if season.SeasonNumber == 0 {
				epType = "special"
			}
			s.db.Exec(`INSERT INTO episodes
				(season_id, media_item_id, number, episode_type, title, overview, air_date)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				seasonID, mediaID, ep.EpisodeNumber, epType,
				ep.Name, ep.Overview, ep.AirDate)
		}
		added++
	}

	s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'seasons_added', ?)`,
		mediaID, fmt.Sprintf("Added %d new season(s)", added))

	writeJSON(w, 200, map[string]int{"seasons_added": added})
}

func genreNames(genres []metadata.TMDBGenre) []string {
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = g.Name
	}
	return names
}
