package postprocess

import (
	"os"
	"path/filepath"
	"testing"

	"mediaforge/internal/database"
)

const oldMovieRelease = "[SubsPlease] Movie.2024.720p.WEB-DL"
const newMovieRelease = "[SubsPlease] Movie.2024.1080p.WEB-DL"

func seedImportedAnimeMovie(t *testing.T) (*database.DB, *Processor, string, string) {
	t.Helper()
	db := testDB(t)
	root := t.TempDir()
	execute(t, db, `INSERT INTO media_items(id,type,title,year,anime,quality_profile_id) VALUES(1,'movie','Movie',2024,1,(SELECT id FROM quality_profiles WHERE scoring_config='{"preset":"radarr-anime"}'))`)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,download_type) VALUES(1,1,?,'completed','torrent')`, oldMovieRelease)
	oldSource := filepath.Join(t.TempDir(), "original.mp4")
	writeMedia(t, oldSource, "old torrent inode")
	p := New(db, root, "")
	if err := p.ProcessTorrent(1, oldSource); err != nil {
		t.Fatal(err)
	}
	return db, p, root, oldSource
}

func TestAutomaticMovieUpgradeChangesContainerAfterCommitAndKeepsSeedingFile(t *testing.T) {
	db, p, root, oldSource := seedImportedAnimeMovie(t)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,download_type,upgrade_from_title) VALUES(2,1,?,'completed','torrent',?)`, newMovieRelease, oldMovieRelease)
	src := filepath.Join(t.TempDir(), "new.mkv")
	writeMedia(t, src, "upgraded file")
	if err := p.ProcessTorrent(2, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(MoviePath(root, "Movie", 2024, ".mp4")); !os.IsNotExist(err) {
		t.Fatal("old container left in library after successful upgrade")
	}
	for path, want := range map[string]string{oldSource: "old torrent inode", src: "upgraded file", MoviePath(root, "Movie", 2024, ".mkv"): "upgraded file"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("file %s changed or lost", path)
		}
	}
	var title, status string
	db.QueryRow(`SELECT current_release_title,status FROM media_items WHERE id=1`).Scan(&title, &status)
	if title != newMovieRelease || status != "available" {
		t.Fatalf("current title=%s status=%s", title, status)
	}
}

func TestAutomaticUpgradeFailurePreservesOldContainerAndBaseline(t *testing.T) {
	db, p, root, _ := seedImportedAnimeMovie(t)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,upgrade_from_title) VALUES(2,1,?,'completed',?)`, newMovieRelease, oldMovieRelease)
	execute(t, db, `CREATE TRIGGER reject_upgrade BEFORE UPDATE ON downloads WHEN NEW.id=2 BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	src := filepath.Join(t.TempDir(), "new.mkv")
	writeMedia(t, src, "upgrade source")
	if err := p.Process(2, src); err == nil {
		t.Fatal("DB failure was ignored")
	}
	if got, err := os.ReadFile(MoviePath(root, "Movie", 2024, ".mp4")); err != nil || string(got) != "old torrent inode" {
		t.Fatal("old file lost on failure")
	}
	if _, err := os.Stat(MoviePath(root, "Movie", 2024, ".mkv")); !os.IsNotExist(err) {
		t.Fatal("uncommitted new file remained")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("failed source lost")
	}
	var title, status string
	db.QueryRow(`SELECT current_release_title,status FROM media_items WHERE id=1`).Scan(&title, &status)
	if title != oldMovieRelease || status != "available" {
		t.Fatalf("failed upgrade changed baseline=%s status=%s", title, status)
	}
}

func TestSameExtensionAutomaticUpgradeRollsBackWithoutChangingOldTorrentInode(t *testing.T) {
	db, p, root, oldSource := seedImportedAnimeMovie(t)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,upgrade_from_title) VALUES(2,1,?,'completed',?)`, newMovieRelease, oldMovieRelease)
	execute(t, db, `CREATE TRIGGER reject_upgrade BEFORE UPDATE ON downloads WHEN NEW.id=2 BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	src := filepath.Join(t.TempDir(), "upgrade.mp4")
	writeMedia(t, src, "replacement")
	if err := p.Process(2, src); err == nil {
		t.Fatal("failed transaction accepted")
	}
	dst := MoviePath(root, "Movie", 2024, ".mp4")
	for _, path := range []string{dst, oldSource} {
		if body, err := os.ReadFile(path); err != nil || string(body) != "old torrent inode" {
			t.Fatalf("old library/torrent contents changed at %s", path)
		}
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("replacement source lost")
	}
	for _, pattern := range []string{".zarr-new-*", ".zarr-backup-*"} {
		matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dst), pattern))
		if len(matches) != 0 {
			t.Fatal("temporary upgrade file left after rollback")
		}
	}
}

func TestAutomaticUpgradeRechecksCurrentReleaseAndProfile(t *testing.T) {
	for _, test := range []struct{ name, update string }{
		{"better manual import", `UPDATE media_items SET current_release_title='[SubsPlease] Movie.2024.1080p.BluRay' WHERE id=1`},
		{"unknown manual import", `UPDATE media_items SET current_release_title='manual-grab' WHERE id=1`},
		{"disabled upgrades", `UPDATE quality_profiles SET upgrade_allowed=0`},
		{"legacy scoring", `UPDATE quality_profiles SET scoring_config='{}'`},
		{"cancelled", `UPDATE downloads SET status='cancelled' WHERE id=2`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, p, root, _ := seedImportedAnimeMovie(t)
			execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,upgrade_from_title) VALUES(2,1,?,'completed',?)`, newMovieRelease, oldMovieRelease)
			execute(t, db, test.update)
			src := filepath.Join(t.TempDir(), "new.mp4")
			writeMedia(t, src, "now unwanted source")
			if err := p.Process(2, src); err == nil {
				t.Fatal("stale automatic download replaced current file")
			}
			if got, err := os.ReadFile(MoviePath(root, "Movie", 2024, ".mp4")); err != nil || string(got) != "old torrent inode" {
				t.Fatal("current file changed")
			}
			if _, err := os.Stat(src); err != nil {
				t.Fatal("rejected source deleted")
			}
		})
	}
}

func TestEpisodeUpgradePersistsReleaseAndReplacesOldExtension(t *testing.T) {
	db := testDB(t)
	seedSeries(t, db)
	execute(t, db, `UPDATE media_items SET anime=1,quality_profile_id=(SELECT id FROM quality_profiles WHERE scoring_config='{"preset":"sonarr-anime"}') WHERE id=1`)
	oldTitle, newTitle := "[SubsPlease] Example.S01E01.720p.WEB-DL", "[SubsPlease] Example.S01E01.1080p.WEB-DL"
	execute(t, db, `INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,status) VALUES(1,1,1,?,'completed')`, oldTitle)
	p := New(db, t.TempDir(), "")
	oldSource := filepath.Join(t.TempDir(), "Example.S01E01.mp4")
	writeMedia(t, oldSource, "episode old")
	if err := p.Process(1, oldSource); err != nil {
		t.Fatal(err)
	}
	var oldPath string
	db.QueryRow(`SELECT file_path FROM episodes WHERE id=1`).Scan(&oldPath)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,status,upgrade_from_title) VALUES(2,1,1,?,'completed',?)`, newTitle, oldTitle)
	src := filepath.Join(t.TempDir(), "Example.S01E01.mkv")
	writeMedia(t, src, "episode upgrade")
	if err := p.Process(2, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old episode path remains after successful commit")
	}
	var title, status, newPath string
	db.QueryRow(`SELECT current_release_title,status,file_path FROM episodes WHERE id=1`).Scan(&title, &status, &newPath)
	if title != newTitle || status != "available" || filepath.Ext(newPath) != ".mkv" {
		t.Fatalf("episode metadata failed: %s %s %s", title, status, newPath)
	}
}
