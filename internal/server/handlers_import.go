package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"mediaforge/internal/indexer"
	"mediaforge/internal/metadata"
	"mediaforge/internal/postprocess"
)

// -- Scan: detect importable content in a directory --

type scanRequest struct {
	Path string `json:"path"`
	Type string `json:"type"` // movie, series, anime, music
}

type scanResultItem struct {
	SourcePath string      `json:"source_path"`
	Files      []string    `json:"files"`
	Title      string      `json:"title"`
	Year       int         `json:"year,omitempty"`
	Season     int         `json:"season,omitempty"`
	Episode    int         `json:"episode,omitempty"`
	Artist     string      `json:"artist,omitempty"`
	Album      string      `json:"album,omitempty"`
	Match      interface{} `json:"match,omitempty"`
	InLibrary  bool        `json:"in_library"`
	LibraryID  int         `json:"library_id,omitempty"`
}

func (s *Server) handleImportScan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Path == "" {
		writeError(w, 400, "path required")
		return
	}

	info, err := os.Stat(req.Path)
	if err != nil {
		writeError(w, 400, "path not accessible: "+err.Error())
		return
	}
	if !info.IsDir() {
		writeError(w, 400, "path must be a directory")
		return
	}

	var items []scanResultItem
	switch req.Type {
	case "movie":
		items = s.scanMovies(req.Path)
	case "series":
		items = s.scanSeries(req.Path, false)
	case "anime":
		items = s.scanSeries(req.Path, true)
	case "music":
		items = s.scanMusic(req.Path)
	default:
		writeError(w, 400, "type must be one of: movie, series, anime, music")
		return
	}

	if items == nil {
		items = []scanResultItem{}
	}

	writeJSON(w, 200, items)
}

// scanMovies scans a directory for movie files.
// Expects either: dir/Movie (Year)/movie.mkv or dir/movie.mkv
func (s *Server) scanMovies(dir string) []scanResultItem {
	var items []scanResultItem
	titleYearRe := regexp.MustCompile(`^(.+?)\s*[\(\[]?(\d{4})[\)\]]?`)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		entryPath := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			// Look for video files inside
			videoFiles := findVideoFilesFlat(entryPath)
			if len(videoFiles) == 0 {
				continue
			}

			title, year := parseMovieDirName(entry.Name(), titleYearRe)
			item := scanResultItem{
				SourcePath: entryPath,
				Files:      videoFiles,
				Title:      title,
				Year:       year,
			}

			// Try to match via TMDB
			if s.tmdb != nil && title != "" {
				item.Match = s.searchMovieMatch(title, year)
			}

			// Check library
			s.checkMovieInLibrary(&item)

			items = append(items, item)
		} else if postprocess.IsVideoFile(entry.Name()) {
			title, year := parseMovieDirName(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), titleYearRe)
			item := scanResultItem{
				SourcePath: entryPath,
				Files:      []string{entryPath},
				Title:      title,
				Year:       year,
			}
			if s.tmdb != nil && title != "" {
				item.Match = s.searchMovieMatch(title, year)
			}
			s.checkMovieInLibrary(&item)
			items = append(items, item)
		}
	}
	return items
}

func parseMovieDirName(name string, re *regexp.Regexp) (string, int) {
	match := re.FindStringSubmatch(name)
	if match != nil {
		title := strings.TrimSpace(match[1])
		title = strings.TrimRight(title, " -._")
		year, _ := strconv.Atoi(match[2])
		return title, year
	}
	return strings.TrimSpace(name), 0
}

type movieMatch struct {
	TMDBID    int     `json:"tmdb_id"`
	Title     string  `json:"title"`
	Year      string  `json:"year"`
	PosterURL string  `json:"poster_url"`
	Overview  string  `json:"overview"`
	Rating    float64 `json:"rating"`
}

func (s *Server) searchMovieMatch(title string, year int) *movieMatch {
	query := title
	if year > 0 {
		query = fmt.Sprintf("%s %d", title, year)
	}
	result, err := s.tmdb.SearchMovies(query)
	if err != nil || len(result.Results) == 0 {
		return nil
	}

	best := result.Results[0]
	return &movieMatch{
		TMDBID:    best.ID,
		Title:     best.Title,
		Year:      best.ReleaseDate,
		PosterURL: metadata.PosterURL(best.PosterPath),
		Overview:  best.Overview,
		Rating:    best.VoteAverage,
	}
}

func (s *Server) checkMovieInLibrary(item *scanResultItem) {
	if item.Title == "" {
		return
	}
	var id int
	query := `SELECT id FROM media_items WHERE type = 'movie' AND title LIKE ?`
	args := []interface{}{"%" + item.Title + "%"}
	if item.Year > 0 {
		query += " AND year = ?"
		args = append(args, item.Year)
	}
	if s.db.QueryRow(query, args...).Scan(&id) == nil {
		item.InLibrary = true
		item.LibraryID = id
	}
}

// scanSeries scans a directory for TV series/anime content.
// Expects: dir/SeriesName/Season XX/episodes or dir/SeriesName/episodes
func (s *Server) scanSeries(dir string, anime bool) []scanResultItem {
	var items []scanResultItem
	epPattern := regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,3})`)
	titleYearRe := regexp.MustCompile(`^(.+?)\s*[\(\[]?(\d{4})[\)\]]?$`)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		seriesPath := filepath.Join(dir, entry.Name())

		// Collect all video files recursively
		var videoFiles []string
		filepath.Walk(seriesPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if postprocess.IsVideoFile(info.Name()) {
				videoFiles = append(videoFiles, path)
			}
			return nil
		})

		if len(videoFiles) == 0 {
			continue
		}

		// Detect episode count
		episodeCount := 0
		for _, f := range videoFiles {
			if epPattern.MatchString(filepath.Base(f)) {
				episodeCount++
			}
		}

		title, year := parseMovieDirName(entry.Name(), titleYearRe)

		item := scanResultItem{
			SourcePath: seriesPath,
			Files:      videoFiles,
			Title:      title,
			Year:       year,
			Episode:    episodeCount,
		}

		// Try TMDB match
		if s.tmdb != nil && title != "" {
			item.Match = s.searchSeriesMatch(title, anime)
		}

		// Check library
		var id int
		if s.db.QueryRow(`SELECT id FROM media_items WHERE type = 'series' AND anime = ? AND title LIKE ?`,
			anime, "%"+title+"%").Scan(&id) == nil {
			item.InLibrary = true
			item.LibraryID = id
		}

		items = append(items, item)
	}
	return items
}

type seriesMatch struct {
	TMDBID    int     `json:"tmdb_id"`
	Title     string  `json:"title"`
	Year      string  `json:"year"`
	PosterURL string  `json:"poster_url"`
	Overview  string  `json:"overview"`
	Rating    float64 `json:"rating"`
	Seasons   int     `json:"seasons"`
	IsAnime   bool    `json:"is_anime"`
}

func (s *Server) searchSeriesMatch(title string, anime bool) *seriesMatch {
	result, err := s.tmdb.SearchTV(title)
	if err != nil || len(result.Results) == 0 {
		return nil
	}

	best := result.Results[0]
	return &seriesMatch{
		TMDBID:    best.ID,
		Title:     best.Name,
		Year:      best.FirstAirDate,
		PosterURL: metadata.PosterURL(best.PosterPath),
		Overview:  best.Overview,
		Rating:    best.VoteAverage,
		IsAnime:   anime,
	}
}

// scanMusic scans for music albums.
// Expects: dir/Artist/Album/tracks or dir/Album/tracks
func (s *Server) scanMusic(dir string) []scanResultItem {
	var items []scanResultItem

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		entryPath := filepath.Join(dir, entry.Name())

		// Check if this directory directly contains audio files (it's an album dir)
		audioFiles := findAudioFilesFlat(entryPath)
		if len(audioFiles) > 0 {
			// Direct album directory
			item := scanResultItem{
				SourcePath: entryPath,
				Files:      audioFiles,
				Album:      entry.Name(),
			}
			if s.musicbrainz != nil {
				item.Match = s.searchMusicMatch("", entry.Name())
			}
			items = append(items, item)
			continue
		}

		// Check subdirectories (Artist/Album structure)
		subEntries, err := os.ReadDir(entryPath)
		if err != nil {
			continue
		}

		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}

			albumPath := filepath.Join(entryPath, sub.Name())
			albumAudioFiles := findAudioFilesFlat(albumPath)
			// Also check one more level deep for disc directories
			if len(albumAudioFiles) == 0 {
				filepath.Walk(albumPath, func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					if postprocess.IsAudioFile(info.Name()) {
						albumAudioFiles = append(albumAudioFiles, path)
					}
					return nil
				})
			}

			if len(albumAudioFiles) == 0 {
				continue
			}

			item := scanResultItem{
				SourcePath: albumPath,
				Files:      albumAudioFiles,
				Artist:     entry.Name(),
				Album:      sub.Name(),
			}

			if s.musicbrainz != nil {
				item.Match = s.searchMusicMatch(entry.Name(), sub.Name())
			}

			// Check library
			var id int
			if s.db.QueryRow(`SELECT al.id FROM albums al JOIN artists a ON al.artist_id = a.id
				WHERE a.name LIKE ? AND al.title LIKE ?`,
				"%"+entry.Name()+"%", "%"+sub.Name()+"%").Scan(&id) == nil {
				item.InLibrary = true
				item.LibraryID = id
			}

			items = append(items, item)
		}
	}
	return items
}

type musicMatch struct {
	ReleaseGroupID string `json:"release_group_id"`
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	ArtistID       string `json:"artist_id"`
	Year           string `json:"year"`
	CoverURL       string `json:"cover_url"`
}

func (s *Server) searchMusicMatch(artist, album string) *musicMatch {
	query := album
	if artist != "" {
		query = artist + " " + album
	}

	result, err := s.musicbrainz.SearchReleaseGroups(query)
	if err != nil || len(result.ReleaseGroups) == 0 {
		return nil
	}

	rg := result.ReleaseGroups[0]
	m := &musicMatch{
		ReleaseGroupID: rg.ID,
		Title:          rg.Title,
		Year:           rg.FirstRelease,
		CoverURL:       fmt.Sprintf("https://coverartarchive.org/release-group/%s/front-250", rg.ID),
	}
	if len(rg.ArtistCredit) > 0 {
		m.Artist = rg.ArtistCredit[0].Artist.Name
		m.ArtistID = rg.ArtistCredit[0].Artist.ID
	}
	return m
}

// -- Execute Import --

type importItem struct {
	SourcePath       string `json:"source_path"`
	Type             string `json:"type"` // movie, series, anime, music
	TMDBID           int    `json:"tmdb_id,omitempty"`
	ReleaseGroupID   string `json:"release_group_id,omitempty"`
	ArtistMBID       string `json:"artist_mbid,omitempty"`
	ArtistName       string `json:"artist_name,omitempty"`
	AlbumTitle       string `json:"album_title,omitempty"`
	QualityProfileID int    `json:"quality_profile_id"`
}

type importResult struct {
	SourcePath string `json:"source_path"`
	Status     string `json:"status"` // imported, error, skipped
	Message    string `json:"message,omitempty"`
	LibraryID  int    `json:"library_id,omitempty"`
}

func (s *Server) handleImportExecute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []importItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if len(req.Items) == 0 {
		writeError(w, 400, "no items to import")
		return
	}

	var results []importResult
	for _, item := range req.Items {
		var res importResult
		res.SourcePath = item.SourcePath

		profileType := "video"
		if item.Type == "music" {
			profileType = "music"
		}
		profileID, err := s.resolveProfile(item.QualityProfileID, profileType)
		if err != nil {
			results = append(results, importResult{SourcePath: item.SourcePath, Status: "error", Message: err.Error()})
			continue
		}
		item.QualityProfileID = profileID
		switch item.Type {
		case "movie":
			res = s.importMovie(item)
		case "series":
			res = s.importSeries(item, false)
		case "anime":
			res = s.importSeries(item, true)
		case "music":
			res = s.importMusic(item)
		default:
			res.Status = "error"
			res.Message = "unknown type: " + item.Type
		}

		results = append(results, res)
	}

	writeJSON(w, 200, results)
}

func (s *Server) importMovie(item importItem) importResult {
	res := importResult{SourcePath: item.SourcePath}

	if item.TMDBID == 0 {
		res.Status = "error"
		res.Message = "tmdb_id required"
		return res
	}

	if s.tmdb == nil {
		res.Status = "error"
		res.Message = "TMDB not configured"
		return res
	}

	// Fetch metadata
	movie, err := s.tmdb.GetMovie(item.TMDBID)
	if err != nil {
		res.Status = "error"
		res.Message = "TMDB fetch failed: " + err.Error()
		return res
	}

	year := 0
	if len(movie.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(movie.ReleaseDate[:4])
	}

	// Check already in library
	var existingID int
	if s.db.QueryRow(`SELECT id FROM media_items WHERE tmdb_id = ? AND type = 'movie'`, item.TMDBID).Scan(&existingID) == nil {
		res.Status = "skipped"
		res.Message = "already in library"
		res.LibraryID = existingID
		return res
	}

	// Find video files
	videoFiles := findAllVideoFiles(item.SourcePath)
	if len(videoFiles) == 0 {
		res.Status = "error"
		res.Message = "no video files found"
		return res
	}

	// Pick largest
	srcFile := postprocess.LargestFile(videoFiles)
	ext := filepath.Ext(srcFile)
	destPath := postprocess.MoviePath(s.cfg.MediaRoot, movie.Title, year, ext)

	// Create library entry
	genres := genreNames(movie.Genres)
	genresJSON, _ := json.Marshal(genres)

	tx, err := s.db.Begin()
	if err != nil {
		res.Status = "error"
		res.Message = err.Error()
		return res
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO media_items
		(type, title, year, anime, tmdb_id, imdb_id, overview, poster_url, backdrop_url, genres, rating, rating_source, quality_profile_id, root_path, status)
		VALUES ('movie', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'tmdb', ?, ?, 'available')`,
		movie.Title, year, s.tmdb.IsAnimeMovie(movie), movie.ID, movie.IMDbID,
		movie.Overview,
		metadata.PosterURL(movie.PosterPath),
		metadata.BackdropURL(movie.BackdropPath),
		string(genresJSON),
		movie.VoteAverage,
		item.QualityProfileID,
		filepath.Dir(destPath),
	)
	if err != nil {
		res.Status = "error"
		res.Message = "database error: " + err.Error()
		return res
	}

	if err := validateImportDestination(s.cfg.MediaRoot, destPath); err != nil {
		res.Status = "error"
		res.Message = err.Error()
		return res
	}
	if err := moveFileForImport(srcFile, destPath); err != nil {
		res.Status = "error"
		res.Message = "move failed: " + err.Error()
		return res
	}
	id, _ := result.LastInsertId()
	tx.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'imported', ?)`,
		id, fmt.Sprintf("Imported %s (%d)", movie.Title, year))

	if err := tx.Commit(); err != nil {
		// Restore the original file when database commit fails, preserving it even
		// when restoration itself is blocked by a newly created source file.
		restoreErr := moveFileForImport(destPath, srcFile)
		res.Status = "error"
		res.Message = fmt.Sprintf("database commit failed: %v (restore: %v)", err, restoreErr)
		return res
	}
	res.Status = "imported"
	res.LibraryID = int(id)
	res.Message = fmt.Sprintf("Imported %s (%d)", movie.Title, year)
	slog.Info("imported movie", "title", movie.Title, "year", year, "dest", destPath)
	return res
}

func (s *Server) importSeries(item importItem, anime bool) importResult {
	res := importResult{SourcePath: item.SourcePath}

	if item.TMDBID == 0 {
		res.Status = "error"
		res.Message = "tmdb_id required"
		return res
	}

	if s.tmdb == nil {
		res.Status = "error"
		res.Message = "TMDB not configured"
		return res
	}

	// Fetch metadata
	tv, err := s.tmdb.GetTV(item.TMDBID)
	if err != nil {
		res.Status = "error"
		res.Message = "TMDB fetch failed: " + err.Error()
		return res
	}

	year := 0
	if len(tv.FirstAirDate) >= 4 {
		year, _ = strconv.Atoi(tv.FirstAirDate[:4])
	}

	isAnime := anime || s.tmdb.IsAnime(tv)

	// Check already in library
	var existingID int
	if s.db.QueryRow(`SELECT id FROM media_items WHERE tmdb_id = ? AND type = 'series'`, item.TMDBID).Scan(&existingID) == nil {
		// Series exists, but we can still import files to match episodes
		res.LibraryID = existingID
	}

	// Find video files
	videoFiles := findAllVideoFiles(item.SourcePath)
	if len(videoFiles) == 0 {
		res.Status = "error"
		res.Message = "no video files found"
		return res
	}

	// Parse episodes before writing anything. A combined file cannot safely be
	// assigned to only its first episode, including in an otherwise valid folder.
	type parsedEpisode struct {
		file    string
		season  int
		episode int
	}
	var episodes []parsedEpisode
	for _, f := range videoFiles {
		name := filepath.Base(f)
		if indexer.IsMultiEpisodeRelease(name) || indexer.HasAbsoluteEpisodeRange(name, tv.Name) {
			res.Status = "error"
			res.Message = "combined episode files are not supported: " + name
			return res
		}
		season, episode, ok := indexer.EpisodeNumbers(name)
		if !ok {
			continue
		}
		episodes = append(episodes, parsedEpisode{file: f, season: season, episode: episode})
	}

	if len(episodes) == 0 {
		res.Status = "error"
		res.Message = "no episodes detected (expected SxxExx or 1x01 pattern)"
		return res
	}

	var mediaID int64
	if res.LibraryID > 0 {
		mediaID = int64(res.LibraryID)
	} else {
		details := make(map[int]*metadata.TMDBSeasonDetail)
		for _, season := range tv.Seasons {
			if season.SeasonNumber == 0 && !isAnime {
				continue
			}
			detail, err := s.tmdb.GetSeason(tv.ID, season.SeasonNumber)
			if err != nil {
				res.Status = "error"
				res.Message = "failed to fetch season metadata: " + err.Error()
				return res
			}
			details[season.SeasonNumber] = detail
		}
		tx, err := s.db.Begin()
		if err != nil {
			res.Status = "error"
			res.Message = err.Error()
			return res
		}
		defer tx.Rollback()
		// Create library entry
		genres := genreNames(tv.Genres)
		genresJSON, _ := json.Marshal(genres)

		result, err := tx.Exec(`INSERT INTO media_items
			(type, title, year, anime, tmdb_id, imdb_id, tvdb_id, overview, poster_url, backdrop_url, genres, rating, rating_source, quality_profile_id, status)
			VALUES ('series', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'tmdb', ?, 'wanted')`,
			tv.Name, year, isAnime, tv.ID,
			tv.ExternalIDs.IMDBID, tv.ExternalIDs.TVDBID,
			tv.Overview,
			metadata.PosterURL(tv.PosterPath),
			metadata.BackdropURL(tv.BackdropPath),
			string(genresJSON),
			tv.VoteAverage,
			item.QualityProfileID,
		)
		if err != nil {
			res.Status = "error"
			res.Message = "database error: " + err.Error()
			return res
		}

		mediaID, _ = result.LastInsertId()

		for _, season := range tv.Seasons {
			if detail := details[season.SeasonNumber]; detail != nil {
				if err := insertSeason(tx, mediaID, season, detail); err != nil {
					res.Status = "error"
					res.Message = "failed to save season: " + err.Error()
					return res
				}
			}
		}
		if err := tx.Commit(); err != nil {
			res.Status = "error"
			res.Message = "failed to save series"
			return res
		}
	}

	// Now import the episode files
	importedCount := 0
	for _, ep := range episodes {
		// Find matching episode in DB
		var epID int
		err := s.db.QueryRow(`SELECT e.id FROM episodes e
			JOIN seasons s ON e.season_id = s.id
			WHERE e.media_item_id = ? AND s.number = ? AND e.number = ?`,
			mediaID, ep.season, ep.episode).Scan(&epID)
		if err != nil {
			continue
		}

		// Get episode details for path generation
		var epTitle sql.NullString
		var epType string
		var absNum sql.NullInt64
		s.db.QueryRow(`SELECT title, episode_type, absolute_number FROM episodes WHERE id = ?`, epID).
			Scan(&epTitle, &epType, &absNum)

		epTitleStr := ""
		if epTitle.Valid {
			epTitleStr = epTitle.String
		}

		ext := filepath.Ext(ep.file)
		var destPath string
		if isAnime && (epType == "special" || epType == "ova" || epType == "ona") {
			destPath = postprocess.AnimeSpecialPath(s.cfg.MediaRoot, tv.Name, ep.episode, epTitleStr, ext)
		} else if isAnime {
			absNumInt := ep.episode
			if absNum.Valid {
				absNumInt = int(absNum.Int64)
			}
			destPath = postprocess.AnimeEpisodePath(s.cfg.MediaRoot, tv.Name, ep.season, ep.episode, absNumInt, epTitleStr, ext)
		} else {
			destPath = postprocess.SeriesEpisodePath(s.cfg.MediaRoot, tv.Name, year, ep.season, ep.episode, epTitleStr, ext)
		}

		if err := validateImportDestination(s.cfg.MediaRoot, destPath); err != nil {
			slog.Warn("unsafe import destination", "error", err)
			continue
		}
		if err := moveFileForImport(ep.file, destPath); err != nil {
			slog.Warn("import episode move failed", "file", ep.file, "error", err)
			continue
		}

		if _, err := s.db.Exec(`UPDATE episodes SET file_path = ?, status = 'available' WHERE id = ?`, destPath, epID); err != nil {
			moveFileForImport(destPath, ep.file)
			continue
		}
		importedCount++
	}

	// Update series status
	var total, available int
	s.db.QueryRow(`SELECT COUNT(*), COUNT(CASE WHEN status = 'available' THEN 1 END)
		FROM episodes WHERE media_item_id = ? AND episode_type = 'standard'`, mediaID).Scan(&total, &available)

	seriesStatus := "wanted"
	if available == total && total > 0 {
		seriesStatus = "available"
	}
	s.db.Exec(`UPDATE media_items SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, seriesStatus, mediaID)

	s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'imported', ?)`,
		mediaID, fmt.Sprintf("Imported %d episodes for %s", importedCount, tv.Name))

	res.Status = "imported"
	if importedCount == 0 {
		res.Status = "error"
	}
	res.LibraryID = int(mediaID)
	res.Message = fmt.Sprintf("Imported %d/%d episodes for %s", importedCount, len(episodes), tv.Name)
	slog.Info("imported series", "title", tv.Name, "episodes", importedCount)
	return res
}

func (s *Server) importMusic(item importItem) importResult {
	res := importResult{SourcePath: item.SourcePath}

	if item.ReleaseGroupID == "" {
		res.Status = "error"
		res.Message = "release_group_id required"
		return res
	}

	if s.musicbrainz == nil {
		res.Status = "error"
		res.Message = "MusicBrainz not configured"
		return res
	}

	if strings.TrimSpace(item.ArtistMBID) == "" || strings.TrimSpace(item.ArtistName) == "" || strings.TrimSpace(item.AlbumTitle) == "" {
		res.Status = "error"
		res.Message = "artist_mbid, artist_name, and album_title required"
		return res
	}
	var audioFiles []string
	if err := filepath.Walk(item.SourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && postprocess.IsAudioFile(info.Name()) {
			audioFiles = append(audioFiles, path)
		}
		return nil
	}); err != nil {
		res.Status = "error"
		res.Message = "cannot read source: " + err.Error()
		return res
	}
	if len(audioFiles) == 0 {
		res.Status = "error"
		res.Message = "no audio files found"
		return res
	}
	// Check already in library
	var existingID int
	if s.db.QueryRow(`SELECT id FROM albums WHERE release_group_id = ?`, item.ReleaseGroupID).Scan(&existingID) == nil {
		res.Status = "skipped"
		res.Message = "already in library"
		res.LibraryID = existingID
		return res
	}

	// Get artist info
	artistName := item.ArtistName
	artistMBID := item.ArtistMBID
	albumTitle := item.AlbumTitle

	// Find or create artist
	var artistID int
	err := s.db.QueryRow(`SELECT id FROM artists WHERE mbid = ?`, artistMBID).Scan(&artistID)
	if err != nil {
		result, err := s.db.Exec(`INSERT INTO artists (mbid, name, sort_name) VALUES (?, ?, ?)`,
			artistMBID, artistName, artistName)
		if err != nil {
			res.Status = "error"
			res.Message = "create artist: " + err.Error()
			return res
		}
		id, _ := result.LastInsertId()
		artistID = int(id)
	}

	// Get release info from MusicBrainz
	release, err := s.musicbrainz.GetBestRelease(item.ReleaseGroupID)
	if err != nil {
		res.Status = "error"
		res.Message = "MusicBrainz release lookup failed: " + err.Error()
		return res
	}

	// Determine year from release date
	year := 0
	if release != nil && len(release.Date) >= 4 {
		year, _ = strconv.Atoi(release.Date[:4])
	}

	// Create album with image URL
	imageURL := fmt.Sprintf("https://coverartarchive.org/release-group/%s/front-250", item.ReleaseGroupID)
	result, err := s.db.Exec(`INSERT INTO albums (artist_id, release_group_id, title, year, album_type, quality_profile_id, image_url, status)
		VALUES (?, ?, ?, ?, 'album', ?, ?, 'wanted')`,
		artistID, item.ReleaseGroupID, albumTitle, year, item.QualityProfileID, imageURL)
	if err != nil {
		res.Status = "error"
		res.Message = "create album: " + err.Error()
		return res
	}
	albumID, _ := result.LastInsertId()

	// Insert tracks from MusicBrainz
	trackCount := 0
	if release != nil {
		for _, media := range release.Media {
			for _, track := range media.Tracks {
				s.db.Exec(`INSERT INTO tracks (album_id, mbid, title, number, disc_number, duration_ms)
					VALUES (?, ?, ?, ?, ?, ?)`,
					albumID, track.Recording.ID, track.Title, track.Position, media.Position, track.Length)
				trackCount++
			}
		}
		s.db.Exec(`UPDATE albums SET track_count = ?, mbid = ? WHERE id = ?`, trackCount, release.ID, albumID)
	}

	// Get tracks from DB for matching
	rows, err := s.db.Query(`SELECT id, number, disc_number, title FROM tracks WHERE album_id = ? ORDER BY disc_number, number`, albumID)
	if err != nil {
		res.Status = "imported"
		res.LibraryID = int(albumID)
		res.Message = "album created but tracks not matched"
		return res
	}
	defer rows.Close()

	type trackInfo struct {
		id     int
		number int
		disc   int
		title  string
	}
	var tracks []trackInfo
	for rows.Next() {
		var t trackInfo
		rows.Scan(&t.id, &t.number, &t.disc, &t.title)
		tracks = append(tracks, t)
	}

	maxDisc := 1
	for _, t := range tracks {
		if t.disc > maxDisc {
			maxDisc = t.disc
		}
	}

	// Match and move audio files
	importedTracks := 0
	for _, af := range audioFiles {
		trackNum, discNum := parseTrackNumber(filepath.Base(af))
		if trackNum == 0 {
			continue
		}

		for _, t := range tracks {
			if t.number == trackNum && (maxDisc == 1 || t.disc == discNum) {
				ext := filepath.Ext(af)
				destPath := postprocess.MusicTrackPath(s.cfg.MediaRoot, artistName, albumTitle, year, t.number, t.disc, t.title, ext)
				if err := validateImportDestination(s.cfg.MediaRoot, destPath); err != nil {
					slog.Warn("unsafe import destination", "error", err)
					continue
				}
				if err := moveFileForImport(af, destPath); err != nil {
					slog.Warn("import track move failed", "track", t.number, "error", err)
					continue
				}
				if _, err := s.db.Exec(`UPDATE tracks SET file_path = ?, status = 'available' WHERE id = ?`, destPath, t.id); err != nil {
					slog.Error("failed to record imported track", "error", err)
					continue
				}
				importedTracks++
				break
			}
		}
	}

	// Save cover art
	albumDir := filepath.Dir(postprocess.MusicTrackPath(s.cfg.MediaRoot, artistName, albumTitle, year, 1, 1, "", ".flac"))
	albumStatus := "wanted"
	if importedTracks > 0 && importedTracks == trackCount {
		albumStatus = "available"
	}
	if _, err := s.db.Exec(`UPDATE albums SET status = ?, root_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, albumStatus, albumDir, albumID); err != nil {
		res.Status = "error"
		res.Message = "failed to save import status"
		return res
	}
	if importedTracks > 0 && s.processor != nil {
		s.processor.SaveCoverToAlbumDir(int(albumID), albumDir, item.SourcePath)
	}

	// Cache cover art in background
	if coverart := s.coverart; coverart != nil {
		go coverart.GetCover(item.ReleaseGroupID)
	}

	s.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'imported', ?)`,
		albumID, fmt.Sprintf("Imported %d/%d tracks for %s - %s", importedTracks, trackCount, artistName, albumTitle))

	res.Status = "imported"
	if importedTracks == 0 {
		res.Status = "error"
	}
	res.LibraryID = int(albumID)
	res.Message = fmt.Sprintf("Imported %d/%d tracks for %s - %s", importedTracks, trackCount, artistName, albumTitle)
	slog.Info("imported music", "artist", artistName, "album", albumTitle, "tracks", importedTracks)
	return res
}

// -- Helpers --

func findVideoFilesFlat(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && postprocess.IsVideoFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func findAudioFilesFlat(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && postprocess.IsAudioFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func findAllVideoFiles(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	if !info.IsDir() {
		if postprocess.IsVideoFile(info.Name()) {
			return []string{path}
		}
		return nil
	}

	var files []string
	filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if postprocess.IsVideoFile(fi.Name()) {
			files = append(files, p)
		}
		return nil
	})
	return files
}

func moveFileForImport(src, dst string) error {
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if destInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(sourceInfo, destInfo) {
			return nil
		}
		return fmt.Errorf("destination already exists: %s", dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	// Link is atomic and refuses to replace an existing destination. Fall back to
	// exclusive creation for cross-volume moves or filesystems without hard links.
	if err := os.Link(src, dst); err == nil {
		if err := os.Remove(src); err != nil {
			os.Remove(dst)
			return err
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, sourceInfo.Mode().Perm())
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		out.Close()
		if !complete {
			os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return err
	}
	complete = true
	return nil
}

// parseTrackNumber extracts track number from filename (same logic as postprocess).
func parseTrackNumber(filename string) (trackNum, discNum int) {
	discNum = 1
	name := filepath.Base(filename)

	var d, t int
	if n, _ := fmt.Sscanf(name, "%d-%02d", &d, &t); n == 2 && d > 0 && d <= 20 && t > 0 {
		return t, d
	}
	if n, _ := fmt.Sscanf(name, "%d", &t); n == 1 && t > 0 && t <= 999 {
		return t, 1
	}
	return 0, 1
}

func validateImportDestination(root, dest string) error {
	rel, err := filepath.Rel(root, dest)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("import destination escapes media root")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("import destination contains a symlink")
		}
	}
	return nil
}
