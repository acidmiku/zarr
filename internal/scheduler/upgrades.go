package scheduler

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"mediaforge/internal/indexer"
	"mediaforge/internal/postprocess"
)

const upgradeBatchSize = 10

type upgradeTarget struct {
	mediaID, episodeID, profileID int
	kind, title, imdbID, path     string
	year, season, episode         int
	absolute                      sql.NullInt64
}

// Available media keeps its status throughout an upgrade. A persisted timestamp
// claims each check without hiding the playable file or repeating on restart.
func (s *Scheduler) checkQualityUpgrades() {
	if s.newznab == nil {
		return
	}
	rows, err := s.db.Query(`
 SELECT m.id,0,m.quality_profile_id,'movie',m.title,COALESCE(m.imdb_id,''),COALESCE(m.root_path,''),COALESCE(m.year,0),0,0,NULL,COALESCE(m.upgrade_checked_at,'1970-01-01') AS checked
 FROM media_items m JOIN quality_profiles p ON p.id=m.quality_profile_id
 WHERE m.type='movie' AND m.anime=1 AND m.status='available' AND p.upgrade_allowed=1
 AND json_extract(CASE WHEN json_valid(p.scoring_config) THEN p.scoring_config ELSE '{}' END,'$.preset')='radarr-anime'
 AND (m.upgrade_checked_at IS NULL OR m.upgrade_checked_at<=datetime('now','-6 hours'))
 AND NOT EXISTS(SELECT 1 FROM downloads d WHERE d.media_item_id=m.id AND d.episode_id IS NULL AND d.status NOT IN ('imported','failed','cancelled','seeding'))
 UNION ALL
 SELECT m.id,e.id,m.quality_profile_id,'series',m.title,COALESCE(m.imdb_id,''),COALESCE(e.file_path,''),COALESCE(m.year,0),se.number,e.number,e.absolute_number,COALESCE(e.upgrade_checked_at,'1970-01-01') AS checked
 FROM episodes e JOIN media_items m ON m.id=e.media_item_id JOIN seasons se ON se.id=e.season_id JOIN quality_profiles p ON p.id=m.quality_profile_id
 WHERE m.type='series' AND m.anime=1 AND e.status='available' AND e.episode_type='standard' AND p.upgrade_allowed=1
 AND json_extract(CASE WHEN json_valid(p.scoring_config) THEN p.scoring_config ELSE '{}' END,'$.preset')='sonarr-anime'
 AND (e.upgrade_checked_at IS NULL OR e.upgrade_checked_at<=datetime('now','-6 hours'))
 AND NOT EXISTS(SELECT 1 FROM downloads d WHERE d.episode_id=e.id AND d.status NOT IN ('imported','failed','cancelled','seeding'))
 ORDER BY checked,1,2 LIMIT ?`, upgradeBatchSize)
	if err != nil {
		slog.Warn("load quality upgrade candidates", "error", err)
		return
	}
	var targets []upgradeTarget
	for rows.Next() {
		var target upgradeTarget
		var checked string
		if err := rows.Scan(&target.mediaID, &target.episodeID, &target.profileID, &target.kind, &target.title, &target.imdbID, &target.path, &target.year, &target.season, &target.episode, &target.absolute, &checked); err != nil {
			rows.Close()
			slog.Warn("read quality upgrade candidate", "error", err)
			return
		}
		targets = append(targets, target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	for _, target := range targets {
		if s.ctx != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
			}
		}
		table, id := "media_items", target.mediaID
		if target.episodeID > 0 {
			table, id = "episodes", target.episodeID
		}
		claim, err := s.db.Exec("UPDATE "+table+" SET upgrade_checked_at=CURRENT_TIMESTAMP WHERE id=? AND status='available' AND (upgrade_checked_at IS NULL OR upgrade_checked_at<=datetime('now','-6 hours'))", id)
		if err != nil {
			continue
		}
		count, _ := claim.RowsAffected()
		if count != 1 {
			continue
		}
		s.searchQualityUpgrade(target)
	}
}

func (s *Scheduler) searchQualityUpgrade(target upgradeTarget) {
	profile := s.upgradeProfile(target)
	if profile == nil {
		return
	}
	if !upgradeFileExists(target) {
		return
	}
	if s.upgradeAwaitingImport(target) {
		return
	}
	currentTitle, err := s.currentReleaseTitle(target.mediaID, target.episodeID)
	if err != nil {
		return
	}
	if releaseForUpgrade(currentTitle, profile) == nil {
		return
	}
	releases := s.findUpgradeReleases(target)
	releases = indexer.FilterBlacklisted(releases, s.loadBlacklist(target.mediaID, target.episodeID))
	// The user may have imported another release while searches were running.
	// The atomic enqueue reservation below checks this baseline a second time.
	latest, err := s.currentReleaseTitle(target.mediaID, target.episodeID)
	if err != nil {
		return
	}
	profile = s.upgradeProfile(target)
	if profile == nil {
		return
	}
	latestRelease := releaseForUpgrade(latest, profile)
	if latestRelease == nil {
		return
	}
	var upgrades []indexer.Release
	for i := range releases {
		indexer.ScoreRelease(&releases[i], profile)
		if indexer.IsUpgrade(&releases[i], latestRelease, profile) {
			upgrades = append(upgrades, releases[i])
		}
	}
	// A higher-ranked release blocked by the quality cutoff must not hide a
	// permitted custom-format improvement within the current quality group.
	best := indexer.BestRelease(upgrades)
	if best == nil {
		return
	}
	if err := s.enqueueRelease(target.mediaID, target.episodeID, 0, *best, latest); err != nil {
		slog.Warn("enqueue quality upgrade", "media_id", target.mediaID, "episode_id", target.episodeID, "error", err)
	}
}

// A failed import is a local file problem, not evidence that another download
// is needed. Keep its source available for explicit retry/cancel.
func (s *Scheduler) upgradeAwaitingImport(target upgradeTarget) bool {
	var episodeID any
	if target.episodeID > 0 {
		episodeID = target.episodeID
	}
	rows, err := s.db.Query(`SELECT download_path FROM downloads WHERE media_item_id=? AND episode_id IS ? AND status='failed' AND upgrade_from_title IS NOT NULL AND download_path IS NOT NULL AND download_path!=''`, target.mediaID, episodeID)
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return true
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return rows.Err() != nil
}

func (s *Scheduler) upgradeProfile(target upgradeTarget) *indexer.QualityProfile {
	var profileID int
	var anime bool
	var kind string
	if err := s.db.QueryRow(`SELECT quality_profile_id,anime,type FROM media_items WHERE id=?`, target.mediaID).Scan(&profileID, &anime, &kind); err != nil || !anime || kind != target.kind {
		return nil
	}
	profile := s.loadProfile(profileID)
	if profile == nil || !profile.UpgradeAllowed {
		return nil
	}
	cfg, err := indexer.ResolveScoringConfig(profile.ScoringConfig)
	expected := "radarr-anime"
	if target.episodeID > 0 {
		expected = "sonarr-anime"
	}
	if err != nil || cfg.Preset != expected {
		return nil
	}
	return profile
}
func upgradeFileExists(target upgradeTarget) bool {
	if target.path == "" {
		return false
	}
	info, err := os.Stat(target.path)
	if err != nil {
		return false
	}
	if target.episodeID > 0 {
		return info.Mode().IsRegular()
	}
	if !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(target.path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && postprocess.IsVideoFile(entry.Name()) {
			info, err := os.Stat(filepath.Join(target.path, entry.Name()))
			if err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}
func releaseForUpgrade(title string, profile *indexer.QualityProfile) *indexer.Release {
	if strings.TrimSpace(title) == "" || title == "manual-grab" {
		return nil
	}
	parsed := indexer.ParseReleaseName(title)
	if parsed.Quality == "" {
		return nil
	}
	release := &indexer.Release{Title: title, Quality: parsed.Quality, Tags: parsed.Tags}
	indexer.ScoreRelease(release, profile)
	return release
}
func (s *Scheduler) currentReleaseTitle(mediaID, episodeID int) (string, error) {
	var title string
	var err error
	if episodeID > 0 {
		err = s.db.QueryRow(`SELECT COALESCE(NULLIF(e.current_release_title,''),(SELECT d.nzb_title FROM downloads d WHERE d.episode_id=e.id AND d.status IN ('imported','seeding') ORDER BY d.completed_at DESC,d.id DESC LIMIT 1),'') FROM episodes e WHERE e.id=? AND e.media_item_id=?`, episodeID, mediaID).Scan(&title)
	} else {
		err = s.db.QueryRow(`SELECT COALESCE(NULLIF(m.current_release_title,''),(SELECT d.nzb_title FROM downloads d WHERE d.media_item_id=m.id AND d.episode_id IS NULL AND d.status IN ('imported','seeding') ORDER BY d.completed_at DESC,d.id DESC LIMIT 1),'') FROM media_items m WHERE m.id=?`, mediaID).Scan(&title)
	}
	return title, err
}
func (s *Scheduler) findUpgradeReleases(target upgradeTarget) []indexer.Release {
	sources := s.loadIndexers()
	torrentSetting, _ := s.db.GetSetting("qbittorrent_enabled")
	torrentsEnabled := torrentSetting == "true" && s.rtClient != nil && s.qbt != nil
	var releases []indexer.Release
	if target.kind == "movie" {
		releases = s.newznab.SearchMovie(indexer.MovieIndexers(sources, "newznab", true), target.imdbID, target.title, target.year)
		for i := range releases {
			releases[i].DownloadType = "nzb"
		}
		if torrentsEnabled {
			query := target.title
			if target.year > 0 {
				query = fmt.Sprintf("%s %d", query, target.year)
			}
			torrents := s.searchTorrentReleases(indexer.MovieIndexers(sources, "rutracker", true), []string{query}, "anime")
			releases = append(releases, indexer.FilterMovieReleases(torrents, target.title, target.year)...)
		}
	} else {
		nzbs := filterIndexersByType(sources, "newznab", "anime")
		if target.absolute.Valid {
			releases = s.newznab.SearchAnimeEpisode(nzbs, []string{target.title}, int(target.absolute.Int64), target.season, target.episode)
		} else {
			releases = s.newznab.SearchEpisode(nzbs, 0, target.season, target.episode, target.title)
		}
		for i := range releases {
			releases[i].DownloadType = "nzb"
		}
		if torrentsEnabled {
			torrents := s.findTorrentEpisodes(filterIndexersByType(sources, "rutracker", "anime"), target.title, "anime", target.season, target.episode, target.absolute)
			releases = append(releases, torrents...)
		}
	}
	return indexer.DeduplicateReleases(releases)
}
