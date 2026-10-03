package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

	if req.AniListID <= 0 && req.Type != "movie" && req.Type != "series" {
		writeError(w, 400, "type must be movie or series")
		return
	}
	for _, season := range req.Seasons {
		if season < 0 {
			writeError(w, 400, "season numbers cannot be negative")
			return
		}
	}
	if req.QualityProfileID != 0 {
		if _, err := s.resolveProfile(req.QualityProfileID, "video"); err != nil {
			writeError(w, 400, err.Error())
			return
		}
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
		anime := req.Anime || s.tmdb.IsAnimeMovie(movie)
		req.QualityProfileID, err = s.resolveMediaProfile(req.QualityProfileID, "movie", anime)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}

		year := 0
		if len(movie.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(movie.ReleaseDate[:4])
		}

		genres := genreNames(movie.Genres)
		genresJSON, _ := json.Marshal(genres)

		result, err := s.db.Exec(`INSERT INTO media_items
			(type, title, year, anime, tmdb_id, imdb_id, overview, poster_url, backdrop_url, genres, rating, rating_source, quality_profile_id)
			VALUES ('movie', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'tmdb', ?)`,
			movie.Title, year, anime, movie.ID, movie.IMDbID,
			movie.Overview,
			metadata.PosterURL(movie.PosterPath),
			metadata.BackdropURL(movie.BackdropPath),
			string(genresJSON),
			movie.VoteAverage,
			req.QualityProfileID,
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
		req.QualityProfileID, err = s.resolveMediaProfile(req.QualityProfileID, "series", anime)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}

		selectedSeasons := make(map[int]bool)
		for _, sn := range req.Seasons {
			selectedSeasons[sn] = true
		}
		details := make(map[int]*metadata.TMDBSeasonDetail)
		for _, season := range tv.Seasons {
			if len(selectedSeasons) > 0 && !selectedSeasons[season.SeasonNumber] {
				continue
			}
			if season.SeasonNumber == 0 && !anime && !selectedSeasons[0] {
				continue
			}
			detail, err := s.tmdb.GetSeason(tv.ID, season.SeasonNumber)
			if err != nil {
				writeError(w, 502, "failed to fetch season metadata: "+err.Error())
				return
			}
			details[season.SeasonNumber] = detail
		}
		for sn := range selectedSeasons {
			if details[sn] == nil {
				writeError(w, 400, "requested season does not exist")
				return
			}
		}
		tx, err := s.db.Begin()
		if err != nil {
			writeError(w, 500, "database error")
			return
		}
		defer tx.Rollback()

		result, err := tx.Exec(`INSERT INTO media_items
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

		for _, season := range tv.Seasons {
			detail := details[season.SeasonNumber]
			if detail == nil {
				continue
			}
			if err := insertSeason(tx, mediaID, season, detail); err != nil {
				writeError(w, 500, "failed to save season: "+err.Error())
				return
			}
		}

		tx.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'added', ?)`,
			mediaID, fmt.Sprintf("Added %s (%d)", tv.Name, year))

		if err := tx.Commit(); err != nil {
			writeError(w, 500, "failed to save series")
			return
		}
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
	mediaType := "series"
	if strings.EqualFold(media.Format, "MOVIE") {
		mediaType = "movie"
	}
	req.QualityProfileID, err = s.resolveMediaProfile(req.QualityProfileID, mediaType, true)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	genres, _ := json.Marshal(media.Genres)
	var tagNames []string
	for _, t := range media.Tags {
		if t.Rank >= 60 {
			tagNames = append(tagNames, t.Name)
		}
	}
	tagsJSON, _ := json.Marshal(tagNames)

	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO media_items
		(type, title, year, anime, anilist_id, overview, poster_url, backdrop_url, genres, tags, rating, rating_source, quality_profile_id)
		VALUES (?, ?, ?, TRUE, ?, ?, ?, ?, ?, ?, ?, 'anilist', ?)`,
		mediaType, title, media.StartDate.Year,
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

	if mediaType == "series" && media.Episodes > 0 {
		result, err := tx.Exec(`INSERT INTO seasons (media_item_id,number,title) VALUES (?,1,'Season 1')`, mediaID)
		if err != nil {
			writeError(w, 500, "failed to save season")
			return
		}
		seasonID, _ := result.LastInsertId()
		for i := 1; i <= media.Episodes; i++ {
			if _, err := tx.Exec(`INSERT INTO episodes (season_id,media_item_id,number,absolute_number,episode_type) VALUES (?,?,?,?,'standard')`, seasonID, mediaID, i, i); err != nil {
				writeError(w, 500, "failed to save episodes")
				return
			}
		}
	}

	tx.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'added', ?)`,
		mediaID, fmt.Sprintf("Added %s", title))

	if err := tx.Commit(); err != nil {
		writeError(w, 500, "failed to save anime")
		return
	}
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
		ID         int             `json:"id"`
		Type       string          `json:"type"`
		Title      string          `json:"title"`
		Year       sql.NullInt64   `json:"-"`
		YearVal    int             `json:"year"`
		Anime      bool            `json:"anime"`
		TMDBID     sql.NullInt64   `json:"-"`
		TMDBIDVal  int             `json:"tmdb_id,omitempty"`
		AniListID  sql.NullInt64   `json:"-"`
		AniListVal int             `json:"anilist_id,omitempty"`
		PosterURL  sql.NullString  `json:"-"`
		PosterVal  string          `json:"poster_url"`
		Status     string          `json:"status"`
		Rating     sql.NullFloat64 `json:"-"`
		RatingVal  float64         `json:"rating"`
		Genres     sql.NullString  `json:"-"`
		GenresVal  json.RawMessage `json:"genres"`
		AddedAt    string          `json:"added_at"`
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

	if nullYear.Valid {
		item.Year = int(nullYear.Int64)
	}
	if nullTMDB.Valid {
		item.TMDBID = int(nullTMDB.Int64)
	}
	if nullIMDB.Valid {
		item.IMDBID = nullIMDB.String
	}
	if nullAniList.Valid {
		item.AniListID = int(nullAniList.Int64)
	}
	if nullTVDB.Valid {
		item.TVDBID = int(nullTVDB.Int64)
	}
	if nullOverview.Valid {
		item.Overview = nullOverview.String
	}
	if nullPoster.Valid {
		item.PosterURL = nullPoster.String
	}
	if nullBackdrop.Valid {
		item.BackdropURL = nullBackdrop.String
	}
	if nullGenres.Valid {
		item.Genres = nullGenres.String
	}
	if nullTags.Valid {
		item.Tags = nullTags.String
	}
	if nullRating.Valid {
		item.Rating = nullRating.Float64
	}
	if nullProfileID.Valid {
		item.QualityProfileID = int(nullProfileID.Int64)
	}
	if nullRootPath.Valid {
		item.RootPath = nullRootPath.String
	}

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
		profileID, err := s.resolveProfile(*req.QualityProfileID, "video")
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		req.QualityProfileID = &profileID
	}
	result, err := s.db.Exec(`UPDATE media_items SET quality_profile_id = COALESCE(?,quality_profile_id), anime = COALESCE(?,anime), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, req.QualityProfileID, req.Anime, id)
	if err != nil {
		writeError(w, 500, "failed to update library item")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, 404, "library item not found")
		return
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	var root sql.NullString
	if err := s.db.QueryRow(`SELECT root_path FROM media_items WHERE id = ?`, id).Scan(&root); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "library item not found")
		} else {
			writeError(w, 500, "database error")
		}
		return
	}
	var paths []string
	if r.URL.Query().Get("delete_files") == "true" {
		rows, err := s.db.Query(`SELECT file_path FROM episodes WHERE media_item_id = ? AND file_path IS NOT NULL AND file_path != ''`, id)
		if err != nil {
			writeError(w, 500, "failed to list library files")
			return
		}
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				rows.Close()
				writeError(w, 500, "failed to read library files")
				return
			}
			paths = append(paths, path)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			writeError(w, 500, "failed to read library files")
			return
		}
		if root.Valid && root.String != "" {
			paths = append(paths, root.String)
		}
		for _, path := range paths {
			if err := validateLibraryDeletePath(s.cfg.MediaRoot, path); err != nil {
				writeError(w, 400, err.Error())
				return
			}
		}
	}
	// Cancel remote transfers first. A failure must leave library records intact.
	rows, err := s.db.Query(`SELECT id,download_type,COALESCE(sabnzbd_nzo_id,''),COALESCE(qbt_hash,'') FROM downloads WHERE media_item_id = ? AND status NOT IN ('imported','failed','cancelled') ORDER BY id`, id)
	if err != nil {
		writeError(w, 500, "failed to list downloads")
		return
	}
	type transfer struct {
		id              int
		kind, nzb, hash string
	}
	var transfers []transfer
	for rows.Next() {
		var t transfer
		if err := rows.Scan(&t.id, &t.kind, &t.nzb, &t.hash); err != nil {
			rows.Close()
			writeError(w, 500, "failed to read downloads")
			return
		}
		transfers = append(transfers, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		writeError(w, 500, "failed to read downloads")
		return
	}
	for _, t := range transfers {
		if err := s.cancelTrackedDownload(t.id, t.kind, sql.NullString{String: t.hash, Valid: t.hash != ""}, sql.NullString{String: t.nzb, Valid: t.nzb != ""}); err != nil {
			writeError(w, 502, "failed to cancel active download: "+err.Error())
			return
		}
	}

	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			writeError(w, 500, "failed to remove library files")
			return
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM downloads WHERE media_item_id = ?`, `DELETE FROM activity_log WHERE media_item_id = ?`, `DELETE FROM media_items WHERE id = ?`} {
		if _, err := tx.Exec(query, id); err != nil {
			writeError(w, 500, "failed to remove library records")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "failed to remove library records")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// Reject broad, escaped and symlinked paths before deleting user files.
func validateLibraryDeletePath(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	within := func(base, path string) bool {
		rel, err := filepath.Rel(base, path)
		return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(strings.Split(rel, string(filepath.Separator))) >= 2
	}
	if !within(rootAbs, targetAbs) {
		return fmt.Errorf("refusing to delete a path outside an individual library item")
	}
	if _, err := os.Lstat(targetAbs); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return err
	}
	realTarget, err := filepath.EvalSymlinks(targetAbs)
	if err != nil {
		return err
	}
	if !within(realRoot, realTarget) {
		return fmt.Errorf("refusing to delete a library path linked outside its media root")
	}
	return nil
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
	err = s.db.QueryRow(`SELECT COALESCE(tmdb_id,0), type, anime FROM media_items WHERE id = ?`, mediaID).Scan(&tmdbID, &mediaType, &anime)
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
		if sn < 0 {
			writeError(w, 400, "season numbers cannot be negative")
			return
		}
		if !existing[sn] {
			newSeasons = append(newSeasons, sn)
			existing[sn] = true
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

	details := make(map[int]*metadata.TMDBSeasonDetail)
	for _, sn := range newSeasons {
		if _, ok := tmdbSeasonMap[sn]; !ok {
			writeError(w, 400, "requested season does not exist")
			return
		}
		detail, err := s.tmdb.GetSeason(tmdbID, sn)
		if err != nil {
			writeError(w, 502, "failed to fetch season metadata: "+err.Error())
			return
		}
		details[sn] = detail
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer tx.Rollback()
	for _, sn := range newSeasons {
		if err := insertSeason(tx, int64(mediaID), tmdbSeasonMap[sn], details[sn]); err != nil {
			writeError(w, 409, "season could not be added: "+err.Error())
			return
		}
	}
	if _, err := tx.Exec(`UPDATE media_items SET status = 'wanted', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'available'`, mediaID); err != nil {
		writeError(w, 500, "failed to update series")
		return
	}
	if _, err := tx.Exec(`INSERT INTO activity_log (media_item_id,action,details) VALUES (?,'seasons_added',?)`, mediaID, fmt.Sprintf("Added %d new season(s)", len(newSeasons))); err != nil {
		writeError(w, 500, "failed to save activity")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "failed to save seasons")
		return
	}
	writeJSON(w, 200, map[string]int{"seasons_added": len(newSeasons)})

}

func genreNames(genres []metadata.TMDBGenre) []string {
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = g.Name
	}
	return names
}

func insertSeason(tx *sql.Tx, mediaID int64, season metadata.TMDBSeason, detail *metadata.TMDBSeasonDetail) error {
	result, err := tx.Exec(`INSERT INTO seasons (media_item_id,number,title,overview,poster_url) VALUES (?,?,?,?,?)`, mediaID, season.SeasonNumber, season.Name, season.Overview, metadata.PosterURL(season.PosterPath))
	if err != nil {
		return err
	}
	seasonID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	for _, ep := range detail.Episodes {
		kind := "standard"
		if season.SeasonNumber == 0 {
			kind = "special"
		}
		if _, err := tx.Exec(`INSERT INTO episodes (season_id,media_item_id,number,episode_type,title,overview,air_date) VALUES (?,?,?,?,?,?,?)`, seasonID, mediaID, ep.EpisodeNumber, kind, ep.Name, ep.Overview, ep.AirDate); err != nil {
			return err
		}
	}
	return nil
}
