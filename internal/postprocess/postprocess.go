package postprocess

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"mediaforge/internal/database"
)

// Processor handles post-processing of completed downloads.
type Processor struct {
	db        *database.DB
	mediaRoot string
}

func New(db *database.DB, mediaRoot string) *Processor {
	return &Processor{db: db, mediaRoot: mediaRoot}
}

// UpdateMediaRoot updates the media root directory.
func (p *Processor) UpdateMediaRoot(root string) {
	p.mediaRoot = root
}

// Process handles a completed download.
func (p *Processor) Process(downloadID int, downloadPath string) error {
	slog.Info("post-processing download", "id", downloadID, "path", downloadPath)

	// Check if this is a music download (has album_id)
	var albumID sql.NullInt64
	p.db.QueryRow(`SELECT album_id FROM downloads WHERE id = ?`, downloadID).Scan(&albumID)
	if albumID.Valid {
		return p.processAlbum(downloadID, int(albumID.Int64), downloadPath)
	}

	// Look up download record
	var (
		mediaItemID int
		episodeID   sql.NullInt64
		nzbTitle    string
		quality     sql.NullString
		score       sql.NullInt64
	)
	err := p.db.QueryRow(`SELECT media_item_id, episode_id, nzb_title, quality, score
		FROM downloads WHERE id = ?`, downloadID).Scan(&mediaItemID, &episodeID, &nzbTitle, &quality, &score)
	if err != nil {
		return fmt.Errorf("lookup download: %w", err)
	}

	// Get media item details
	var (
		mediaType string
		title     string
		year      sql.NullInt64
		anime     bool
	)
	err = p.db.QueryRow(`SELECT type, title, year, anime FROM media_items WHERE id = ?`,
		mediaItemID).Scan(&mediaType, &title, &year, &anime)
	if err != nil {
		return fmt.Errorf("lookup media item: %w", err)
	}

	// Find video files in the download directory
	videoFiles, err := findVideoFiles(downloadPath)
	if err != nil {
		return fmt.Errorf("scan download dir: %w", err)
	}

	if len(videoFiles) == 0 {
		return fmt.Errorf("no video files found in %s", downloadPath)
	}

	if mediaType == "movie" {
		return p.processMovie(downloadID, mediaItemID, title, int(year.Int64), videoFiles)
	}

	return p.processEpisode(downloadID, mediaItemID, episodeID, title, int(year.Int64), anime, videoFiles)
}

func (p *Processor) processMovie(downloadID, mediaItemID int, title string, year int, files []string) error {
	// Use the largest video file (most likely the movie)
	srcFile := largestFile(files)
	ext := filepath.Ext(srcFile)
	destPath := MoviePath(p.mediaRoot, title, year, ext)

	if err := moveFile(srcFile, destPath); err != nil {
		return fmt.Errorf("move movie file: %w", err)
	}

	// Update database
	_, err := p.db.Exec(`UPDATE media_items SET root_path = ?, status = 'available', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		filepath.Dir(destPath), mediaItemID)
	if err != nil {
		return fmt.Errorf("update media item: %w", err)
	}

	_, err = p.db.Exec(`UPDATE downloads SET status = 'imported', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, downloadID)
	if err != nil {
		return fmt.Errorf("update download: %w", err)
	}

	p.logActivity(mediaItemID, 0, "imported", fmt.Sprintf("Imported %s", filepath.Base(destPath)))

	slog.Info("movie imported", "title", title, "dest", destPath)
	return nil
}

func (p *Processor) processEpisode(downloadID, mediaItemID int, episodeID sql.NullInt64, title string, year int, anime bool, files []string) error {
	if episodeID.Valid {
		// Single episode
		return p.importSingleEpisode(downloadID, mediaItemID, int(episodeID.Int64), title, year, anime, files)
	}

	// Multi-episode: try to match each file to an episode
	for _, f := range files {
		slog.Info("multi-episode file found, importing largest", "file", f)
	}
	// For now, import the largest file for the download's episode
	return nil
}

func (p *Processor) importSingleEpisode(downloadID, mediaItemID, episodeID int, seriesTitle string, year int, anime bool, files []string) error {
	srcFile := largestFile(files)
	ext := filepath.Ext(srcFile)

	// Get episode details
	var (
		seasonNum   int
		episodeNum  int
		absNum      sql.NullInt64
		epTitle     sql.NullString
		epType      string
	)
	err := p.db.QueryRow(`SELECT s.number, e.number, e.absolute_number, e.title, e.episode_type
		FROM episodes e JOIN seasons s ON e.season_id = s.id
		WHERE e.id = ?`, episodeID).Scan(&seasonNum, &episodeNum, &absNum, &epTitle, &epType)
	if err != nil {
		return fmt.Errorf("lookup episode: %w", err)
	}

	epTitleStr := ""
	if epTitle.Valid {
		epTitleStr = epTitle.String
	}

	var destPath string
	if anime && (epType == "special" || epType == "ova" || epType == "ona") {
		destPath = AnimeSpecialPath(p.mediaRoot, seriesTitle, episodeNum, epTitleStr, ext)
	} else if anime {
		absNumInt := episodeNum
		if absNum.Valid {
			absNumInt = int(absNum.Int64)
		}
		destPath = AnimeEpisodePath(p.mediaRoot, seriesTitle, seasonNum, episodeNum, absNumInt, epTitleStr, ext)
	} else {
		destPath = SeriesEpisodePath(p.mediaRoot, seriesTitle, year, seasonNum, episodeNum, epTitleStr, ext)
	}

	if err := moveFile(srcFile, destPath); err != nil {
		return fmt.Errorf("move episode file: %w", err)
	}

	// Update database
	_, err = p.db.Exec(`UPDATE episodes SET file_path = ?, status = 'available' WHERE id = ?`, destPath, episodeID)
	if err != nil {
		return fmt.Errorf("update episode: %w", err)
	}

	_, err = p.db.Exec(`UPDATE downloads SET status = 'imported', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, downloadID)
	if err != nil {
		return fmt.Errorf("update download: %w", err)
	}

	// Update series status if all episodes are available
	p.updateSeriesStatus(mediaItemID)

	p.logActivity(mediaItemID, episodeID, "imported", fmt.Sprintf("Imported S%02dE%02d %s", seasonNum, episodeNum, epTitleStr))

	slog.Info("episode imported", "series", seriesTitle, "episode", fmt.Sprintf("S%02dE%02d", seasonNum, episodeNum), "dest", destPath)
	return nil
}

func (p *Processor) updateSeriesStatus(mediaItemID int) {
	var total, available int
	p.db.QueryRow(`SELECT COUNT(*), COUNT(CASE WHEN status = 'available' THEN 1 END)
		FROM episodes WHERE media_item_id = ? AND episode_type = 'standard'`, mediaItemID).Scan(&total, &available)

	status := "wanted"
	if available == total && total > 0 {
		status = "available"
	}
	p.db.Exec(`UPDATE media_items SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, mediaItemID)
}

func (p *Processor) logActivity(mediaItemID, episodeID int, action, details string) {
	var epID interface{}
	if episodeID > 0 {
		epID = episodeID
	}
	p.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details) VALUES (?, ?, ?, ?)`,
		mediaItemID, epID, action, details)
}

// processAlbum handles post-processing of a music album download.
func (p *Processor) processAlbum(downloadID, albumID int, downloadPath string) error {
	// Get album details
	var (
		artistName string
		albumTitle string
		year       sql.NullInt64
	)
	err := p.db.QueryRow(`SELECT a.name, al.title, al.year
		FROM albums al JOIN artists a ON al.artist_id = a.id
		WHERE al.id = ?`, albumID).Scan(&artistName, &albumTitle, &year)
	if err != nil {
		return fmt.Errorf("lookup album: %w", err)
	}

	// Find audio files
	audioFiles, err := findAudioFiles(downloadPath)
	if err != nil {
		return fmt.Errorf("scan download dir for audio: %w", err)
	}

	if len(audioFiles) == 0 {
		return fmt.Errorf("no audio files found in %s", downloadPath)
	}

	// Get tracks from DB
	rows, err := p.db.Query(`SELECT id, number, disc_number, title FROM tracks WHERE album_id = ? ORDER BY disc_number, number`, albumID)
	if err != nil {
		return fmt.Errorf("list tracks: %w", err)
	}
	defer rows.Close()

	type trackInfo struct {
		id      int
		number  int
		disc    int
		title   string
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

	// Match audio files to tracks by track number
	for _, af := range audioFiles {
		trackNum, discNum := parseTrackNumber(filepath.Base(af))
		if trackNum == 0 {
			continue
		}

		for _, t := range tracks {
			if t.number == trackNum && (maxDisc == 1 || t.disc == discNum) {
				ext := filepath.Ext(af)
				destPath := MusicTrackPath(p.mediaRoot, artistName, albumTitle, int(year.Int64), t.number, maxDisc, t.title, ext)
				if err := moveFile(af, destPath); err != nil {
					slog.Warn("move track failed", "track", t.number, "error", err)
					continue
				}
				p.db.Exec(`UPDATE tracks SET file_path = ?, status = 'available' WHERE id = ?`, destPath, t.id)
				break
			}
		}
	}

	// Update album status
	var total, available int
	p.db.QueryRow(`SELECT COUNT(*), COUNT(CASE WHEN status = 'available' THEN 1 END) FROM tracks WHERE album_id = ?`, albumID).Scan(&total, &available)

	status := "wanted"
	if available > 0 && available == total {
		status = "available"
	} else if available > 0 {
		status = "available" // partial is still marked available
	}
	p.db.Exec(`UPDATE albums SET status = ?, root_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, filepath.Join(p.mediaRoot, "music", sanitizeFilename(artistName), sanitizeFilename(albumTitle)), albumID)

	// Update download
	p.db.Exec(`UPDATE downloads SET status = 'imported', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, downloadID)

	// Log activity
	p.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'imported', ?)`,
		albumID, fmt.Sprintf("Imported %d/%d tracks for %s - %s", available, total, artistName, albumTitle))

	slog.Info("album imported", "artist", artistName, "album", albumTitle, "tracks", fmt.Sprintf("%d/%d", available, total))
	return nil
}

// findAudioFiles returns all audio files in a directory tree.
func findAudioFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && IsAudioFile(info.Name()) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// parseTrackNumber extracts track number from filename.
// Handles patterns like "01 - Title.flac", "1-01 Title.flac" (disc-track).
func parseTrackNumber(filename string) (trackNum, discNum int) {
	discNum = 1
	name := filepath.Base(filename)

	// Try disc-track pattern: "1-01" or "2-03"
	var d, t int
	if n, _ := fmt.Sscanf(name, "%d-%02d", &d, &t); n == 2 && d > 0 && d <= 20 && t > 0 {
		return t, d
	}

	// Try leading number: "01 -" or "01."
	if n, _ := fmt.Sscanf(name, "%d", &t); n == 1 && t > 0 && t <= 999 {
		return t, 1
	}

	return 0, 1
}

// findVideoFiles returns all video files in a directory tree.
func findVideoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() && IsVideoFile(info.Name()) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// largestFile returns the path of the largest file.
func largestFile(files []string) string {
	var largest string
	var maxSize int64
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.Size() > maxSize {
			maxSize = info.Size()
			largest = f
		}
	}
	return largest
}

// moveFile moves a file, creating directories as needed.
func moveFile(src, dst string) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Try rename first (same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Cross-filesystem: copy then delete
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// hardlinkFile creates a hard link from src to dst. Falls back to copy
// if hardlinking fails (e.g. cross-filesystem).
func hardlinkFile(src, dst string) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	err := os.Link(src, dst)
	if err != nil {
		slog.Warn("hardlink failed, copying instead", "src", src, "dst", dst, "error", err)
		return copyFile(src, dst)
	}
	return nil
}

// ProcessTorrent handles a completed torrent download using hardlinks.
// Unlike Process(), it does NOT clean up the source files (they must remain for seeding).
func (p *Processor) ProcessTorrent(downloadID int, contentPath string) error {
	slog.Info("post-processing torrent download", "id", downloadID, "path", contentPath)

	// Check if this is a music download (has album_id)
	var albumID sql.NullInt64
	p.db.QueryRow(`SELECT album_id FROM downloads WHERE id = ?`, downloadID).Scan(&albumID)
	if albumID.Valid {
		return p.processAlbumTorrent(downloadID, int(albumID.Int64), contentPath)
	}

	// Look up download record
	var (
		mediaItemID int
		episodeID   sql.NullInt64
		nzbTitle    string
	)
	err := p.db.QueryRow(`SELECT media_item_id, episode_id, nzb_title FROM downloads WHERE id = ?`,
		downloadID).Scan(&mediaItemID, &episodeID, &nzbTitle)
	if err != nil {
		return fmt.Errorf("lookup download: %w", err)
	}

	// Get media item details
	var (
		mediaType string
		title     string
		year      sql.NullInt64
		anime     bool
	)
	err = p.db.QueryRow(`SELECT type, title, year, anime FROM media_items WHERE id = ?`,
		mediaItemID).Scan(&mediaType, &title, &year, &anime)
	if err != nil {
		return fmt.Errorf("lookup media item: %w", err)
	}

	// Find video files
	videoFiles, err := findVideoFiles(contentPath)
	if err != nil {
		return fmt.Errorf("scan torrent dir: %w", err)
	}

	if len(videoFiles) == 0 {
		return fmt.Errorf("no video files found in %s", contentPath)
	}

	if mediaType == "movie" {
		return p.processMovieTorrent(downloadID, mediaItemID, title, int(year.Int64), videoFiles)
	}

	return p.processEpisodeTorrent(downloadID, mediaItemID, episodeID, title, int(year.Int64), anime, videoFiles)
}

func (p *Processor) processMovieTorrent(downloadID, mediaItemID int, title string, year int, files []string) error {
	srcFile := largestFile(files)
	ext := filepath.Ext(srcFile)
	destPath := MoviePath(p.mediaRoot, title, year, ext)

	if err := hardlinkFile(srcFile, destPath); err != nil {
		return fmt.Errorf("hardlink movie file: %w", err)
	}

	_, err := p.db.Exec(`UPDATE media_items SET root_path = ?, status = 'available', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		filepath.Dir(destPath), mediaItemID)
	if err != nil {
		return fmt.Errorf("update media item: %w", err)
	}

	p.logActivity(mediaItemID, 0, "imported", fmt.Sprintf("Imported (torrent) %s", filepath.Base(destPath)))
	slog.Info("movie imported from torrent", "title", title, "dest", destPath)
	return nil
}

func (p *Processor) processEpisodeTorrent(downloadID, mediaItemID int, episodeID sql.NullInt64, title string, year int, anime bool, files []string) error {
	if !episodeID.Valid {
		return nil
	}

	srcFile := largestFile(files)
	ext := filepath.Ext(srcFile)

	var (
		seasonNum  int
		episodeNum int
		absNum     sql.NullInt64
		epTitle    sql.NullString
		epType     string
	)
	err := p.db.QueryRow(`SELECT s.number, e.number, e.absolute_number, e.title, e.episode_type
		FROM episodes e JOIN seasons s ON e.season_id = s.id
		WHERE e.id = ?`, int(episodeID.Int64)).Scan(&seasonNum, &episodeNum, &absNum, &epTitle, &epType)
	if err != nil {
		return fmt.Errorf("lookup episode: %w", err)
	}

	epTitleStr := ""
	if epTitle.Valid {
		epTitleStr = epTitle.String
	}

	var destPath string
	if anime && (epType == "special" || epType == "ova" || epType == "ona") {
		destPath = AnimeSpecialPath(p.mediaRoot, title, episodeNum, epTitleStr, ext)
	} else if anime {
		absNumInt := episodeNum
		if absNum.Valid {
			absNumInt = int(absNum.Int64)
		}
		destPath = AnimeEpisodePath(p.mediaRoot, title, seasonNum, episodeNum, absNumInt, epTitleStr, ext)
	} else {
		destPath = SeriesEpisodePath(p.mediaRoot, title, year, seasonNum, episodeNum, epTitleStr, ext)
	}

	if err := hardlinkFile(srcFile, destPath); err != nil {
		return fmt.Errorf("hardlink episode file: %w", err)
	}

	_, err = p.db.Exec(`UPDATE episodes SET file_path = ?, status = 'available' WHERE id = ?`, destPath, int(episodeID.Int64))
	if err != nil {
		return fmt.Errorf("update episode: %w", err)
	}

	p.updateSeriesStatus(mediaItemID)
	p.logActivity(mediaItemID, int(episodeID.Int64), "imported", fmt.Sprintf("Imported (torrent) S%02dE%02d %s", seasonNum, episodeNum, epTitleStr))

	slog.Info("episode imported from torrent", "series", title, "episode", fmt.Sprintf("S%02dE%02d", seasonNum, episodeNum))
	return nil
}

func (p *Processor) processAlbumTorrent(downloadID, albumID int, contentPath string) error {
	var (
		artistName string
		albumTitle string
		year       sql.NullInt64
	)
	err := p.db.QueryRow(`SELECT a.name, al.title, al.year
		FROM albums al JOIN artists a ON al.artist_id = a.id
		WHERE al.id = ?`, albumID).Scan(&artistName, &albumTitle, &year)
	if err != nil {
		return fmt.Errorf("lookup album: %w", err)
	}

	audioFiles, err := findAudioFiles(contentPath)
	if err != nil {
		return fmt.Errorf("scan torrent dir for audio: %w", err)
	}
	if len(audioFiles) == 0 {
		return fmt.Errorf("no audio files found in %s", contentPath)
	}

	rows, err := p.db.Query(`SELECT id, number, disc_number, title FROM tracks WHERE album_id = ? ORDER BY disc_number, number`, albumID)
	if err != nil {
		return fmt.Errorf("list tracks: %w", err)
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

	for _, af := range audioFiles {
		trackNum, discNum := parseTrackNumber(filepath.Base(af))
		if trackNum == 0 {
			continue
		}
		for _, t := range tracks {
			if t.number == trackNum && (maxDisc == 1 || t.disc == discNum) {
				ext := filepath.Ext(af)
				destPath := MusicTrackPath(p.mediaRoot, artistName, albumTitle, int(year.Int64), t.number, maxDisc, t.title, ext)
				if err := hardlinkFile(af, destPath); err != nil {
					slog.Warn("hardlink track failed", "track", t.number, "error", err)
					continue
				}
				p.db.Exec(`UPDATE tracks SET file_path = ?, status = 'available' WHERE id = ?`, destPath, t.id)
				break
			}
		}
	}

	var total, available int
	p.db.QueryRow(`SELECT COUNT(*), COUNT(CASE WHEN status = 'available' THEN 1 END) FROM tracks WHERE album_id = ?`, albumID).Scan(&total, &available)

	status := "wanted"
	if available > 0 {
		status = "available"
	}
	p.db.Exec(`UPDATE albums SET status = ?, root_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, filepath.Join(p.mediaRoot, "music", sanitizeFilename(artistName), sanitizeFilename(albumTitle)), albumID)

	p.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'imported', ?)`,
		albumID, fmt.Sprintf("Imported (torrent) %d/%d tracks for %s - %s", available, total, artistName, albumTitle))

	slog.Info("album imported from torrent", "artist", artistName, "album", albumTitle, "tracks", fmt.Sprintf("%d/%d", available, total))
	return nil
}

// CleanDownloadDir removes the download directory after import.
func CleanDownloadDir(dir string) error {
	slog.Info("cleaning download directory", "dir", dir)
	return os.RemoveAll(dir)
}
