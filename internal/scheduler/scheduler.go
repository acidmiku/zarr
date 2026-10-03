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
	"path/filepath"
	"strconv"
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
	metadataMu  sync.RWMutex
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

// SetMetadataClient refreshes the client after onboarding/settings changes.
func (s *Scheduler) SetMetadataClient(client *metadata.TMDBClient) {
	s.metadataMu.Lock()
	defer s.metadataMu.Unlock()
	s.tmdb = client
}

// Start begins all scheduled tasks.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Recover interrupted searches before launching the new scheduler workers.
	for _, table := range []string{"episodes", "media_items", "albums"} {
		s.db.Exec("UPDATE " + table + " SET status='wanted' WHERE status='searching'")
	}

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
	rows, err := s.db.Query(`SELECT id, download_path FROM downloads WHERE download_type = 'nzb' AND status = 'failed' AND download_path IS NOT NULL AND download_path != ''`)
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
		s.db.Exec(`UPDATE downloads SET status = ? WHERE sabnzbd_nzo_id = ? AND status NOT IN ('completed', 'imported', 'failed', 'cancelled')`,
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

		if dlStatus == "imported" || dlStatus == "failed" || dlStatus == "cancelled" {
			continue
		}

		if slot.Status == "Completed" {
			storagePath := resolveSABnzbdPath(slot.Storage)
			s.db.Exec(`UPDATE downloads SET status = 'completed', download_path = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ? AND status NOT IN ('cancelled','imported')`,
				storagePath, dlID)

			// Trigger post-processing
			if err := s.processor.Process(dlID, storagePath); err != nil {
				slog.Error("post-processing failed", "download_id", dlID, "error", err)
				s.db.Exec(`UPDATE downloads SET status = 'failed' WHERE id = ? AND status != 'cancelled'`, dlID)
				// Import/path failures retain the downloaded source for an
				// explicit retry; searching again would re-grab the same release.

			} else {
				// Clean up download dir
				postprocess.CleanDownloadDir(storagePath)
			}
		} else if slot.Status == "Failed" {
			slog.Warn("download failed", "nzo_id", slot.NzoID, "message", slot.FailMessage)
			s.db.Exec(`UPDATE downloads SET status = 'failed', completed_at = CURRENT_TIMESTAMP WHERE id = ? AND status != 'cancelled'`, dlID)

			var mediaItemID, episodeID, albumID sql.NullInt64
			var nzbTitle string
			s.db.QueryRow(`SELECT media_item_id, episode_id, album_id, nzb_title FROM downloads WHERE id = ?`, dlID).
				Scan(&mediaItemID, &episodeID, &albumID, &nzbTitle)

			// Blacklist the failed release
			s.blacklistRelease(mediaItemID, episodeID, albumID, nzbTitle, slot.FailMessage)

			if albumID.Valid && albumID.Int64 > 0 {
				s.db.Exec(`UPDATE albums SET status = 'wanted', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'downloading'`,
					albumID.Int64)
				s.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'failed', ?)`,
					albumID.Int64, slot.FailMessage)
				slog.Warn("music download failed, will retry with next release", "album_id", albumID.Int64)
			} else if episodeID.Valid {
				s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ?`, episodeID.Int64)
				s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details) VALUES (?, ?, 'failed', ?)`,
					mediaItemID.Int64, episodeID.Int64, "Failed: "+slot.FailMessage+", retrying with next release")
				slog.Warn("episode download failed, will retry with next release", "episode_id", episodeID.Int64)
				// Auto-retry with next best release
				s.retryEpisodeWithNextRelease(int(mediaItemID.Int64), int(episodeID.Int64))
			} else if mediaItemID.Valid {
				s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ?`, mediaItemID.Int64)
				s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'failed', ?)`,
					mediaItemID.Int64, "Failed: "+slot.FailMessage+", retrying with next release")
				slog.Warn("movie download failed, will retry with next release", "media_item_id", mediaItemID.Int64)
				s.retryMovieWithNextRelease(int(mediaItemID.Int64))
			}
		}
	}
}

// checkEpisodeAirDates searches for episodes that have aired.
func (s *Scheduler) checkEpisodeAirDates() {
	rows, err := s.db.Query(`SELECT e.id, e.media_item_id, e.number, e.absolute_number, e.episode_type,
		s.number as season_number, m.title, COALESCE(m.tvdb_id,0), m.anime, COALESCE(m.quality_profile_id,0)
		FROM episodes e
		JOIN seasons s ON e.season_id = s.id
		JOIN media_items m ON e.media_item_id = m.id
		WHERE m.type = 'series' AND e.status = 'wanted' AND e.air_date <= date('now') AND e.episode_type = 'standard'`)
	if err != nil {
		slog.Error("episode air date check failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			epID, mediaItemID, epNum, seasonNum, tvdbID, profileID int
			absNum                                                 sql.NullInt64
			epType, title                                          string
			anime                                                  bool
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
	rows, err := s.db.Query(`SELECT id FROM media_items WHERE type='movie' AND status='wanted'`)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		s.retryMovieWithNextRelease(id)
	}

}

// refreshMetadata re-fetches metadata for all items.
func (s *Scheduler) refreshMetadata() {
	slog.Info("refreshing metadata for all items")
	s.metadataMu.RLock()
	tmdbClient := s.tmdb
	s.metadataMu.RUnlock()

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
		} else if tmdbClient != nil && tmdbID > 0 {
			if mediaType == "movie" {
				movie, err := tmdbClient.GetMovie(tmdbID)
				if err != nil {
					continue
				}
				s.db.Exec(`UPDATE media_items SET rating = ?, overview = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
					movie.VoteAverage, movie.Overview, id)
			} else {
				tv, err := tmdbClient.GetTV(tmdbID)
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
	rows, err := s.db.Query(`SELECT al.id, a.name, al.title, COALESCE(al.year,0), COALESCE(al.quality_profile_id,0)
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
	claimed, claimErr := s.db.Exec(`UPDATE albums SET status = 'searching' WHERE id = ? AND status = 'wanted'`, albumID)
	if claimErr != nil {
		return
	}
	count, _ := claimed.RowsAffected()
	if count != 1 {
		return
	}
	defer s.db.Exec(`UPDATE albums SET status = 'wanted' WHERE id = ? AND status = 'searching'`, albumID)

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

	// Filter blacklisted releases and score
	albumBlacklist := s.loadAlbumBlacklist(albumID)
	releases = indexer.FilterBlacklisted(releases, albumBlacklist)
	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		return
	}

	if err := s.enqueueRelease(0, 0, albumID, *best); err != nil {
		slog.Warn("enqueue release failed", "title", best.Title, "error", err)
	}
}

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
	claimed, claimErr := s.db.Exec(`UPDATE episodes SET status = 'searching' WHERE id = ? AND status = 'wanted'`, episodeID)
	if claimErr != nil {
		return
	}
	count, _ := claimed.RowsAffected()
	if count != 1 {
		return
	}
	defer s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ? AND status = 'searching'`, episodeID)

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
		releases = s.newznab.SearchAnimeEpisode(newznabIdxs, titles, int(absNum.Int64), season, episode)
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
				absolute := 0
				if anime && absNum.Valid {
					absolute = int(absNum.Int64)
				}
				if !indexer.MatchesSeriesEpisode(r.Title, title, season, episode, absolute) {
					continue
				}
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

	// Filter blacklisted releases and score
	blacklist := s.loadBlacklist(mediaItemID, episodeID)
	releases = indexer.FilterBlacklisted(releases, blacklist)
	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		return
	}

	if err := s.enqueueRelease(mediaItemID, episodeID, 0, *best); err != nil {
		slog.Warn("enqueue release failed", "title", best.Title, "error", err)
	}
}

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

		var currentStatus string
		if err := s.db.QueryRow(`SELECT status FROM downloads WHERE id = ? AND download_type = 'torrent'`, dlID).Scan(&currentStatus); err != nil {
			continue
		}
		if currentStatus == "failed" || currentStatus == "cancelled" || currentStatus == "imported" {
			continue
		}
		s.db.Exec(`UPDATE downloads SET qbt_hash = ?, seed_ratio = ? WHERE id = ?`, t.Hash, t.Ratio, dlID)
		// An imported torrent must not be blacklisted or re-imported if its
		// client later reports missing files or a temporary recheck.
		if currentStatus == "seeding" {
			continue
		}
		status := "downloading"
		if t.State == "error" || t.State == "missingFiles" {
			status = "failed"
		}
		s.db.Exec(`UPDATE downloads SET status = ? WHERE id = ? AND status NOT IN ('imported','failed','cancelled','seeding')`, status, dlID)

		if status == "failed" && currentStatus != "failed" {
			// Torrent just failed — blacklist and retry
			var mediaItemID, episodeID, albumID sql.NullInt64
			var nzbTitle string
			s.db.QueryRow(`SELECT media_item_id, episode_id, album_id, nzb_title FROM downloads WHERE id = ?`, dlID).
				Scan(&mediaItemID, &episodeID, &albumID, &nzbTitle)

			s.blacklistRelease(mediaItemID, episodeID, albumID, nzbTitle, "torrent error: "+t.State)

			if albumID.Valid && albumID.Int64 > 0 {
				s.db.Exec(`UPDATE albums SET status = 'wanted', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'downloading'`,
					albumID.Int64)
				s.db.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'failed', ?)`,
					albumID.Int64, "Torrent failed: "+t.State)
			} else if episodeID.Valid {
				s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ?`, episodeID.Int64)
				s.db.Exec(`INSERT INTO activity_log (media_item_id, episode_id, action, details) VALUES (?, ?, 'failed', ?)`,
					mediaItemID.Int64, episodeID.Int64, "Torrent failed, retrying with next release")
				s.retryEpisodeWithNextRelease(int(mediaItemID.Int64), int(episodeID.Int64))
			} else if mediaItemID.Valid {
				s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ?`, mediaItemID.Int64)
				s.db.Exec(`INSERT INTO activity_log (media_item_id, action, details) VALUES (?, 'failed', ?)`,
					mediaItemID.Int64, "Torrent failed, retrying with next release")
				s.retryMovieWithNextRelease(int(mediaItemID.Int64))
			}

			// Delete the failed torrent from qBittorrent
			s.qbt.DeleteTorrent(t.Hash, true)
			s.db.Exec(`UPDATE downloads SET status = 'failed' WHERE id = ? AND status != 'cancelled'`, dlID)
			continue
		}

		// Never use the shared save directory as the import source.
		if t.Progress >= 1 && !strings.HasPrefix(t.State, "checking") && t.State != "moving" {
			contentPath := t.ContentPath
			if contentPath == "" && t.SavePath != "" && t.Name != "" && filepath.Base(t.Name) == t.Name {
				contentPath = filepath.Join(t.SavePath, t.Name)
			}
			if contentPath == "" {
				continue
			}
			s.db.Exec(`UPDATE downloads SET download_path = ?, completed_at_ts = COALESCE(completed_at_ts, ?) WHERE id = ?`, contentPath, time.Now().Unix(), dlID)
			if err := s.processor.ProcessTorrent(dlID, contentPath); err != nil {
				slog.Error("torrent import failed; source retained for retry", "download_id", dlID, "error", err)
				s.db.Exec(`UPDATE downloads SET status = 'failed' WHERE id = ? AND status != 'cancelled'`, dlID)
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
	if seedTimeHours < 0 {
		return
	}
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

		s.db.Exec(`UPDATE downloads SET status = 'imported' WHERE id = ?`, dlID)
		slog.Info("removed seeded torrent", "download_id", dlID, "seed_hours", (now-completedTS)/3600)
	}
}

// extractDownloadID extracts the download ID from qBittorrent tags like "dl_42".
func extractDownloadID(tags string) int {
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(tag, "dl_") {
			id, err := strconv.Atoi(strings.TrimPrefix(tag, "dl_"))
			if err == nil && id > 0 {
				return id
			}
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

// blacklistRelease adds a release title to the blacklist so it won't be grabbed again.
func (s *Scheduler) blacklistRelease(mediaItemID, episodeID, albumID sql.NullInt64, title, reason string) {
	if title == "" || title == "manual-grab" {
		return
	}
	s.db.Exec(`INSERT INTO release_blacklist (media_item_id, episode_id, album_id, release_title, reason) VALUES (?, ?, ?, ?, ?)`,
		nullInt(mediaItemID), nullInt(episodeID), nullInt(albumID), title, reason)
	slog.Info("blacklisted release", "title", title, "reason", reason)
}

func nullInt(n sql.NullInt64) interface{} {
	if n.Valid {
		return n.Int64
	}
	return nil
}

// loadBlacklist returns blacklisted release titles for a given media item or episode.
func (s *Scheduler) loadBlacklist(mediaItemID int, episodeID int) []string {
	var titles []string
	var rows *sql.Rows
	var err error
	if episodeID > 0 {
		rows, err = s.db.Query(`SELECT release_title FROM release_blacklist WHERE episode_id = ?`, episodeID)
	} else {
		rows, err = s.db.Query(`SELECT release_title FROM release_blacklist WHERE media_item_id = ? AND episode_id IS NULL`, mediaItemID)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		rows.Scan(&t)
		titles = append(titles, t)
	}
	return titles
}

// loadAlbumBlacklist returns blacklisted release titles for an album.
func (s *Scheduler) loadAlbumBlacklist(albumID int) []string {
	rows, err := s.db.Query(`SELECT release_title FROM release_blacklist WHERE album_id = ?`, albumID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var t string
		rows.Scan(&t)
		titles = append(titles, t)
	}
	return titles
}

// retryEpisodeWithNextRelease searches for the next best non-blacklisted release for an episode.
func (s *Scheduler) retryEpisodeWithNextRelease(mediaItemID, episodeID int) {
	claimed, claimErr := s.db.Exec(`UPDATE episodes SET status = 'searching' WHERE id = ? AND status = 'wanted'`, episodeID)
	if claimErr != nil {
		return
	}
	count, _ := claimed.RowsAffected()
	if count != 1 {
		return
	}
	defer s.db.Exec(`UPDATE episodes SET status = 'wanted' WHERE id = ? AND status = 'searching'`, episodeID)

	var title string
	var tvdbID, profileID int
	var anime bool
	var seasonNum, epNum int
	var absNum sql.NullInt64
	s.db.QueryRow(`SELECT title, COALESCE(tvdb_id,0), anime, COALESCE(quality_profile_id,0)
		FROM media_items WHERE id = ?`, mediaItemID).Scan(&title, &tvdbID, &anime, &profileID)
	s.db.QueryRow(`SELECT s.number, e.number, e.absolute_number
		FROM episodes e JOIN seasons s ON e.season_id = s.id WHERE e.id = ?`,
		episodeID).Scan(&seasonNum, &epNum, &absNum)

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

	newznabIdxs := filterIndexersByType(allIndexers, "newznab", contentType)
	var releases []indexer.Release
	if anime && absNum.Valid {
		releases = s.newznab.SearchAnimeEpisode(newznabIdxs, []string{title}, int(absNum.Int64), seasonNum, epNum)
	} else {
		releases = s.newznab.SearchEpisode(newznabIdxs, tvdbID, seasonNum, epNum, title)
	}
	for i := range releases {
		releases[i].DownloadType = "nzb"
	}

	rtIndexers := filterIndexersByType(allIndexers, "rutracker", contentType)
	if len(rtIndexers) > 0 && s.rtClient != nil {
		query := fmt.Sprintf("%s S%02dE%02d", title, seasonNum, epNum)
		if anime && absNum.Valid {
			query = fmt.Sprintf("%s %d", title, absNum.Int64)
		}
		forumIDs := rutracker.DefaultForumIDs[contentType]
		for _, idx := range rtIndexers {
			results, err := s.rtClient.Search(query, forumIDs, idx.Username, idx.Password)
			if err != nil {
				continue
			}
			for _, r := range results {
				absolute := 0
				if anime && absNum.Valid {
					absolute = int(absNum.Int64)
				}
				if !indexer.MatchesSeriesEpisode(r.Title, title, seasonNum, epNum, absolute) {
					continue
				}
				parsed := indexer.ParseReleaseName(r.Title)
				releases = append(releases, indexer.Release{
					Title: r.Title, Size: r.Size, Quality: parsed.Quality, Tags: parsed.Tags,
					Indexer: idx.Name, DownloadType: "torrent", Seeders: r.Seeders, Leechers: r.Leechers, TopicID: r.TopicID,
				})
			}
		}
	}

	// Filter blacklisted and score
	blacklist := s.loadBlacklist(mediaItemID, episodeID)
	releases = indexer.FilterBlacklisted(releases, blacklist)
	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		slog.Info("no alternative releases found after blacklisting", "episode_id", episodeID)
		return
	}

	if err := s.enqueueRelease(mediaItemID, episodeID, 0, *best); err != nil {
		slog.Warn("enqueue release failed", "title", best.Title, "error", err)
	}
}

func (s *Scheduler) retryMovieWithNextRelease(mediaItemID int) {
	claimed, claimErr := s.db.Exec(`UPDATE media_items SET status = 'searching' WHERE id = ? AND status = 'wanted'`, mediaItemID)
	if claimErr != nil {
		return
	}
	count, _ := claimed.RowsAffected()
	if count != 1 {
		return
	}
	defer s.db.Exec(`UPDATE media_items SET status = 'wanted' WHERE id = ? AND status = 'searching'`, mediaItemID)

	var title, imdbID string
	var year, profileID int
	var anime bool
	s.db.QueryRow(`SELECT title, COALESCE(imdb_id,''), COALESCE(year,0), COALESCE(quality_profile_id,0), anime
		FROM media_items WHERE id = ?`, mediaItemID).Scan(&title, &imdbID, &year, &profileID, &anime)

	allIndexers := s.loadIndexers()
	if len(allIndexers) == 0 {
		return
	}
	profile := s.loadProfile(profileID)
	if profile == nil {
		return
	}

	newznabIdxs := indexer.MovieIndexers(allIndexers, "newznab", anime)
	releases := s.newznab.SearchMovie(newznabIdxs, imdbID, title, year)
	for i := range releases {
		releases[i].DownloadType = "nzb"
	}

	rtIndexers := indexer.MovieIndexers(allIndexers, "rutracker", anime)
	if len(rtIndexers) > 0 && s.rtClient != nil {
		query := title
		if year > 0 {
			query = fmt.Sprintf("%s %d", title, year)
		}
		rtContentType := "movie"
		if anime {
			rtContentType = "anime"
		}
		forumIDs := rutracker.DefaultForumIDs[rtContentType]
		for _, idx := range rtIndexers {
			results, err := s.rtClient.Search(query, forumIDs, idx.Username, idx.Password)
			if err != nil {
				continue
			}
			for _, r := range results {
				if !indexer.MatchesMovieText(r.Title, title, year) {
					continue
				}
				parsed := indexer.ParseReleaseName(r.Title)
				releases = append(releases, indexer.Release{
					Title: r.Title, Size: r.Size, Quality: parsed.Quality, Tags: parsed.Tags,
					Indexer: idx.Name, DownloadType: "torrent", Seeders: r.Seeders, Leechers: r.Leechers, TopicID: r.TopicID,
				})
			}
		}
	}

	blacklist := s.loadBlacklist(mediaItemID, 0)
	releases = indexer.FilterBlacklisted(releases, blacklist)
	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
	}

	best := indexer.BestRelease(releases)
	if best == nil {
		slog.Info("no alternative releases found after blacklisting", "media_item_id", mediaItemID)
		return
	}

	if err := s.enqueueRelease(mediaItemID, 0, 0, *best); err != nil {
		slog.Warn("enqueue release failed", "title", best.Title, "error", err)
	}
}

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
func (s *Scheduler) enqueueRelease(mediaID, episodeID, albumID int, rel indexer.Release) error {
	var media, episode, album any
	if mediaID > 0 {
		media = mediaID
	}
	if episodeID > 0 {
		episode = episodeID
	}
	if albumID > 0 {
		album = albumID
	}
	if rel.DownloadType == "" {
		rel.DownloadType = "nzb"
	}
	if rel.DownloadType != "nzb" && rel.DownloadType != "torrent" {
		return fmt.Errorf("unsupported download type")
	}
	if rel.Title == "" {
		rel.Title = "manual-grab"
	}
	if rel.DownloadType == "torrent" && (s.qbt == nil || s.rtClient == nil || rel.TopicID <= 0) {
		return fmt.Errorf("torrent client and topic are required")
	}
	if rel.DownloadType == "nzb" && (s.grabberSvc == nil || rel.NZBURL == "") {
		return fmt.Errorf("NZB URL and client are required")
	}
	var valid int
	var err error
	if albumID > 0 {
		err = s.db.QueryRow(`SELECT id FROM albums WHERE id=?`, albumID).Scan(&valid)
	} else if episodeID > 0 {
		err = s.db.QueryRow(`SELECT id FROM episodes WHERE id=? AND media_item_id=?`, episodeID, mediaID).Scan(&valid)
	} else {
		err = s.db.QueryRow(`SELECT id FROM media_items WHERE id=?`, mediaID).Scan(&valid)
	}
	if err != nil {
		return fmt.Errorf("library item not found")
	}
	quality, _ := json.Marshal(rel.Quality)
	res, err := s.db.Exec(`INSERT INTO downloads (media_item_id,episode_id,album_id,nzb_title,quality,score,download_type)
 SELECT ?,?,?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM downloads WHERE media_item_id IS ? AND episode_id IS ? AND album_id IS ? AND status NOT IN ('imported','failed','cancelled','completed','seeding'))`, media, episode, album, rel.Title, string(quality), rel.Score, rel.DownloadType, media, episode, album)
	if err != nil {
		return fmt.Errorf("record download: %w", err)
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return fmt.Errorf("a download is already active for this item")
	}
	id, _ := res.LastInsertId()
	submitted := false
	defer func() {
		if !submitted {
			s.db.Exec(`UPDATE downloads SET status='failed' WHERE id=?`, id)
		}
	}()
	nzoID := ""
	if rel.DownloadType == "torrent" {
		idxs := filterIndexersByType(s.loadIndexers(), "rutracker", "")
		if len(idxs) == 0 {
			return fmt.Errorf("no torrent indexer configured")
		}
		idx := idxs[0]
		for _, candidate := range idxs {
			if candidate.Name == rel.Indexer {
				idx = candidate
				break
			}
		}
		data, err := s.rtClient.DownloadTorrent(rel.TopicID, idx.Username, idx.Password)
		if err != nil {
			return err
		}
		var hash string
		hash, err = s.qbt.AddTorrent(data, fmt.Sprintf("dl_%d.torrent", id), int(id))
		if err != nil {
			return err
		}
		if hash != "" {
			if _, err := s.db.Exec(`UPDATE downloads SET qbt_hash=? WHERE id=?`, hash, id); err != nil {
				s.qbt.DeleteTorrent(hash, false)
				return err
			}
		}

	} else {
		nzoID, err = s.grabberSvc.GrabNZB(rel.NZBURL, rel.Title)
		if err != nil {
			return err
		}
		if _, err = s.db.Exec(`UPDATE downloads SET sabnzbd_nzo_id=? WHERE id=?`, nzoID, id); err != nil {
			s.grabberSvc.DeleteFromQueue(nzoID)
			return err
		}
	}
	submitted = true
	if albumID > 0 {
		_, err = s.db.Exec(`UPDATE albums SET status='downloading',updated_at=CURRENT_TIMESTAMP WHERE id=?`, albumID)
	} else if episodeID > 0 {
		_, err = s.db.Exec(`UPDATE episodes SET status='downloading' WHERE id=?`, episodeID)
	} else {
		_, err = s.db.Exec(`UPDATE media_items SET status='downloading' WHERE id=?`, mediaID)
	}
	if err != nil {
		return err
	}
	s.db.Exec(`INSERT INTO activity_log (media_item_id,episode_id,album_id,action,details) VALUES (?,?,?,'grabbed',?)`, media, episode, album, "Grabbed "+rel.Title)
	return nil
}
