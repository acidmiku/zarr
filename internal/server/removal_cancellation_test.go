package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"mediaforge/internal/grabber"
)

func TestCancelThenRemoveDoesNotCancelRemoteJobAgain(t *testing.T) {
	for _, kind := range []string{"movie", "series", "music"} {
		t.Run(kind, func(t *testing.T) {
			s, remove, table := removalFixture(t, kind)
			calls := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Query().Get("mode") != "queue" || r.URL.Query().Get("name") != "delete" {
					t.Errorf("unexpected downloader operation: %s", r.URL.Path)
				}
				if calls == 1 {
					fmt.Fprint(w, `{"status":true}`)
				} else {
					fmt.Fprint(w, `{"status":false,"error":"Job not found"}`)
				}
			}))
			defer remote.Close()
			s.grabber = grabber.New(remote.Client(), remote.Client(), remote.URL, "test")
			w := callDomain(s.handleCancelDownload, "DELETE", "/", "", "id", "1")
			if w.Code != 200 {
				t.Fatalf("initial cancellation: %d %s", w.Code, w.Body.String())
			}
			w = callDomain(remove, "DELETE", "/", "", "id", "1")
			if w.Code != 200 || calls != 1 {
				t.Fatalf("remove after cancellation: %d %s; remote calls=%d", w.Code, w.Body.String(), calls)
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
				t.Fatalf("library record remains: count=%d err=%v", count, err)
			}
			if err := s.db.QueryRow(`SELECT count(*) FROM downloads`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("download record remains: count=%d err=%v", count, err)
			}
		})
	}
}

func TestRepeatedCancellationDoesNotNeedDownloader(t *testing.T) {
	s, _, _ := removalFixture(t, "movie")
	if _, err := s.db.Exec(`UPDATE downloads SET status='cancelled' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	w := callDomain(s.handleCancelDownload, "DELETE", "/", "", "id", "1")
	if w.Code != 200 {
		t.Fatalf("repeated cancellation: %d %s", w.Code, w.Body.String())
	}
}

func TestMissingSABJobCanBeCancelledOrRemoved(t *testing.T) {
	for _, kind := range []string{"movie", "series", "music"} {
		for _, action := range []string{"cancel", "remove"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				s, remove, table := removalFixture(t, kind)
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					q := r.URL.Query()
					if q.Get("mode") == "queue" && q.Get("name") == "delete" {
						fmt.Fprint(w, `{"status":false,"nzo_ids":[]}`)
						return
					}
					if q.Get("nzo_ids") != "job-1" {
						t.Error("did not verify the tracked job")
					}
					fmt.Fprintf(w, `{%q:{"slots":[],"noofslots":0}}`, q.Get("mode"))
				}))
				defer remote.Close()
				s.grabber = grabber.New(remote.Client(), remote.Client(), remote.URL, "test")
				handler := remove
				if action == "cancel" {
					handler = s.handleCancelDownload
				}
				w := callDomain(handler, "DELETE", "/", "", "id", "1")
				if w.Code != 200 {
					t.Fatalf("stale job blocked %s: %d %s", action, w.Code, w.Body.String())
				}
				if action == "cancel" {
					var status string
					if err := s.db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status); err != nil || status != "cancelled" {
						t.Fatalf("download status=%s err=%v", status, err)
					}
					statusTable := table
					if kind == "series" {
						statusTable = "episodes"
					}
					if err := s.db.QueryRow("SELECT status FROM " + statusTable + " WHERE id=1").Scan(&status); err != nil || status != "wanted" {
						t.Fatalf("library status=%s err=%v", status, err)
					}
				} else {
					var count int
					if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
						t.Fatalf("library record remains: count=%d err=%v", count, err)
					}
				}
			})
		}
	}
}

func TestUnverifiedSABJobStillBlocksRemoval(t *testing.T) {
	s, remove, _ := removalFixture(t, "movie")
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "delete" {
			fmt.Fprint(w, `{"status":false,"nzo_ids":[]}`)
		} else {
			fmt.Fprint(w, `{"status":false,"error":"API Key Incorrect"}`)
		}
	}))
	defer remote.Close()
	s.grabber = grabber.New(remote.Client(), remote.Client(), remote.URL, "test")
	w := callDomain(remove, "DELETE", "/", "", "id", "1")
	if w.Code != 502 {
		t.Fatalf("unverified cancellation allowed removal: %d %s", w.Code, w.Body.String())
	}
	for _, table := range []string{"downloads", "media_items"} {
		var status string
		if err := s.db.QueryRow("SELECT status FROM " + table + " WHERE id=1").Scan(&status); err != nil || status != "downloading" {
			t.Fatalf("failed cancellation changed %s: status=%s err=%v", table, status, err)
		}
	}
}

func TestRemovalRetryRemembersSuccessfulCancellations(t *testing.T) {
	for _, kind := range []string{"movie", "series", "music"} {
		t.Run(kind, func(t *testing.T) {
			s, remove, table := removalFixture(t, kind)
			if _, err := s.db.Exec(`INSERT INTO downloads(id,media_item_id,episode_id,album_id,nzb_title,sabnzbd_nzo_id,status)
SELECT 2,media_item_id,episode_id,album_id,'Second release','job-2','downloading' FROM downloads WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			calls := map[string]int{}
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				job := r.URL.Query().Get("value")
				calls[job]++
				switch {
				case job == "job-1" && calls[job] > 1:
					fmt.Fprint(w, `{"status":false,"error":"Job not found"}`)
				case job == "job-2" && calls[job] == 1:
					w.WriteHeader(http.StatusServiceUnavailable)
				default:
					fmt.Fprint(w, `{"status":true}`)
				}
			}))
			defer remote.Close()
			s.grabber = grabber.New(remote.Client(), remote.Client(), remote.URL, "test")
			w := callDomain(remove, "DELETE", "/", "", "id", "1")
			if w.Code != 502 {
				t.Fatalf("active cancellation failure was ignored: %d %s", w.Code, w.Body.String())
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 1 {
				t.Fatalf("failed removal lost library record: count=%d err=%v", count, err)
			}
			for id, want := range map[int]string{1: "cancelled", 2: "downloading"} {
				var status string
				if err := s.db.QueryRow(`SELECT status FROM downloads WHERE id=?`, id).Scan(&status); err != nil || status != want {
					t.Fatalf("download %d status=%s want=%s err=%v", id, status, want, err)
				}
			}
			w = callDomain(remove, "DELETE", "/", "", "id", "1")
			if w.Code != 200 || calls["job-1"] != 1 || calls["job-2"] != 2 {
				t.Fatalf("removal retry: %d %s; remote calls=%v", w.Code, w.Body.String(), calls)
			}
		})
	}
}

func removalFixture(t *testing.T, kind string) (*Server, http.HandlerFunc, string) {
	t.Helper()
	s := newConnectionTestServer(t)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	if kind == "music" {
		exec(`INSERT INTO artists(id,mbid,name) VALUES(1,'artist','Artist')`)
		exec(`INSERT INTO albums(id,artist_id,title,status) VALUES(1,1,'Album','downloading')`)
		exec(`INSERT INTO downloads(id,album_id,nzb_title,sabnzbd_nzo_id,status) VALUES(1,1,'Release','job-1','downloading')`)
		return s, s.handleDeleteMusicLibraryItem, "albums"
	}
	exec(`INSERT INTO media_items(id,type,title,status) VALUES(1,?,'Title','downloading')`, kind)
	var episode any
	if kind == "series" {
		exec(`INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`)
		exec(`INSERT INTO episodes(id,season_id,media_item_id,number,status) VALUES(1,1,1,1,'downloading')`)
		episode = 1
	}
	exec(`INSERT INTO downloads(id,media_item_id,episode_id,nzb_title,sabnzbd_nzo_id,status) VALUES(1,1,?,'Release','job-1','downloading')`, episode)
	return s, s.handleDeleteLibraryItem, "media_items"
}
