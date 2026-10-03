package scheduler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"mediaforge/internal/database"
	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
)

func upgradeExec(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func seedUpgradeMovie(t *testing.T, db *database.DB, id int, title string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Example.mkv"), []byte("playable existing movie"), 0644); err != nil {
		t.Fatal(err)
	}
	upgradeExec(t, db, `INSERT INTO media_items(id,type,title,year,anime,status,root_path,quality_profile_id,current_release_title)
 VALUES(?,'movie','Example',2024,1,'available',?,(SELECT id FROM quality_profiles WHERE scoring_config='{"preset":"radarr-anime"}'),?)`, id, root, title)
}

func fakeUpgradeClients(t *testing.T, db *database.DB, candidate string, failSubmission bool) (*Scheduler, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	searches, submits := &atomic.Int32{}, &atomic.Int32{}
	var remote *httptest.Server
	remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			io.WriteString(w, `<nzb/>`)
			return
		}
		if r.URL.Query().Get("mode") == "addfile" {
			submits.Add(1)
			if failSubmission {
				io.WriteString(w, `{"status":false,"error":"test failure"}`)
			} else {
				io.WriteString(w, `{"status":true,"nzo_ids":["test-job"]}`)
			}
			return
		}
		searches.Add(1)
		if candidate == "" {
			io.WriteString(w, `<rss><channel/></rss>`)
		} else {
			io.WriteString(w, `<rss><channel>`)
			for i, title := range strings.Split(candidate, "\n") {
				fmt.Fprintf(w, `<item><title>%s</title><link>%s/download?release=%d</link></item>`, title, remote.URL, i)
			}
			io.WriteString(w, `</channel></rss>`)
		}
	}))
	t.Cleanup(remote.Close)
	upgradeExec(t, db, `INSERT INTO indexers(name,url,api_key,type,content_types) VALUES('Anime only',?,'test','newznab','["anime"]')`, remote.URL)
	return &Scheduler{db: db, newznab: indexer.NewNewznabClient(remote.Client()), grabberSvc: grabber.New(remote.Client(), remote.Client(), remote.URL, "test")}, searches, submits
}

func TestAutomaticUpgradePreservesAvailableAndReservesOneJob(t *testing.T) {
	for _, failSubmission := range []bool{false, true} {
		t.Run(fmt.Sprint(failSubmission), func(t *testing.T) {
			db := testSchedulerDB(t)
			baseline := "[SubsPlease] Example.2024.720p.WEB-DL"
			seedUpgradeMovie(t, db, 1, baseline)
			s, searches, submits := fakeUpgradeClients(t, db, "[SubsPlease] Example.2024.1080p.WEB-DL", failSubmission)
			s.checkQualityUpgrades()
			s.checkQualityUpgrades()
			var status, upgradeFrom, downloadStatus, path string
			if err := db.QueryRow(`SELECT status,root_path FROM media_items WHERE id=1`).Scan(&status, &path); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT status,upgrade_from_title FROM downloads WHERE media_item_id=1`).Scan(&downloadStatus, &upgradeFrom); err != nil {
				t.Fatalf("no upgrade reserved: %v (searches=%d submits=%d)", err, searches.Load(), submits.Load())
			}
			if status != "available" || upgradeFrom != baseline || submits.Load() != 1 {
				t.Fatalf("availability=%s baseline=%s downloads=%d", status, upgradeFrom, submits.Load())
			}
			if failSubmission && downloadStatus != "failed" || !failSubmission && downloadStatus != "queued" {
				t.Fatal(downloadStatus)
			}
			if body, err := os.ReadFile(filepath.Join(path, "Example.mkv")); err != nil || string(body) != "playable existing movie" {
				t.Fatal("existing file changed while queuing upgrade")
			}
		})
	}
}

func TestUpgradeSkipsLegacyDisabledUnknownAndWorseCandidates(t *testing.T) {
	for _, test := range []struct{ name, baseline, update, candidate string }{
		{"legacy", "Example.2024.720p.WEB-DL", `UPDATE quality_profiles SET scoring_config='{}'`, "Example.2024.1080p.WEB-DL"},
		{"disabled", "Example.2024.720p.WEB-DL", `UPDATE quality_profiles SET upgrade_allowed=0`, "Example.2024.1080p.WEB-DL"},
		{"unknown", "manual-grab", "", "Example.2024.1080p.WEB-DL"},
		{"worse", "Example.2024.1080p.BluRay", "", "Example.2024.720p.WEB-DL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := testSchedulerDB(t)
			seedUpgradeMovie(t, db, 1, test.baseline)
			if test.update != "" {
				upgradeExec(t, db, test.update)
			}
			s, searches, submits := fakeUpgradeClients(t, db, test.candidate, false)
			s.checkQualityUpgrades()
			if submits.Load() != 0 {
				t.Fatal("unexpected upgrade submission")
			}
			if test.name != "worse" && searches.Load() != 0 {
				t.Fatal("unconfigured or unknown baseline searched")
			}
		})
	}
}

func TestUpgradeUsesSuccessfulDownloadFallbackAndRejectsStaleReservation(t *testing.T) {
	db := testSchedulerDB(t)
	baseline := "Example.2024.720p.WEB-DL"
	seedUpgradeMovie(t, db, 1, "")
	upgradeExec(t, db, `INSERT INTO downloads(media_item_id,nzb_title,status) VALUES(1,?,'imported')`, baseline)
	s, _, submits := fakeUpgradeClients(t, db, "Example.2024.1080p.WEB-DL", false)
	if got, err := s.currentReleaseTitle(1, 0); err != nil || got != baseline {
		t.Fatalf("baseline fallback=%q err=%v", got, err)
	}
	upgradeExec(t, db, `UPDATE media_items SET current_release_title='Example.2024.1080p.BluRay' WHERE id=1`)
	err := s.enqueueRelease(1, 0, 0, indexer.Release{Title: "Example.2024.1080p.WEB-DL", NZBURL: "https://not-requested.test"}, baseline)
	if err == nil || submits.Load() != 0 {
		t.Fatal("stale baseline queued after another import")
	}
}

func TestUpgradeBatchAndCooldownBoundWork(t *testing.T) {
	db := testSchedulerDB(t)
	for id := 1; id <= upgradeBatchSize+2; id++ {
		seedUpgradeMovie(t, db, id, "Example.2024.720p.WEB-DL")
	}
	s, _, _ := fakeUpgradeClients(t, db, "", false)
	s.checkQualityUpgrades()
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE upgrade_checked_at IS NOT NULL`).Scan(&count)
	if count != upgradeBatchSize {
		t.Fatalf("first scan checked %d items", count)
	}
	s.checkQualityUpgrades()
	db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE upgrade_checked_at IS NOT NULL`).Scan(&count)
	if count != upgradeBatchSize+2 {
		t.Fatalf("second scan failed to move past cooldown targets: %d", count)
	}
}

func TestEpisodeUpgradeRetainsAvailableAndUsesBothQueries(t *testing.T) {
	db := testSchedulerDB(t)
	file := filepath.Join(t.TempDir(), "Example.S01E01.mkv")
	if err := os.WriteFile(file, []byte("old episode"), 0644); err != nil {
		t.Fatal(err)
	}
	upgradeExec(t, db, `INSERT INTO media_items(id,type,title,anime,quality_profile_id) VALUES(1,'series','Example',1,(SELECT id FROM quality_profiles WHERE scoring_config='{"preset":"sonarr-anime"}'))`)
	upgradeExec(t, db, `INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
	upgradeExec(t, db, `INSERT INTO episodes(id,media_item_id,season_id,number,absolute_number,status,file_path,current_release_title) VALUES(1,1,1,1,1,'available',?,'[SubsPlease] Example.S01E01.720p.WEB-DL')`, file)
	s, searches, submits := fakeUpgradeClients(t, db, "[SubsPlease] Example.S01E01.1080p.WEB-DL", false)
	s.checkQualityUpgrades()
	var status string
	db.QueryRow(`SELECT status FROM episodes WHERE id=1`).Scan(&status)
	if status != "available" || searches.Load() != 2 || submits.Load() != 1 {
		t.Fatalf("status=%s queries=%d submissions=%d", status, searches.Load(), submits.Load())
	}
}

func TestFailedAutoUpgradeDoesNotEnterUnconditionalWantedRetry(t *testing.T) {
	db := testSchedulerDB(t)
	seedUpgradeMovie(t, db, 1, "Example.2024.1080p.BluRay")
	upgradeExec(t, db, `INSERT INTO media_items(id,type,title,anime) VALUES(2,'series','Example',1)`)
	upgradeExec(t, db, `INSERT INTO seasons(id,media_item_id,number) VALUES(1,2,1)`)
	upgradeExec(t, db, `INSERT INTO episodes(id,media_item_id,season_id,number,status) VALUES(1,2,1,1,'available')`)
	upgradeExec(t, db, `INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,status,sabnzbd_nzo_id,upgrade_from_title)
 VALUES(1,1,NULL,'Movie upgrade','downloading','movie-job','old movie'),(2,2,1,'Episode upgrade','downloading','episode-job','old episode')`)
	var unexpected atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("mode") {
		case "queue":
			io.WriteString(w, `{"queue":{"slots":[]}}`)
		case "history":
			io.WriteString(w, `{"history":{"slots":[{"nzo_id":"movie-job","status":"Failed","fail_message":"test failure"},{"nzo_id":"episode-job","status":"Failed","fail_message":"test failure"}]}}`)
		default:
			unexpected.Add(1)
			io.WriteString(w, `<rss><channel/></rss>`)
		}
	}))
	defer remote.Close()
	upgradeExec(t, db, `INSERT INTO indexers(name,url,api_key,type,content_types) VALUES('Anime only',?,'test','newznab','["anime"]')`, remote.URL)
	s := &Scheduler{db: db, newznab: indexer.NewNewznabClient(remote.Client()), grabberSvc: grabber.New(remote.Client(), remote.Client(), remote.URL, "test")}
	s.pollDownloads()
	var movieStatus, episodeStatus string
	var failed, blacklisted int
	db.QueryRow(`SELECT status FROM media_items WHERE id=1`).Scan(&movieStatus)
	db.QueryRow(`SELECT status FROM episodes WHERE id=1`).Scan(&episodeStatus)
	db.QueryRow(`SELECT COUNT(*) FROM downloads WHERE status='failed'`).Scan(&failed)
	db.QueryRow(`SELECT COUNT(*) FROM release_blacklist`).Scan(&blacklisted)
	if movieStatus != "available" || episodeStatus != "available" || failed != 2 || blacklisted != 2 || unexpected.Load() != 0 {
		t.Fatalf("movie=%s episode=%s failed=%d blacklisted=%d unexpected searches=%d", movieStatus, episodeStatus, failed, blacklisted, unexpected.Load())
	}
}

func TestQualityCutoffDoesNotHideEligibleFormatUpgrade(t *testing.T) {
	db := testSchedulerDB(t)
	seedUpgradeMovie(t, db, 1, "[SubsPlease] Example.2024.1080p.WEB-DL")
	upgradeExec(t, db, `UPDATE quality_profiles SET scoring_config='{"preset":"radarr-anime","upgrade_until_quality":"web-1080p"}' WHERE id=(SELECT quality_profile_id FROM media_items WHERE id=1)`)
	want := "[LostYears] Example.2024.1080p.WEB-DL"
	s, _, submits := fakeUpgradeClients(t, db, "[SCY] Example.2024.1080p.BluRay\n"+want, false)
	s.checkQualityUpgrades()
	var title string
	if err := db.QueryRow(`SELECT nzb_title FROM downloads WHERE media_item_id=1`).Scan(&title); err != nil || title != want || submits.Load() != 1 {
		t.Fatalf("eligible format upgrade hidden by blocked quality upgrade: title=%s submissions=%d err=%v", title, submits.Load(), err)
	}
}

func TestFailedImportSourcePreventsAutomaticRegrabUntilResolved(t *testing.T) {
	db := testSchedulerDB(t)
	baseline := "[SubsPlease] Example.2024.720p.WEB-DL"
	seedUpgradeMovie(t, db, 1, baseline)
	src := filepath.Join(t.TempDir(), "completed.mkv")
	if err := os.WriteFile(src, []byte("awaiting local import"), 0644); err != nil {
		t.Fatal(err)
	}
	upgradeExec(t, db, `INSERT INTO downloads(media_item_id,nzb_title,status,download_path,upgrade_from_title) VALUES(1,'upgrade','failed',?,?)`, src, baseline)
	s, searches, submits := fakeUpgradeClients(t, db, "[SubsPlease] Example.2024.1080p.WEB-DL", false)
	s.checkQualityUpgrades()
	if searches.Load() != 0 || submits.Load() != 0 {
		t.Fatal("already-downloaded failed import was regrabbed")
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	upgradeExec(t, db, `UPDATE media_items SET upgrade_checked_at=NULL WHERE id=1`)
	s.checkQualityUpgrades()
	if submits.Load() != 1 {
		t.Fatal("missing failed source incorrectly blocked a fresh upgrade")
	}
}
