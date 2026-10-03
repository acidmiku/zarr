package postprocess

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mediaforge/internal/indexer"
)

// Preserve an explicit unknown marker: NULL would revive an older successful
// download as the baseline after a manual import of unknown provenance.
func importedReleaseTitle(title string) string {
	if strings.TrimSpace(title) == "" || indexer.ParseReleaseName(title).Quality == "" {
		return "manual-grab"
	}
	return title
}

// Automatic downloads are re-evaluated against the current profile and file at
// import time. A better manual import or changed profile can invalidate a job
// that was an upgrade when it was queued.
type upgradeReader interface {
	QueryRow(string, ...any) *sql.Row
}

func (p *Processor) validateUpgrade(mediaID, episodeID int, candidateTitle string) error {
	return validateUpgrade(p.db, mediaID, episodeID, candidateTitle)
}

func validateUpgrade(db upgradeReader, mediaID, episodeID int, candidateTitle string) error {
	var profile indexer.QualityProfile
	var anime bool
	var kind string
	err := db.QueryRow(`SELECT p.id,p.name,p.qualities,p.tags,p.language,p.reject_patterns,p.upgrade_allowed,p.profile_type,p.scoring_config,m.anime,m.type
 FROM media_items m JOIN quality_profiles p ON p.id=m.quality_profile_id WHERE m.id=?`, mediaID).Scan(
		&profile.ID, &profile.Name, &profile.Qualities, &profile.Tags, &profile.Language, &profile.RejectPatterns,
		&profile.UpgradeAllowed, &profile.ProfileType, &profile.ScoringConfig, &anime, &kind)
	if err != nil {
		return fmt.Errorf("automatic upgrade profile is no longer available: %w", err)
	}
	cfg, err := indexer.ResolveScoringConfig(profile.ScoringConfig)
	expected := "radarr-anime"
	if episodeID > 0 {
		expected = "sonarr-anime"
	}
	if err != nil || !anime || !profile.UpgradeAllowed || cfg.Preset != expected || (episodeID == 0 && kind != "movie") || (episodeID > 0 && kind != "series") {
		return fmt.Errorf("automatic upgrades are no longer enabled for this item")
	}
	var currentTitle, path, status string
	if episodeID > 0 {
		err = db.QueryRow(`SELECT COALESCE(NULLIF(e.current_release_title,''),(SELECT d.nzb_title FROM downloads d WHERE d.episode_id=e.id AND d.status IN ('imported','seeding') ORDER BY d.completed_at DESC,d.id DESC LIMIT 1),''),COALESCE(e.file_path,''),e.status FROM episodes e WHERE e.id=? AND e.media_item_id=?`, episodeID, mediaID).Scan(&currentTitle, &path, &status)
	} else {
		err = db.QueryRow(`SELECT COALESCE(NULLIF(m.current_release_title,''),(SELECT d.nzb_title FROM downloads d WHERE d.media_item_id=m.id AND d.episode_id IS NULL AND d.status IN ('imported','seeding') ORDER BY d.completed_at DESC,d.id DESC LIMIT 1),''),COALESCE(m.root_path,''),m.status FROM media_items m WHERE m.id=?`, mediaID).Scan(&currentTitle, &path, &status)
	}
	if err != nil || status != "available" || path == "" {
		return fmt.Errorf("automatic upgrade no longer has an available baseline")
	}
	info, err := os.Stat(path)
	if err != nil || (episodeID > 0 && !info.Mode().IsRegular()) || (episodeID == 0 && !info.IsDir()) {
		return fmt.Errorf("automatic upgrade baseline file is missing")
	}
	currentParsed, candidateParsed := indexer.ParseReleaseName(currentTitle), indexer.ParseReleaseName(candidateTitle)
	if currentParsed.Quality == "" || candidateParsed.Quality == "" {
		return fmt.Errorf("automatic upgrade quality cannot be established")
	}
	current := indexer.Release{Title: currentTitle, Quality: currentParsed.Quality, Tags: currentParsed.Tags}
	candidate := indexer.Release{Title: candidateTitle, Quality: candidateParsed.Quality, Tags: candidateParsed.Tags}
	indexer.ScoreRelease(&current, &profile)
	indexer.ScoreRelease(&candidate, &profile)
	if !indexer.IsUpgrade(&candidate, &current, &profile) {
		return fmt.Errorf("download is no longer an upgrade to the current library file")
	}
	return nil
}

// Movies have a directory path in the database. Only a single canonical movie
// filename is eligible for retirement; bonus videos or ambiguous files stay.
func previousMovieFile(dst string) string {
	entries, err := os.ReadDir(filepath.Dir(dst))
	if err != nil {
		return ""
	}
	stem := strings.TrimSuffix(filepath.Base(dst), filepath.Ext(dst))
	var previous string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !IsVideoFile(entry.Name()) || strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())) != stem {
			continue
		}
		if previous != "" {
			return ""
		}
		previous = filepath.Join(filepath.Dir(dst), entry.Name())
	}
	return previous
}
