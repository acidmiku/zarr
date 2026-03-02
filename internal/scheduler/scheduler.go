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
	"mediaforge/internal/qbt"
	"mediaforge/internal/rutracker"
)

// Scheduler runs periodic background tasks.
type Scheduler struct {
	db          *database.DB
	cfg         *config.Config
	grabberSvc  *grabber.Grabber
	newznab     *indexer.NewznabClient
	tmdb        *metadata.TMDBClient
	anilist     *metadata.AniListClient
	mapping     *metadata.Mapping
	processor   *postprocess.Processor
	musicbrainz *metadata.MusicBrainzClient
	qbt         *qbt.Client
	rtClient    *rutracker.Client
	cancel      context.CancelFunc
	wg          sync.WaitGroup
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
	mb *metadata.MusicBrainzClient,
	qbtClient *qbt.Client,
	rtClient *rutracker.Client,
) *Scheduler {
	return &Scheduler{
		db:          db,
		cfg:         cfg,
		grabberSvc:  g,
		newznab:     n,
		tmdb:        t,
		anilist:     a,
		mapping:     m,
		processor:   p,
		musicbrainz: mb,
		qbt:         qbtClient,
		rtClient:    rtClient,
	}
}

// Start begins all scheduled tasks.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Retry previously failed downloads on startup
	s.retryFailedDownloads()

	s.runTask(ctx, "download-poll", 10*time.Second, s.pollDownloads)
	s.runTask(ctx, "torrent-poll", 15*time.Second, s.pollTorrentDownloads)
	s.runTask(ctx, "rss-sync", 15*time.Minute, s.rssSync)
	s.runTask(ctx, "episode-check", 30*time.Minute, s.checkEpisodeAirDates)
	s.runTask(ctx, "music-search", 30*time.Minute, s.searchWantedAlbums)
	s.runTask(ctx, "seed-cleanup", 30*time.Minute, s.cleanupSeededTorrents)
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
				// Reset music album to wanted on post-processing failure
				var ppAlbumID sql.NullInt64
				s.db.QueryRow(`SELECT album_id FROM downloads WHERE id = ?`, dlID).Scan(&ppAlbumID)
				if ppAlbumID.Valid && ppAlbumID.Int64 > 0 {
					s.db.Exec(`UPDATE albums SET status = 'wanted', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'downloading'`,
						ppAlbumID.Int64)
				}
			} else {
				// Clean up download dir
				postprocess.CleanDownloadDir(storagePath)
			}
		} else if slot.Status == "Failed" {
			slog.Warn("download failed", "nzo_id", slot.NzoID, "message", slot.FailMessage)
			s.db.Exec(`UPDATE downloads SET status = 'failed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, dlID)

			var mediaItemID sql.NullInt64
			var albumID sql.NullInt64
			s.db.QueryRow(`SELECT media_item_id, album_id FROM downloads WHERE id = ?`, dlID).Scan(&mediaItemID, &albumID)

			if albumID.Valid && albumID.Int64 > 0 {
				// Music download failed — reset album to wanted so scheduler retries
				s.db.Exec(`UPDATE albums SET status = 'wanted', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'downloading'`,
					albumID.Int64)
				s.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'failed', ?)`,
					albumID.Int64, slot.FailMessage)
				slog.Warn("music download failed, album reset to wanted", "album_id", albumID.Int64)
			} else if mediaItemID.Valid {
				s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'failed', ?)`,
					mediaItemID.Int64, slot.FailMessage)
			}
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

// searchWantedAlbums searches for all albums with "wanted" status.
func (s *Scheduler) searchWantedAlbums() {
	rows, err := s.db.Query(`SELECT al.id, a.name, al.title, al.year, al.quality_profile_id
		FROM albums al JOIN artists a ON al.artist_id = a.id
		WHERE al.status = 'wanted'`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var albumID, year, profileID int
		var artist, title string
		if err := rows.Scan(&albumID, &artist, &title, &year, &profileID); err != nil {
			continue
		}
		s.searchAndGrabAlbum(albumID, artist, title, year, profileID)
	}
}

// searchAndGrabAlbum searches indexers for a specific album and grabs the best release.
func (s *Scheduler) searchAndGrabAlbum(albumID int, artist, title string, year, profileID int) {
	allIndexers := s.loadIndexers()
	if len(allIndexers) == 0 {
		return
	}

	profile := s.loadProfile(profileID)
	if profile == nil {
		return
	}

	// Newznab search
	newznabIdxs := filterIndexersByType(allIndexers, "newznab", "music")
	releases := s.newznab.SearchMusic(newznabIdxs, artist, title, year)
	for i := range releases {
		releases[i].DownloadType = "nzb"
	}

	// Rutracker search
	rtIndexers := filterIndexersByType(allIndexers, "rutracker", "music")
	if len(rtIndexers) > 0 && s.rtClient != nil {
		query := fmt.Sprintf("%s %s", artist, title)
		forumIDs := rutracker.DefaultForumIDs["music"]
		for _, idx := range rtIndexers {
			results, err := s.rtClient.Search(query, forumIDs, idx.Username, idx.Password)
			if err != nil {
				slog.Warn("rutracker music search failed", "indexer", idx.Name, "error", err)
				continue
			}
			for _, r := range results {
				parsed := indexer.ParseMusicReleaseName(r.Title)
				releases = append(releases, indexer.Release{
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
	}

	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		return
	}

	// Check existing download score
	var existingScore sql.NullInt64
	s.db.QueryRow(`SELECT score FROM downloads WHERE album_id = ? AND status = 'imported' ORDER BY score DESC LIMIT 1`,
		albumID).Scan(&existingScore)
	if existingScore.Valid && !indexer.ShouldUpgrade(best.Score, int(existingScore.Int64), profile.UpgradeAllowed) {
		return
	}

	qualityJSON, _ := json.Marshal(best.Quality)

	if best.DownloadType == "torrent" && best.TopicID > 0 {
		rtIdxs := filterIndexersByType(allIndexers, "rutracker", "")
		if len(rtIdxs) > 0 && s.rtClient != nil && s.qbt != nil {
			idx := rtIdxs[0]
			torrentData, err := s.rtClient.DownloadTorrent(best.TopicID, idx.Username, idx.Password)
			if err != nil {
				slog.Error("torrent download failed", "title", best.Title, "error", err)
				return
			}
			result, _ := s.db.Exec(`INSERT INTO downloads (album_id, nzb_title, quality, score, download_type) VALUES (?, ?, ?, ?, 'torrent')`,
				albumID, best.Title, string(qualityJSON), best.Score)
			dlID, _ := result.LastInsertId()
			s.qbt.AddTorrent(torrentData, fmt.Sprintf("dl_%d.torrent", dlID), int(dlID))
			s.db.Exec(`UPDATE albums SET status = 'downloading', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, albumID)
			s.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'grabbed', ?)`,
				albumID, fmt.Sprintf("Grabbed torrent %s from %s", best.Title, best.Indexer))
			slog.Info("grabbed music torrent", "title", best.Title, "score", best.Score, "indexer", best.Indexer)
			return
		}
	}

	nzoID, err := s.grabberSvc.GrabNZB(best.NZBURL, best.Title)
	if err != nil {
		slog.Error("music grab failed", "title", best.Title, "error", err)
		return
	}

	s.db.Exec(`INSERT INTO downloads (album_id, nzb_title, sabnzbd_nzo_id, quality, score, download_type) VALUES (?, ?, ?, ?, ?, 'nzb')`,
		albumID, best.Title, nzoID, string(qualityJSON), best.Score)

	s.db.Exec(`UPDATE albums SET status = 'downloading', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, albumID)

	s.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'grabbed', ?)`,
		albumID, fmt.Sprintf("Grabbed %s from %s", best.Title, best.Indexer))

	slog.Info("grabbed music release", "title", best.Title, "score", best.Score, "indexer", best.Indexer)
}

// loadIndexers returns enabled indexers from the database.
func (s *Scheduler) loadIndexers() []indexer.IndexerConfig {
	idxRows, err := s.db.Query(`SELECT id, name, url, api_key, priority, enabled, COALESCE(type,'newznab'), COALESCE(username,''), COALESCE(password,''), COALESCE(content_types,'["movie","series","anime","music"]') FROM indexers WHERE enabled = TRUE ORDER BY priority DESC`)
	if err != nil {
		return nil
	}
	defer idxRows.Close()

	var indexers []indexer.IndexerConfig
	for idxRows.Next() {
		var idx indexer.IndexerConfig
		var contentTypesJSON string
		if err := idxRows.Scan(&idx.ID, &idx.Name, &idx.URL, &idx.APIKey, &idx.Priority, &idx.Enabled, &idx.Type, &idx.Username, &idx.Password, &contentTypesJSON); err != nil {
			continue
		}
		json.Unmarshal([]byte(contentTypesJSON), &idx.ContentTypes)
		if idx.Type == "" {
			idx.Type = "newznab"
		}
		indexers = append(indexers, idx)
	}
	return indexers
}

// loadProfile loads a quality profile by ID.
func (s *Scheduler) loadProfile(profileID int) *indexer.QualityProfile {
	var profile indexer.QualityProfile
	err := s.db.QueryRow(`SELECT id, name, qualities, tags, language, reject_patterns, upgrade_allowed, COALESCE(profile_type, 'video')
		FROM quality_profiles WHERE id = ?`, profileID).Scan(
		&profile.ID, &profile.Name, &profile.Qualities, &profile.Tags,
		&profile.Language, &profile.RejectPatterns, &profile.UpgradeAllowed, &profile.ProfileType)
	if err != nil {
		return nil
	}
	return &profile
}

func (s *Scheduler) searchAndGrab(mediaItemID, episodeID int, title string, tvdbID, season, episode int, absNum sql.NullInt64, anime bool, profileID int) {
	allIndexers := s.loadIndexers()
	if len(allIndexers) == 0 {
		return
	}

	profile := s.loadProfile(profileID)
	if profile == nil {
		return
	}

	contentType := "series"
	if anime {
		contentType = "anime"
	}

	// Newznab search
	newznabIdxs := filterIndexersByType(allIndexers, "newznab", contentType)
	var releases []indexer.Release
	if anime && absNum.Valid {
		titles := []string{title}
		releases = s.newznab.SearchAnimeEpisode(newznabIdxs, titles, int(absNum.Int64))
	} else {
		releases = s.newznab.SearchEpisode(newznabIdxs, tvdbID, season, episode, title)
	}
	for i := range releases {
		releases[i].DownloadType = "nzb"
	}

	// Rutracker search
	rtIndexers := filterIndexersByType(allIndexers, "rutracker", contentType)
	if len(rtIndexers) > 0 && s.rtClient != nil {
		query := fmt.Sprintf("%s S%02dE%02d", title, season, episode)
		if anime && absNum.Valid {
			query = fmt.Sprintf("%s %d", title, absNum.Int64)
		}
		forumIDs := rutracker.DefaultForumIDs[contentType]
		for _, idx := range rtIndexers {
			results, err := s.rtClient.Search(query, forumIDs, idx.Username, idx.Password)
			if err != nil {
				slog.Warn("rutracker search failed", "indexer", idx.Name, "error", err)
				continue
			}
			for _, r := range results {
				parsed := indexer.ParseReleaseName(r.Title)
				releases = append(releases, indexer.Release{
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
	}

	// Score releases
	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
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

	qualityJSON, _ := json.Marshal(best.Quality)

	if best.DownloadType == "torrent" && best.TopicID > 0 {
		// Torrent grab
		rtIdxs := filterIndexersByType(allIndexers, "rutracker", "")
		if len(rtIdxs) > 0 && s.rtClient != nil && s.qbt != nil {
			idx := rtIdxs[0]
			torrentData, err := s.rtClient.DownloadTorrent(best.TopicID, idx.Username, idx.Password)
			if err != nil {
				slog.Error("torrent download failed", "title", best.Title, "error", err)
				return
			}
			result, _ := s.db.Exec(`INSERT INTO downloads (media_item_id, episode_id, nzb_title, quality, score, download_type) VALUES (?, ?, ?, ?, ?, 'torrent')`,
				mediaItemID, episodeID, best.Title, string(qualityJSON), best.Score)
			dlID, _ := result.LastInsertId()
			s.qbt.AddTorrent(torrentData, fmt.Sprintf("dl_%d.torrent", dlID), int(dlID))
			s.db.Exec(`UPDATE episodes SET status = 'downloading' WHERE id = ?`, episodeID)
			s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details) VALUES (?, ?, 'grabbed', ?)`,
				mediaItemID, episodeID, fmt.Sprintf("Grabbed torrent %s from %s", best.Title, best.Indexer))
			slog.Info("grabbed torrent release", "title", best.Title, "score", best.Score, "indexer", best.Indexer)
			return
		}
	}

	// NZB grab
	nzoID, err := s.grabberSvc.GrabNZB(best.NZBURL, best.Title)
	if err != nil {
		slog.Error("grab failed", "title", best.Title, "error", err)
		return
	}

	s.db.Exec(`INSERT INTO downloads (media_item_id, episode_id, nzb_title, sabnzbd_nzo_id, quality, score, download_type)
		VALUES (?, ?, ?, ?, ?, ?, 'nzb')`,
		mediaItemID, episodeID, best.Title, nzoID, string(qualityJSON), best.Score)

	s.db.Exec(`UPDATE episodes SET status = 'downloading' WHERE id = ?`, episodeID)

	s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details)
		VALUES (?, ?, 'grabbed', ?)`,
		mediaItemID, episodeID, fmt.Sprintf("Grabbed %s from %s", best.Title, best.Indexer))

	slog.Info("grabbed release", "title", best.Title, "score", best.Score, "indexer", best.Indexer)
}

// pollTorrentDownloads checks qBittorrent for torrent progress and triggers post-processing.
func (s *Scheduler) pollTorrentDownloads() {
	if s.qbt == nil {
		return
	}

	// Check if qBittorrent is enabled
	enabled, _ := s.db.GetSetting("qbittorrent_enabled")
	if enabled != "true" {
		return
	}

	torrents, err := s.qbt.GetTorrents()
	if err != nil {
		slog.Debug("failed to poll qBittorrent", "error", err)
		return
	}

	for _, t := range torrents {
		// Extract download ID from tags ("dl_42" → 42)
		dlID := extractDownloadID(t.Tags)
		if dlID == 0 {
			continue
		}

		// Map qBittorrent state to our status
		var status string
		switch t.State {
		case "downloading", "stalledDL", "metaDL", "allocating", "checkingDL":
			status = "downloading"
		case "uploading", "stalledUP", "pausedUP", "forcedUP", "checkingUP", "queuedUP":
			status = "seeding"
		case "error", "missingFiles":
			status = "failed"
		default:
			status = "downloading"
		}

		// Update download record
		s.db.Exec(`UPDATE downloads SET status = ?, qbt_hash = ? WHERE id = ? AND status NOT IN ('imported', 'failed')`,
			status, t.Hash, dlID)

		// If torrent completed downloading (progress == 1.0) and not yet post-processed
		if t.Progress >= 1.0 {
			var completedTS sql.NullInt64
			s.db.QueryRow(`SELECT completed_at_ts FROM downloads WHERE id = ?`, dlID).Scan(&completedTS)

			if !completedTS.Valid {
				// Mark completion time
				now := time.Now().Unix()
				s.db.Exec(`UPDATE downloads SET completed_at_ts = ?, status = 'seeding' WHERE id = ?`, now, dlID)

				// Trigger post-processing via hardlink
				contentPath := t.ContentPath
				if contentPath == "" {
					contentPath = t.SavePath
				}
				if err := s.processor.ProcessTorrent(dlID, contentPath); err != nil {
					slog.Error("torrent post-processing failed", "download_id", dlID, "error", err)
					// Don't set status to failed — let it keep seeding, user can retry
				}
			}
		}
	}
}

// cleanupSeededTorrents removes torrents that have seeded long enough.
func (s *Scheduler) cleanupSeededTorrents() {
	if s.qbt == nil {
		return
	}

	enabled, _ := s.db.GetSetting("qbittorrent_enabled")
	if enabled != "true" {
		return
	}

	// Load seed time setting
	seedTimeStr, _ := s.db.GetSetting("torrent_seed_time_hours")
	seedTimeHours := 24
	if v, err := fmt.Sscanf(seedTimeStr, "%d", &seedTimeHours); v == 0 || err != nil {
		seedTimeHours = 24
	}

	removeAfterStr, _ := s.db.GetSetting("torrent_remove_after_seed")
	removeAfterSeed := removeAfterStr != "false" // default true

	if !removeAfterSeed {
		return
	}

	rows, err := s.db.Query(`SELECT id, qbt_hash, completed_at_ts FROM downloads WHERE download_type = 'torrent' AND status = 'seeding' AND completed_at_ts IS NOT NULL`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now().Unix()
	seedDuration := int64(seedTimeHours) * 3600

	for rows.Next() {
		var dlID int
		var hash sql.NullString
		var completedTS int64
		if err := rows.Scan(&dlID, &hash, &completedTS); err != nil {
			continue
		}

		if now-completedTS < seedDuration {
			continue
		}

		// Seed time exceeded — remove torrent
		if hash.Valid && hash.String != "" {
			if err := s.qbt.DeleteTorrent(hash.String, true); err != nil {
				slog.Warn("failed to delete seeded torrent", "hash", hash.String, "error", err)
				continue
			}
		}

		s.db.Exec(`UPDATE downloads SET status = 'completed' WHERE id = ?`, dlID)
		slog.Info("removed seeded torrent", "download_id", dlID, "seed_hours", (now-completedTS)/3600)
	}
}

// extractDownloadID extracts the download ID from qBittorrent tags like "dl_42".
func extractDownloadID(tags string) int {
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(tag, "dl_") {
			var id int
			fmt.Sscanf(tag, "dl_%d", &id)
			return id
		}
	}
	return 0
}

// filterIndexersByType returns indexers of a specific type that support a content type.
func filterIndexersByType(indexers []indexer.IndexerConfig, idxType, contentType string) []indexer.IndexerConfig {
	var filtered []indexer.IndexerConfig
	for _, idx := range indexers {
		if idx.Type != idxType {
			continue
		}
		if contentType != "" {
			found := false
			for _, ct := range idx.ContentTypes {
				if ct == contentType {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, idx)
	}
	return filtered
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
