package postprocess

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type MusicImportTrack struct {
	RecordingID, Title             string
	Number, DiscNumber, DurationMS int
}
type MusicImport struct {
	ArtistMBID, ArtistName, ArtistSortName, ReleaseGroupID, ReleaseID string
	Title, EditionTitle, EditionDisambiguation, AlbumType, ImageURL   string
	Year, QualityProfileID                                            int
	Tracks                                                            []MusicImportTrack
}

// ImportMusic validates every source against verified metadata before creating
// files or library rows. The database changes commit together; staged files roll
// back on any error. Source files are removed only after that commit.
func (p *Processor) ImportMusic(source string, input MusicImport) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(source) == "" || strings.TrimSpace(p.mediaRoot) == "" {
		return 0, fmt.Errorf("source and media root are required")
	}
	if input.ArtistMBID == "" || input.ArtistName == "" || input.ReleaseGroupID == "" || input.ReleaseID == "" || input.Title == "" || len(input.Tracks) == 0 {
		return 0, fmt.Errorf("complete verified album metadata is required")
	}
	files, err := findAudioFiles(source)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("no audio files found")
	}
	album := albumIdentity{artist: input.ArtistName, title: input.Title, editionTitle: input.EditionTitle, edition: input.EditionDisambiguation, artistID: input.ArtistMBID, releaseID: input.ReleaseID, groupID: input.ReleaseGroupID}
	tracks := make([]albumTrack, 0, len(input.Tracks))
	for i, t := range input.Tracks {
		if t.Title == "" || t.Number <= 0 || t.DiscNumber <= 0 {
			return 0, fmt.Errorf("invalid album tracklist")
		}
		tracks = append(tracks, albumTrack{id: i + 1, number: t.Number, disc: t.DiscNumber, title: t.Title, recordingID: t.RecordingID})
	}
	var qualities string
	if input.QualityProfileID > 0 {
		if err := p.db.QueryRow(`SELECT qualities FROM quality_profiles WHERE id=? AND profile_type='music'`, input.QualityProfileID).Scan(&qualities); err != nil {
			return 0, fmt.Errorf("music quality profile is unavailable")
		}
	}
	plan, err := p.planAlbumFiles(album, input.Year, tracks, files, qualities)
	if err != nil {
		return 0, err
	}
	if len(plan) != len(tracks) {
		return 0, fmt.Errorf("complete album source is required")
	}
	paths := map[int]string{}
	seen := map[string]bool{}
	for _, f := range plan {
		if seen[f.dst] {
			return 0, fmt.Errorf("multiple tracks target the same path")
		}
		seen[f.dst] = true
		if err := validateDestination(p.mediaRoot, f.dst); err != nil {
			return 0, err
		}
		paths[f.trackID] = f.dst
	}
	var staged []stagedFile
	committed := false
	defer func() {
		if !committed {
			for i := len(staged) - 1; i >= 0; i-- {
				staged[i].rollback()
			}
		}
	}()
	for _, f := range plan {
		installed, err := stageFile(f.src, f.dst, false)
		if err != nil {
			return 0, err
		}
		staged = append(staged, installed)
	}
	tx, err := p.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var artistID int
	err = tx.QueryRow(`SELECT id FROM artists WHERE mbid=?`, input.ArtistMBID).Scan(&artistID)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(`INSERT INTO artists(mbid,name,sort_name) VALUES(?,?,?)`, input.ArtistMBID, input.ArtistName, input.ArtistSortName)
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		artistID = int(id)
	} else if err != nil {
		return 0, err
	}
	var profile any
	if input.QualityProfileID > 0 {
		profile = input.QualityProfileID
	}
	res, err := tx.Exec(`INSERT INTO albums(artist_id,mbid,release_group_id,title,year,album_type,quality_profile_id,image_url,status,root_path,track_count,edition_title,edition_disambiguation,monitored) VALUES(?,?,?,?,?,?,?,?,'available',?,?,?,?,0)`, artistID, input.ReleaseID, input.ReleaseGroupID, input.Title, input.Year, input.AlbumType, profile, input.ImageURL, filepath.Dir(plan[0].dst), len(tracks), input.EditionTitle, input.EditionDisambiguation)
	if err != nil {
		return 0, err
	}
	albumID, _ := res.LastInsertId()
	for i, t := range input.Tracks {
		if _, err := tx.Exec(`INSERT INTO tracks(album_id,mbid,title,number,disc_number,duration_ms,status,file_path) VALUES(?,?,?,?,?,?,'available',?)`, albumID, t.RecordingID, t.Title, t.Number, t.DiscNumber, t.DurationMS, paths[i+1]); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO activity_log(album_id,action,details) VALUES(?,'imported',?)`, albumID, fmt.Sprintf("Imported %d validated tracks", len(tracks))); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	for _, file := range staged {
		file.commit()
	}
	p.SaveCoverToAlbumDir(int(albumID), filepath.Dir(plan[0].dst), source)
	for _, f := range plan {
		if filepath.Clean(f.src) != filepath.Clean(f.dst) {
			if err := os.Remove(f.src); err != nil {
				slog.Warn("could not remove imported music source", "path", f.src, "error", err)
			}
		}
	}
	return int(albumID), nil
}
