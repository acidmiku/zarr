package music

import (
	"fmt"
	"mediaforge/internal/database"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRetainedImportSourceIsNotDownloadedAgain(t *testing.T) {
	db := musicDB(t)
	path := filepath.Join(t.TempDir(), "album")
	os.Mkdir(path, 0755)
	db.Exec(`INSERT INTO downloads(id,album_id,nzb_title,status,download_path) VALUES(1,1,'album','failed',?)`, path)
	db.Exec(`INSERT INTO downloads(id,album_id,nzb_title,status,download_path) VALUES(2,1,'newer missing payload','failed',?)`, filepath.Join(path, "missing"))
	a, err := Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "failed" || a.DownloadID != 1 {
		t.Fatal(a)
	}
	var requests int
	db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&requests)
	if requests != 0 {
		t.Fatal("retained source triggered new acquisition")
	}
}

func musicDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist'); INSERT INTO albums(id,artist_id,title,track_count) VALUES(1,1,'Album',1)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSavedIntentAndConcurrentRequests(t *testing.T) {
	db := musicDB(t)
	if got := Snapshot(db, 1); got.Status != "saved" {
		t.Fatal(got)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Request(db, 1, false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n, monitored int
	db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&n)
	db.QueryRow(`SELECT monitored FROM albums WHERE id=1`).Scan(&monitored)
	if n != 1 || monitored != 0 {
		t.Fatalf("requests=%d monitored=%d", n, monitored)
	}
	if a := Snapshot(db, 1); a.Status != "queued" || a.Retryable {
		t.Fatal(a)
	}
}

func TestRetryDoesNotReuseOldTransferAndImportMustBeVerified(t *testing.T) {
	db := musicDB(t)
	db.Exec(`INSERT INTO downloads(id,album_id,nzb_title,status) VALUES(1,1,'old','failed')`)
	first, err := Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "queued" || first.DownloadID != 0 {
		t.Fatal(first)
	}
	Finish(db, first.RequestID, "no_results", "No matching release", 0)
	next, err := Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.RequestID == first.RequestID {
		t.Fatal("terminal request reused")
	}
	db.Exec(`INSERT INTO downloads(id,album_id,nzb_title,status) VALUES(2,1,'new','completed')`)
	Finish(db, next.RequestID, "download_queued", "queued", 2)
	if got := Snapshot(db, 1); got.Status != "importing" || got.Retryable {
		t.Fatal(got)
	}
	db.Exec(`UPDATE albums SET status='available' WHERE id=1`)
	if got := Snapshot(db, 1); got.Status != "available" || got.Retryable {
		t.Fatal(got)
	}
	Request(db, 1, false)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&n)
	if n != 2 {
		t.Fatalf("available album created request: %d", n)
	}
}

func TestActiveTransferAndPauseWinOverSearch(t *testing.T) {
	db := musicDB(t)
	a, err := Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = Pause(db, 1); err != nil {
		t.Fatal(err)
	}
	Finish(db, a.RequestID, "download_queued", "late response", 99)
	if got := Snapshot(db, 1); got.Status != "cancelled" {
		t.Fatal(got)
	}
	db.Exec(`INSERT INTO downloads(id,album_id,nzb_title,status) VALUES(1,1,'old failure','failed'),(2,1,'active','downloading')`)
	current, err := Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "downloading" || current.DownloadID != 2 {
		t.Fatal(current)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate active transfer requested")
	}
}

func TestRequestsSurviveRestartAndDeleteCascades(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');INSERT INTO albums(id,artist_id,title) VALUES(1,1,'Album')`)
	a, err := Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := Snapshot(db, 1); got.Status != "queued" || got.RequestID != a.RequestID {
		t.Fatal(got)
	}
	if _, err = db.Exec(`DELETE FROM albums WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&n)
	if n != 0 {
		t.Fatal(fmt.Sprint("orphan requests: ", n))
	}
}
