package scheduler

import (
	"encoding/json"
	"io"
	"mediaforge/internal/database"
	"mediaforge/internal/indexer"
	"mediaforge/internal/postprocess"
	"mediaforge/internal/qbt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testSchedulerDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func TestFailedTorrentImportRetainsSourceAndNeverSeedCleans(t *testing.T) {
	db := testSchedulerDB(t)
	db.SetSetting("qbittorrent_enabled", "true")
	db.SetSetting("torrent_seed_time_hours", "0")
	db.Exec(`INSERT INTO media_items(id,type,title,year) VALUES(1,'movie','Movie',2024)`)
	db.Exec(`INSERT INTO downloads(id,media_item_id,nzb_title,download_type,status) VALUES(1,1,'torrent','torrent','downloading')`)
	source := t.TempDir()
	keep := filepath.Join(source, "unrecognized.txt")
	os.WriteFile(keep, []byte("keep"), 0644)
	deletes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/delete" {
			deletes++
			return
		}
		json.NewEncoder(w).Encode([]qbt.Torrent{{Hash: "hash", Tags: "dl_1", Progress: 1, State: "stoppedUP", ContentPath: source}})
	}))
	defer srv.Close()
	s := &Scheduler{db: db, qbt: qbt.New(srv.Client(), srv.URL, "", ""), processor: postprocess.New(db, t.TempDir(), "")}
	s.pollTorrentDownloads()
	s.cleanupSeededTorrents()
	var status string
	db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status)
	if status != "failed" || deletes != 0 {
		t.Fatalf("failed import was cleaned: status=%s deletes=%d", status, deletes)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("source removed")
	}
}
func TestCancelledTorrentNeverImportsAndImportedTorrentDoesNotRegress(t *testing.T) {
	db := testSchedulerDB(t)
	db.SetSetting("qbittorrent_enabled", "true")
	db.Exec(`INSERT INTO media_items(id,type,title) VALUES(1,'movie','Movie')`)
	db.Exec(`INSERT INTO downloads(id,media_item_id,nzb_title,download_type,status) VALUES(1,1,'cancelled','torrent','cancelled'),(2,1,'done','torrent','imported')`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]qbt.Torrent{{Tags: "dl_1", Progress: 1, State: "uploading"}, {Tags: "dl_2", Progress: 1, State: "error"}})
	}))
	defer srv.Close()
	s := &Scheduler{db: db, qbt: qbt.New(srv.Client(), srv.URL, "", "")}
	s.pollTorrentDownloads()
	for id, want := range map[int]string{1: "cancelled", 2: "imported"} {
		var got string
		db.QueryRow(`SELECT status FROM downloads WHERE id=?`, id).Scan(&got)
		if got != want {
			t.Fatalf("download %d regressed to %s", id, got)
		}
	}
}
func TestCleanupMarksImportedOnlyAfterClientDeletion(t *testing.T) {
	db := testSchedulerDB(t)
	db.SetSetting("qbittorrent_enabled", "true")
	db.SetSetting("torrent_seed_time_hours", "0")
	db.Exec(`INSERT INTO media_items(id,type,title) VALUES(1,'movie','Movie')`)
	db.Exec(`INSERT INTO downloads(id,media_item_id,nzb_title,download_type,status,qbt_hash,completed_at_ts) VALUES(1,1,'done','torrent','seeding','hash',?)`, time.Now().Unix()-100)
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	s := &Scheduler{db: db, qbt: qbt.New(srv.Client(), srv.URL, "", "")}
	s.cleanupSeededTorrents()
	var status string
	db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status)
	if status != "seeding" {
		t.Fatal(status)
	}
	fail = false
	s.cleanupSeededTorrents()
	db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status)
	if status != "imported" {
		t.Fatal(status)
	}
}
func TestExtractDownloadIDRequiresExactPositiveTag(t *testing.T) {
	for input, want := range map[string]int{"dl_42": 42, "other, dl_7": 7, "dl_9garbage": 0, "dl_-1": 0, "dl_0": 0} {
		if got := extractDownloadID(input); got != want {
			t.Errorf("%q = %d", input, got)
		}
	}
}

func TestAnimeMovieRetryUsesAnimeOnlyIndexerWithMovieAPI(t *testing.T) {
	db := testSchedulerDB(t)
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "movie" {
			calls++
			if r.URL.Query().Get("imdbid") != "1234567" {
				t.Error("IMDb movie identity lost")
			}
		}
		if r.URL.Query().Get("t") == "tvsearch" {
			t.Error("anime film routed to episode search")
		}
		io.WriteString(w, `<rss><channel></channel></rss>`)
	}))
	defer remote.Close()
	db.Exec(`INSERT INTO media_items(id,type,title,year,anime,imdb_id,quality_profile_id) VALUES(1,'movie','Chainsaw Man The Movie Reze Arc',2025,1,'tt1234567',2)`)
	db.Exec(`INSERT INTO indexers(name,url,api_key,type,content_types) VALUES('Anime only',?,'test','newznab','["anime"]')`, remote.URL)
	s := &Scheduler{db: db, newznab: indexer.NewNewznabClient(remote.Client())}
	s.retryMovieWithNextRelease(1)
	if calls != 1 {
		t.Fatalf("typed movie searches=%d", calls)
	}
}
