package postprocess

import (
	"mediaforge/internal/database"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func writeMedia(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func execute(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func seedSeries(t *testing.T, db *database.DB) {
	execute(t, db, `INSERT INTO media_items(id,type,title,year) VALUES (1,'series','Example',2024)`)
	execute(t, db, `INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
	execute(t, db, `INSERT INTO episodes(id,season_id,media_item_id,number,title) VALUES(1,1,1,1,'Pilot'),(2,1,1,2,'Second')`)
}
func TestSeasonPackImportsEveryEpisodeAndPreservesExtras(t *testing.T) {
	db := testDB(t)
	seedSeries(t, db)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status) VALUES(1,1,'pack','completed')`)
	source := t.TempDir()
	root := t.TempDir()
	writeMedia(t, filepath.Join(source, "Example.S01E01.mkv"), "first")
	writeMedia(t, filepath.Join(source, "Example.S01E02.mkv"), "second")
	extra := filepath.Join(source, "unrelated.txt")
	writeMedia(t, extra, "keep")
	if err := New(db, root, "").Process(1, source); err != nil {
		t.Fatal(err)
	}
	var available int
	db.QueryRow(`SELECT COUNT(*) FROM episodes WHERE status='available'`).Scan(&available)
	if available != 2 {
		t.Fatalf("imported %d episodes", available)
	}
	if err := CleanDownloadDir(source); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatal("cleanup deleted unrelated files")
	}
}
func TestUnmatchedPackFailsWithoutDeletingSource(t *testing.T) {
	db := testDB(t)
	seedSeries(t, db)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status) VALUES(1,1,'pack','completed')`)
	src := filepath.Join(t.TempDir(), "unrecognizable.mkv")
	writeMedia(t, src, "important")
	if err := New(db, t.TempDir(), "").Process(1, filepath.Dir(src)); err == nil {
		t.Fatal("unmatched pack falsely succeeded")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source deleted")
	}
}
func TestSingleEpisodePicksMatchingFileNotLargest(t *testing.T) {
	db := testDB(t)
	seedSeries(t, db)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,status) VALUES(1,1,1,'episode','completed')`)
	src := t.TempDir()
	writeMedia(t, filepath.Join(src, "Example.S01E01.mkv"), "right")
	wrong := filepath.Join(src, "Example.S01E02.mkv")
	writeMedia(t, wrong, strings.Repeat("wrong", 100))
	if err := New(db, t.TempDir(), "").Process(1, src); err != nil {
		t.Fatal(err)
	}
	var path string
	db.QueryRow(`SELECT file_path FROM episodes WHERE id=1`).Scan(&path)
	body, _ := os.ReadFile(path)
	if string(body) != "right" {
		t.Fatalf("imported wrong episode: %s", body)
	}
	if _, err := os.Stat(wrong); err != nil {
		t.Fatal("deleted unimported episode")
	}
}
func TestTorrentRetryNeverTruncatesHardlinkedSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library.mkv")
	writeMedia(t, src, "precious torrent")
	if err := hardlinkFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := hardlinkFile(src, dst); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(src)
	if string(body) != "precious torrent" {
		t.Fatalf("source corrupted: %q", body)
	}
	other := filepath.Join(dir, "other.mkv")
	writeMedia(t, other, "replacement")
	if err := hardlinkFile(other, dst); err == nil {
		t.Fatal("overwrote existing library inode")
	}
	body, _ = os.ReadFile(src)
	if string(body) != "precious torrent" {
		t.Fatal("original hardlink overwritten")
	}
}
func TestMultiDiscAlbumUsesEachTrackDiscAndRealAlbumRoot(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO artists(id,name) VALUES(1,'Artist')`)
	execute(t, db, `INSERT INTO albums(id,artist_id,title,year) VALUES(1,1,'Album',2024)`)
	execute(t, db, `INSERT INTO tracks(id,album_id,number,disc_number,title) VALUES(1,1,1,1,'Song'),(2,1,1,2,'Song')`)
	execute(t, db, `INSERT INTO downloads(id,album_id,nzb_title,status,download_type) VALUES(1,1,'album','completed','torrent')`)
	src := t.TempDir()
	first := filepath.Join(src, "CD1", "01 - Song.flac")
	second := filepath.Join(src, "CD2", "01 - Song.flac")
	writeMedia(t, first, "disc1")
	writeMedia(t, second, "disc2")
	if err := New(db, t.TempDir(), "").ProcessTorrent(1, src); err != nil {
		t.Fatal(err)
	}
	var p1, p2, root, status string
	db.QueryRow(`SELECT file_path FROM tracks WHERE id=1`).Scan(&p1)
	db.QueryRow(`SELECT file_path FROM tracks WHERE id=2`).Scan(&p2)
	db.QueryRow(`SELECT root_path FROM albums WHERE id=1`).Scan(&root)
	db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status)
	if p1 == p2 || filepath.Dir(p1) != root || !strings.Contains(root, "2024") || status != "seeding" {
		t.Fatalf("bad import paths/status: %q %q %q %q", p1, p2, root, status)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("torrent source deleted")
	}
}
func TestUnknownAlbumTrackFails(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO artists(id,name) VALUES(1,'Artist')`)
	execute(t, db, `INSERT INTO albums(id,artist_id,title) VALUES(1,1,'Album')`)
	execute(t, db, `INSERT INTO tracks(album_id,number,title) VALUES(1,1,'Song')`)
	execute(t, db, `INSERT INTO downloads(id,album_id,nzb_title) VALUES(1,1,'album')`)
	src := filepath.Join(t.TempDir(), "Unknown.flac")
	writeMedia(t, src, "keep")
	if err := New(db, t.TempDir(), "").Process(1, filepath.Dir(src)); err == nil {
		t.Fatal("unmatched album incorrectly imported")
	}
}
func TestMissingPathAndFilenameTraversal(t *testing.T) {
	if _, err := findVideoFiles(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory scan swallowed error")
	}
	root := t.TempDir()
	path := AnimeEpisodePath(root, "..", 1, 1, 1, "..", ".mkv")
	rel, _ := filepath.Rel(root, path)
	if strings.HasPrefix(rel, "..") {
		t.Fatal("title escaped media root")
	}
}

func TestImportRejectsSymlinkedDestination(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "movies")); err != nil {
		t.Skip("symlink permission unavailable")
	}
	if err := validateDestination(root, filepath.Join(root, "movies", "Movie", "Movie.mkv")); err == nil {
		t.Fatal("import followed symlink outside library")
	}
}

func TestFailedImportRollsBackCreatedLibraryFiles(t *testing.T) {
	db := testDB(t)
	seedSeries(t, db)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status) VALUES(1,1,'pack','completed')`)
	src := t.TempDir()
	root := t.TempDir()
	writeMedia(t, filepath.Join(src, "Example.S01E01.mkv"), "first")
	writeMedia(t, filepath.Join(src, "Example.S01E02.mkv"), "second")
	occupied := SeriesEpisodePath(root, "Example", 2024, 1, 2, "Second", ".mkv")
	writeMedia(t, occupied, "untracked file")
	if err := New(db, root, "").Process(1, src); err == nil {
		t.Fatal("unexpected overwrite")
	}
	first := SeriesEpisodePath(root, "Example", 2024, 1, 1, "Pilot", ".mkv")
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("partial import destination was not rolled back")
	}
	body, _ := os.ReadFile(occupied)
	if string(body) != "untracked file" {
		t.Fatal("existing destination changed")
	}
}
func TestTrackedUpgradePreservesOldSeedingInode(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO media_items(id,type,title,year) VALUES(1,'movie','Movie',2024)`)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,download_type) VALUES(1,1,'first','completed','torrent'),(2,1,'upgrade','completed','torrent')`)
	root := t.TempDir()
	p := New(db, root, "")
	oldSource := filepath.Join(t.TempDir(), "old.mkv")
	newSource := filepath.Join(t.TempDir(), "new.mkv")
	writeMedia(t, oldSource, "old torrent")
	writeMedia(t, newSource, "new torrent")
	if err := p.ProcessTorrent(1, oldSource); err != nil {
		t.Fatal(err)
	}
	if err := p.ProcessTorrent(2, newSource); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(oldSource)
	if string(body) != "old torrent" {
		t.Fatal("old seeding data was overwritten")
	}
	body, _ = os.ReadFile(MoviePath(root, "Movie", 2024, ".mkv"))
	if string(body) != "new torrent" {
		t.Fatal("upgrade not installed")
	}
}

func TestDatabaseFailureRestoresPreviousLibraryVersion(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO media_items(id,type,title,year) VALUES(1,'movie','Movie',2024)`)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status,download_type) VALUES(1,1,'first','completed','torrent'),(2,1,'upgrade','completed','torrent')`)
	root := t.TempDir()
	p := New(db, root, "")
	oldSource := filepath.Join(t.TempDir(), "old.mkv")
	newSource := filepath.Join(t.TempDir(), "new.mkv")
	writeMedia(t, oldSource, "old torrent")
	writeMedia(t, newSource, "new torrent")
	if err := p.ProcessTorrent(1, oldSource); err != nil {
		t.Fatal(err)
	}
	execute(t, db, `CREATE TRIGGER fail_import BEFORE UPDATE ON downloads WHEN NEW.id=2 BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	if err := p.ProcessTorrent(2, newSource); err == nil {
		t.Fatal("database failure ignored")
	}
	body, _ := os.ReadFile(MoviePath(root, "Movie", 2024, ".mkv"))
	if string(body) != "old torrent" {
		t.Fatal("previous version was not restored")
	}
	body, _ = os.ReadFile(newSource)
	if string(body) != "new torrent" {
		t.Fatal("upgrade source was lost")
	}
}

func TestIncompleteAlbumRetainsSourceForRetry(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO artists(id,name) VALUES(1,'Artist')`)
	execute(t, db, `INSERT INTO albums(id,artist_id,title,status) VALUES(1,1,'Album','downloading')`)
	execute(t, db, `INSERT INTO tracks(album_id,number,title) VALUES(1,1,'One'),(1,2,'Two')`)
	execute(t, db, `INSERT INTO downloads(id,album_id,nzb_title) VALUES(1,1,'album')`)
	src := filepath.Join(t.TempDir(), "01 - One.flac")
	writeMedia(t, src, "track1")
	if err := New(db, t.TempDir(), "").Process(1, filepath.Dir(src)); err == nil {
		t.Fatal("incomplete album silently imported")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("incomplete album source deleted")
	}
	var status string
	db.QueryRow(`SELECT status FROM albums WHERE id=1`).Scan(&status)
	if status != "downloading" {
		t.Fatal("incomplete album reset for automatic duplicate grab")
	}
}

func TestAnimeMovieImportsAsMovieBesideAnimeSeries(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO media_items(id,type,title,year,anime) VALUES(1,'series','Chainsaw Man',2022,1),(2,'movie','Chainsaw Man The Movie Reze Arc',2025,1)`)
	execute(t, db, `INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
	execute(t, db, `INSERT INTO episodes(id,season_id,media_item_id,number,absolute_number,title) VALUES(1,1,1,1,1,'Dog and Chainsaw')`)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status) VALUES(1,2,'Reze Arc','completed')`)
	src := filepath.Join(t.TempDir(), "Chainsaw.Man.The.Movie.Reze.Arc.2025.1080p.mkv")
	writeMedia(t, src, "movie")
	root := t.TempDir()
	if err := New(db, root, "").Process(1, src); err != nil {
		t.Fatal(err)
	}
	var movieRoot, episodeStatus string
	db.QueryRow(`SELECT root_path FROM media_items WHERE id=2`).Scan(&movieRoot)
	db.QueryRow(`SELECT status FROM episodes WHERE id=1`).Scan(&episodeStatus)
	if !strings.Contains(movieRoot, string(filepath.Separator)+"movies"+string(filepath.Separator)) || episodeStatus != "wanted" {
		t.Fatalf("movie root=%s series episode=%s", movieRoot, episodeStatus)
	}
}
func TestAnimeEpisodeImportRejectsRezeArcMovie(t *testing.T) {
	db := testDB(t)
	execute(t, db, `INSERT INTO media_items(id,type,title,year,anime) VALUES(1,'series','Chainsaw Man',2022,1)`)
	execute(t, db, `INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
	execute(t, db, `INSERT INTO episodes(id,season_id,media_item_id,number,absolute_number,title) VALUES(1,1,1,1,1,'Dog and Chainsaw')`)
	execute(t, db, `INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,status) VALUES(1,1,1,'wrong release','completed')`)
	src := filepath.Join(t.TempDir(), "Chainsaw.Man.The.Movie.Reze.Arc.2025.1080p.DDP5.1.mkv")
	writeMedia(t, src, "keep movie")
	if err := New(db, t.TempDir(), "").Process(1, src); err == nil {
		t.Fatal("movie imported as episode 1")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("mismatched movie source removed")
	}
}
func TestMovieImportRejectsEpisodeAndUntrackedLegacyRoot(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	destination := MoviePath(root, "Movie", 2025, ".mkv")
	execute(t, db, `INSERT INTO media_items(id,type,title,year,root_path) VALUES(1,'movie','Movie',2025,?)`, filepath.Dir(destination))
	execute(t, db, `INSERT INTO downloads(id,media_item_id,nzb_title,status) VALUES(1,1,'movie','completed')`)
	source := filepath.Join(t.TempDir(), "Movie.S01E01.mkv")
	writeMedia(t, source, "episode")
	p := New(db, root, "")
	if err := p.Process(1, source); err == nil {
		t.Fatal("episode accepted for movie")
	}
	writeMedia(t, destination, "untracked canonical file")
	source = filepath.Join(t.TempDir(), "Movie.2025.mkv")
	writeMedia(t, source, "replacement")
	if err := p.Process(1, source); err == nil {
		t.Fatal("legacy root_path incorrectly proved tracked ownership")
	}
	body, _ := os.ReadFile(destination)
	if string(body) != "untracked canonical file" {
		t.Fatal("untracked file overwritten")
	}
}

func TestEpisodeRangeCannotUseSingleFileFallback(t *testing.T) {
	for _, name := range []string{"Chainsaw Man - 01-02.mkv", "Chainsaw Man - 01v2-03.mkv", "Chainsaw Man S01E01E02.mkv", "Chainsaw Man 1x01-02.mkv"} {
		t.Run(name, func(t *testing.T) {
			db := testDB(t)
			execute(t, db, `INSERT INTO media_items(id,type,title,anime) VALUES(1,'series','Chainsaw Man',1)`)
			execute(t, db, `INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
			execute(t, db, `INSERT INTO episodes(id,season_id,media_item_id,number,absolute_number) VALUES(1,1,1,1,1)`)
			execute(t, db, `INSERT INTO downloads(id,media_item_id,episode_id,nzb_title) VALUES(1,1,1,'pack')`)
			src := filepath.Join(t.TempDir(), name)
			writeMedia(t, src, "retain combined episodes")
			if err := New(db, t.TempDir(), "").Process(1, src); err == nil {
				t.Fatal("combined episodes incorrectly assigned to episode1")
			}
			if _, err := os.Stat(src); err != nil {
				t.Fatal("combined source lost")
			}
		})
	}
}
