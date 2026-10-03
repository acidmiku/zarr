package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/qbt"
)

func TestCancelUpgradeKeepsPlayableMovieAndEpisode(t *testing.T) {
	for _, episode := range []bool{false, true} {
		t.Run(fmt.Sprint(episode), func(t *testing.T) {
			db := downloadsTestDB(t)
			db.Exec(`INSERT INTO media_items(id,type,title,status,current_release_title) VALUES(1,'movie','Example','available','Original.1080p.BluRay')`)
			var episodeID any
			if episode {
				db.Exec(`UPDATE media_items SET type='series' WHERE id=1`)
				db.Exec(`INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
				db.Exec(`INSERT INTO episodes(id,season_id,media_item_id,number,status,current_release_title,file_path) VALUES(1,1,1,1,'available','Original.1080p.BluRay','existing.mkv')`)
				episodeID = 1
			}
			db.Exec(`INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,status,download_type,qbt_hash,upgrade_from_title) VALUES(1,1,?,'Upgrade','downloading','torrent','testhash','Original.1080p.BluRay')`, episodeID)
			client := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
			defer client.Close()
			s := &Server{db: db, qbt: qbt.New(client.Client(), client.URL, "", "")}
			req := httptest.NewRequest("DELETE", "/api/downloads/1", nil)
			req.SetPathValue("id", "1")
			response := httptest.NewRecorder()
			s.handleCancelDownload(response, req)
			var status, title, downloadStatus string
			table := "media_items"
			if episode {
				table = "episodes"
			}
			db.QueryRow("SELECT status,current_release_title FROM "+table+" WHERE id=1").Scan(&status, &title)
			db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&downloadStatus)
			if response.Code != 200 || status != "available" || title != "Original.1080p.BluRay" || downloadStatus != "cancelled" {
				t.Fatalf("HTTP=%d library=%s title=%s download=%s", response.Code, status, title, downloadStatus)
			}
		})
	}
}

func TestRetryMissingAutomaticUpgradeSchedulesPolicyCheck(t *testing.T) {
	db := downloadsTestDB(t)
	db.Exec(`INSERT INTO media_items(id,type,title,status,current_release_title,upgrade_checked_at) VALUES(1,'movie','Example','available','Original.1080p.BluRay',CURRENT_TIMESTAMP)`)
	db.Exec(`INSERT INTO downloads(id,media_item_id,nzb_title,status,upgrade_from_title) VALUES(1,1,'Upgrade','failed','Old.720p.WEB-DL')`)
	s := &Server{db: db} // No network clients: retry must not enter manual search.
	req := httptest.NewRequest("POST", "/api/downloads/1/retry", nil)
	req.SetPathValue("id", "1")
	response := httptest.NewRecorder()
	s.handleRetryDownload(response, req)
	var status string
	var eligible bool
	db.QueryRow(`SELECT status,upgrade_checked_at IS NULL FROM media_items WHERE id=1`).Scan(&status, &eligible)
	if response.Code != 200 || status != "available" || !eligible {
		t.Fatalf("HTTP=%d status=%s eligible=%v", response.Code, status, eligible)
	}
}

func TestManualUpgradeEnqueueKeepsAvailableAndCompletedJobBlocksDuplicate(t *testing.T) {
	db := downloadsTestDB(t)
	db.Exec(`INSERT INTO media_items(id,type,title,status) VALUES(1,'movie','Example','available')`)
	submits := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nzb" {
			io.WriteString(w, `<nzb/>`)
		} else {
			submits++
			io.WriteString(w, `{"status":true,"nzo_ids":["job"]}`)
		}
	}))
	defer remote.Close()
	s := &Server{db: db, grabber: grabber.New(remote.Client(), remote.Client(), remote.URL, "test")}
	release := indexer.Release{Title: "Example.1080p.WEB-DL", NZBURL: remote.URL + "/nzb"}
	id, err := s.enqueueRelease(1, 0, 0, release)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE downloads SET status='completed' WHERE id=?`, id)
	if _, err := s.enqueueRelease(1, 0, 0, release); err == nil {
		t.Fatal("submitted duplicate while prior job awaits import")
	}
	var status string
	db.QueryRow(`SELECT status FROM media_items WHERE id=1`).Scan(&status)
	if status != "available" || submits != 1 {
		t.Fatalf("status=%s submissions=%d", status, submits)
	}
}
