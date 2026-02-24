package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"os"
	"strings"

	"mediaforge/internal/config"
	"mediaforge/internal/database"
	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/metadata"
	"mediaforge/internal/postprocess"
)

// Scheduler runs periodic background tasks.
type Scheduler struct {
	db        *database.DB
	cfg       *config.Config
	grabberSvc *grabber.Grabber
	newznab   *indexer.NewznabClient
	tmdb      *metadata.TMDBClient
	anilist   *metadata.AniListClient
	mapping   *metadata.Mapping
	processor *postprocess.Processor
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func New(
	db *database.DB,
	cfg *config.Config,
	g *grabber.Grabber,
	n *indexer.NewznabClient,
	t *metadata.TMDBClient,
	a *metadata.AniListClient,
	m *metadata.Mapping,
	p *postprocess.Processor,
) *Scheduler {
	return &Scheduler{
		db:        db,
		cfg:       cfg,
		grabberSvc: g,
		newznab:   n,
		tmdb:      t,
		anilist:   a,
		mapping:   m,
		processor: p,
	}
}

// Start begins all scheduled tasks.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Retry previously failed downloads on startup
	s.retryFailedDownloads()

	s.runTask(ctx, "download-poll", 10*time.Second, s.pollDownloads)
	s.runTask(ctx, "rss-sync", 15*time.Minute, s.rssSync)
	s.runTask(ctx, "episode-check", 30*time.Minute, s.checkEpisodeAirDates)
	s.runTask(ctx, "metadata-refresh", 24*time.Hour, s.refreshMetadata)
	s.runTask(ctx, "mapping-refresh", 24*time.Hour, s.refreshMapping)

	slog.Info("scheduler started")
}

// Stop gracefully stops all scheduled tasks.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	slog.Info("scheduler stopped")
}

func (s *Scheduler) runTask(ctx context.Context, name string, interval time.Duration, fn func()) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run immediately on start
		s.safeRun(name, fn)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.safeRun(name, fn)
			}
		}
	}()
}

func (s *Scheduler) safeRun(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scheduler task panicked", "task", name, "error", r)
		}
	}()
	fn()
}

// retryFailedDownloads attempts to re-process downloads that were marked as failed,
// in case they failed due to path issues that have since been fixed.
func (s *Scheduler) retryFailedDownloads() {
	rows, err := s.db.Query(`SELECT id, download_path FROM downloads WHERE status = 'failed' AND download_path IS NOT NULL AND download_path != ''`)
	if err != nil {
		return
	}
	defer rows.Close()

	var retries []struct {
		id   int
		path string
	}
	for rows.Next() {
		var id int
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			continue
		}
		retries = append(retries, struct {
			id   int
			path string
		}{id, path})
	}

	for _, r := range retries {
		resolved := resolveSABnzbdPath(r.path)
		if _, err := os.Stat(resolved); err != nil {
			continue // path still not accessible, skip
		}

		slog.Info("retrying failed download", "id", r.id, "path", resolved)
		if err := s.processor.Process(r.id, resolved); err != nil {
			slog.Debug("retry still failed", "id", r.id, "error", err)
		} else {
			postprocess.CleanDownloadDir(resolved)
			slog.Info("retry succeeded", "id", r.id)
		}
	}
}

// pollDownloads checks SABnzbd for download progress and triggers post-processing.
func (s *Scheduler) pollDownloads() {
	// Check queue for progress updates
	queue, err := s.grabberSvc.GetQueue()
	if err != nil {
		slog.Debug("failed to poll SABnzbd queue", "error", err)
		return
	}

	for _, slot := range queue.Slots {
		status := "downloading"
		if slot.Status == "Extracting" || slot.Status == "Repairing" {
			status = "extracting"
		}
		s.db.Exec(`UPDATE downloads SET status = ? WHERE sabnzbd_nzo_id = ? AND status NOT IN ('completed', 'imported', 'failed')`,
			status, slot.NzoID)
	}

	// Check history for completed/failed downloads
	history, err := s.grabberSvc.GetHistory()
	if err != nil {
		slog.Debug("failed to poll SABnzbd history", "error", err)
		return
	}

	for _, slot := range history.Slots {
		var dlID int
		var dlStatus string
		err := s.db.QueryRow(`SELECT id, status FROM downloads WHERE sabnzbd_nzo_id = ?`, slot.NzoID).Scan(&dlID, &dlStatus)
		if err != nil {
			continue
		}

		if dlStatus == "imported" || dlStatus == "failed" {
			continue
		}

		if slot.Status == "Completed" {
			storagePath := resolveSABnzbdPath(slot.Storage)
			s.db.Exec(`UPDATE downloads SET status = 'completed', download_path = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
				storagePath, dlID)

			// Trigger post-processing
			if err := s.processor.Process(dlID, storagePath); err != nil {
				slog.Error("post-processing failed", "download_id", dlID, "error", err)
				s.db.Exec(`UPDATE downloads SET status = 'failed' WHERE id = ?`, dlID)
			} else {
				// Clean up download dir
				postprocess.CleanDownloadDir(storagePath)
			}
		} else if slot.Status == "Failed" {
			slog.Warn("download failed", "nzo_id", slot.NzoID, "message", slot.FailMessage)
			s.db.Exec(`UPDATE downloads SET status = 'failed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, dlID)

			var mediaItemID int
			s.db.QueryRow(`SELECT media_item_id FROM downloads WHERE id = ?`, dlID).Scan(&mediaItemID)
			s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'failed', ?)`,
				mediaItemID, slot.FailMessage)
		}
	}
}

// checkEpisodeAirDates searches for episodes that have aired.
func (s *Scheduler) checkEpisodeAirDates() {
	rows, err := s.db.Query(`SELECT e.id, e.media_item_id, e.number, e.absolute_number, e.episode_type,
		s.number as season_number, m.title, m.tvdb_id, m.anime, m.quality_profile_id
		FROM episodes e
		JOIN seasons s ON e.season_id = s.id
		JOIN media_items m ON e.media_item_id = m.id
		WHERE e.status = 'wanted' AND e.air_date <= date('now') AND e.episode_type = 'standard'`)
	if err != nil {
		slog.Error("episode air date check failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			epID, mediaItemID, epNum, seasonNum, tvdbID, profileID int
			absNum sql.NullInt64
			epType, title string
			anime bool
		)
		if err := rows.Scan(&epID, &mediaItemID, &epNum, &absNum, &epType, &seasonNum, &title, &tvdbID, &anime, &profileID); err != nil {
			continue
		}

		s.searchAndGrab(mediaItemID, epID, title, tvdbID, seasonNum, epNum, absNum, anime, profileID)
	}
}

// rssSync queries indexers for new releases matching wanted items.
func (s *Scheduler) rssSync() {
	slog.Debug("running RSS sync")
	// This performs the same logic as checkEpisodeAirDates but is designed
	// to also pick up movies and catch releases that appeared between checks
	s.checkEpisodeAirDates()
}

// refreshMetadata re-fetches metadata for all items.
func (s *Scheduler) refreshMetadata() {
	slog.Info("refreshing metadata for all items")

	rows, err := s.db.Query(`SELECT id, tmdb_id, anilist_id, type, anime FROM media_items`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, tmdbID int
		var anilistID sql.NullInt64
		var mediaType string
		var anime bool
		if err := rows.Scan(&id, &tmdbID, &anilistID, &mediaType, &anime); err != nil {
			continue
		}

		if anime && anilistID.Valid && s.anilist != nil {
			media, err := s.anilist.GetAnime(int(anilistID.Int64))
			if err != nil {
				slog.Warn("refresh anilist metadata failed", "id", id, "error", err)
				continue
			}
			s.db.Exec(`UPDATE media_items SET rating = ?, overview = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				float64(media.AverageScore)/10.0, media.Description, id)
		} else if s.tmdb != nil && tmdbID > 0 {
			if mediaType == "movie" {
				movie, err := s.tmdb.GetMovie(tmdbID)
				if err != nil {
					continue
				}
				s.db.Exec(`UPDATE media_items SET rating = ?, overview = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
					movie.VoteAverage, movie.Overview, id)
			} else {
				tv, err := s.tmdb.GetTV(tmdbID)
				if err != nil {
					continue
				}
				s.db.Exec(`UPDATE media_items SET rating = ?, overview = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
					tv.VoteAverage, tv.Overview, id)
			}
		}
	}
}

// refreshMapping re-downloads the AniDB<->TMDB mapping.
func (s *Scheduler) refreshMapping() {
	if s.mapping != nil {
		if err := s.mapping.Refresh(); err != nil {
			slog.Error("mapping refresh failed", "error", err)
		}
	}
}

func (s *Scheduler) searchAndGrab(mediaItemID, episodeID int, title string, tvdbID, season, episode int, absNum sql.NullInt64, anime bool, profileID int) {
	// Load indexers
	idxRows, err := s.db.Query(`SELECT id, name, url, api_key, priority, enabled FROM indexers WHERE enabled = TRUE ORDER BY priority DESC`)
	if err != nil {
		return
	}
	defer idxRows.Close()

	var indexers []indexer.IndexerConfig
	for idxRows.Next() {
		var idx indexer.IndexerConfig
		if err := idxRows.Scan(&idx.ID, &idx.Name, &idx.URL, &idx.APIKey, &idx.Priority, &idx.Enabled); err != nil {
			continue
		}
		indexers = append(indexers, idx)
	}

	if len(indexers) == 0 {
		return
	}

	// Load quality profile
	var profile indexer.QualityProfile
	err = s.db.QueryRow(`SELECT id, name, qualities, tags, language, reject_patterns, upgrade_allowed
		FROM quality_profiles WHERE id = ?`, profileID).Scan(
		&profile.ID, &profile.Name, &profile.Qualities, &profile.Tags,
		&profile.Language, &profile.RejectPatterns, &profile.UpgradeAllowed)
	if err != nil {
		return
	}

	// Search
	var releases []indexer.Release
	if anime && absNum.Valid {
		titles := []string{title}
		releases = s.newznab.SearchAnimeEpisode(indexers, titles, int(absNum.Int64))
	} else {
		releases = s.newznab.SearchEpisode(indexers, tvdbID, season, episode, title)
	}

	// Score releases
	for i := range releases {
		indexer.ScoreRelease(&releases[i], &profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		return
	}

	// Check if we already have a file with equal/higher score
	var existingScore sql.NullInt64
	s.db.QueryRow(`SELECT score FROM downloads WHERE episode_id = ? AND status = 'imported' ORDER BY score DESC LIMIT 1`,
		episodeID).Scan(&existingScore)
	if existingScore.Valid && !indexer.ShouldUpgrade(best.Score, int(existingScore.Int64), profile.UpgradeAllowed) {
		return
	}

	// Grab the release
	nzoID, err := s.grabberSvc.GrabNZB(best.NZBURL, best.Title)
	if err != nil {
		slog.Error("grab failed", "title", best.Title, "error", err)
		return
	}

	// Record download
	qualityJSON, _ := json.Marshal(best.Quality)
	s.db.Exec(`INSERT INTO downloads (media_item_id, episode_id, nzb_title, sabnzbd_nzo_id, quality, score)
		VALUES (?, ?, ?, ?, ?, ?)`,
		mediaItemID, episodeID, best.Title, nzoID, string(qualityJSON), best.Score)

	s.db.Exec(`UPDATE episodes SET status = 'downloading' WHERE id = ?`, episodeID)

	s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details)
		VALUES (?, ?, 'grabbed', ?)`,
		mediaItemID, episodeID, fmt.Sprintf("Grabbed %s from %s", best.Title, best.Indexer))

	slog.Info("grabbed release", "title", best.Title, "score", best.Score, "indexer", best.Indexer)
}

// resolveSABnzbdPath translates SABnzbd's internal path to a path accessible by Zarr.
// SABnzbd reports paths like /config/Downloads/complete/... but Zarr sees that directory
// mounted at /sabnzbd-downloads or /data/usenet depending on configuration.
func resolveSABnzbdPath(sabPath string) string {
	// If the path already exists (e.g. using /data/usenet), use it directly
	if _, err := os.Stat(sabPath); err == nil {
		return sabPath
	}

	// Try remapping SABnzbd's /config/Downloads prefix to /sabnzbd-downloads
	if strings.HasPrefix(sabPath, "/config/Downloads/") {
		remapped := "/sabnzbd-downloads/" + sabPath[len("/config/Downloads/"):]
		if _, err := os.Stat(remapped); err == nil {
			slog.Debug("remapped SABnzbd path", "from", sabPath, "to", remapped)
			return remapped
		}
	}

	// Try /data/usenet as alternative base
	if strings.HasPrefix(sabPath, "/config/Downloads/") {
		remapped := "/data/usenet/" + sabPath[len("/config/Downloads/"):]
		if _, err := os.Stat(remapped); err == nil {
			slog.Debug("remapped SABnzbd path", "from", sabPath, "to", remapped)
			return remapped
		}
	}

	// Return original if no remapping works
	slog.Warn("could not remap SABnzbd path", "path", sabPath)
	return sabPath
}
