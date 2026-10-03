package scanner

import (
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"mediaforge/internal/database"
	"mediaforge/internal/postprocess"
)

// Scanner discovers existing media files on disk and matches them to library items.
type Scanner struct {
	mu        sync.Mutex
	db        *database.DB
	mediaRoot string
}

func New(db *database.DB, mediaRoot string) *Scanner {
	return &Scanner{db: db, mediaRoot: mediaRoot}
}

// UpdateMediaRoot updates the media root directory.
func (s *Scanner) UpdateMediaRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mediaRoot = root
}

// ScanResult holds the results of a library scan.
type ScanResult struct {
	FilesFound   int `json:"files_found"`
	FilesMatched int `json:"files_matched"`
	FilesNew     int `json:"files_new"`
}

// Scan walks the media root and matches files to library items.
func (s *Scanner) Scan() (*ScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slog.Info("starting library scan", "root", s.mediaRoot)

	result := &ScanResult{}

	for _, subdir := range []string{"movies", "tv", "anime"} {
		dir := filepath.Join(s.mediaRoot, subdir)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !postprocess.IsVideoFile(info.Name()) {
				return nil
			}

			result.FilesFound++

			switch subdir {
			case "movies":
				if s.matchMovieFile(path) {
					result.FilesMatched++
				}
			case "tv":
				if s.matchTVFile(path) {
					result.FilesMatched++
				}
			case "anime":
				if s.matchAnimeFile(path) {
					result.FilesMatched++
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	result.FilesNew = result.FilesFound - result.FilesMatched
	slog.Info("library scan complete", "found", result.FilesFound, "matched", result.FilesMatched)
	return result, nil
}

// Movie pattern: movies/Title (Year)/Title (Year).ext
var movieDirPattern = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)$`)

func (s *Scanner) matchMovieFile(path string) bool {
	rel, err := filepath.Rel(filepath.Join(s.mediaRoot, "movies"), path)
	if err != nil {
		return false
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 1 {
		return false
	}

	match := movieDirPattern.FindStringSubmatch(parts[0])
	if match == nil {
		return false
	}

	title := strings.TrimSpace(match[1])
	year, _ := strconv.Atoi(match[2])

	// Try to match against library
	id, err := s.findMedia(title, "movie", false, year)
	if err != nil {
		return false
	}

	// A scan matches names, not an imported file identity. Even the same path
	// may now contain an externally replaced file. Never retain an old release
	// title or use NULL (which revives download history as an upgrade baseline).
	_, err = s.db.Exec(`UPDATE media_items SET root_path = ?, status = 'available', current_release_title = 'manual-grab', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		filepath.Dir(path), id)
	if err != nil {
		return false
	}

	slog.Info("matched movie file", "title", title, "year", year, "path", path)
	return true
}

// TV pattern: tv/Title (Year)/Season XX/Title - SxxExx - Episode Title.ext
var tvEpisodePattern = regexp.MustCompile(`(?i)S(\d{1,3})E(\d{1,4})(?:[^0-9]|$)`)

func (s *Scanner) matchTVFile(path string) bool {
	rel, err := filepath.Rel(filepath.Join(s.mediaRoot, "tv"), path)
	if err != nil {
		return false
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return false
	}

	// Extract series from directory name
	match := movieDirPattern.FindStringSubmatch(parts[0])
	if match == nil {
		return false
	}
	title := strings.TrimSpace(match[1])

	// Extract season/episode from filename
	filename := filepath.Base(path)
	epMatch := tvEpisodePattern.FindStringSubmatch(filename)
	if epMatch == nil {
		return false
	}
	season, _ := strconv.Atoi(epMatch[1])
	episode, _ := strconv.Atoi(epMatch[2])

	year, _ := strconv.Atoi(match[2])
	return s.matchEpisode(title, false, year, season, episode, path)
}

// Anime pattern: anime/Title/Season XX/Title - SxxExx - xxx - Episode Title.ext
func (s *Scanner) matchAnimeFile(path string) bool {
	rel, err := filepath.Rel(filepath.Join(s.mediaRoot, "anime"), path)
	if err != nil {
		return false
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return false
	}

	title := parts[0]

	filename := filepath.Base(path)
	epMatch := tvEpisodePattern.FindStringSubmatch(filename)
	if epMatch == nil {
		return false
	}
	season, _ := strconv.Atoi(epMatch[1])
	episode, _ := strconv.Atoi(epMatch[2])

	return s.matchEpisode(title, true, 0, season, episode, path)
}

func (s *Scanner) matchEpisode(title string, anime bool, year, season, episode int, path string) bool {
	mediaID, err := s.findMedia(title, "series", anime, year)
	if err != nil {
		return false
	}

	var epID int
	err = s.db.QueryRow(`SELECT e.id FROM episodes e
		JOIN seasons s ON e.season_id = s.id
		WHERE e.media_item_id = ? AND s.number = ? AND e.number = ?`,
		mediaID, season, episode).Scan(&epID)
	if err != nil {
		return false
	}

	// No persisted inode/content identity proves this is the previously
	// imported file, even when file_path is unchanged; invalidate provenance.
	if _, err := s.db.Exec(`UPDATE episodes SET file_path = ?, status = 'available', current_release_title = 'manual-grab' WHERE id = ?`, path, epID); err != nil {
		return false
	}

	slog.Info("matched episode file", "title", title, "season", season, "episode", episode, "path", path)
	return true
}

// Match the sanitized on-disk title exactly. LIKE allowed a folder such as
// "Alien" to mark "Aliens" or a wildcard-containing title as available.
func (s *Scanner) findMedia(title, kind string, anime bool, year int) (int, error) {
	rows, err := s.db.Query(`SELECT id,title,year FROM media_items WHERE type=? AND (type='movie' OR anime=?)`, kind, anime)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var id int
		var candidate string
		var candidateYear sql.NullInt64
		if err := rows.Scan(&id, &candidate, &candidateYear); err != nil {
			return 0, err
		}
		if !strings.EqualFold(postprocess.SanitizeFilename(candidate), title) || (year > 0 && candidateYear.Int64 != int64(year)) {
			continue
		}
		if found != 0 {
			return 0, sql.ErrNoRows
		}
		found = id
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if found == 0 {
		return 0, sql.ErrNoRows
	}
	return found, nil
}
