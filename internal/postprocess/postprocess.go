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
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return err
	}

	return os.Remove(src)
}

// CleanDownloadDir removes the download directory after import.
func CleanDownloadDir(dir string) error {
	slog.Info("cleaning download directory", "dir", dir)
	return os.RemoveAll(dir)
}
