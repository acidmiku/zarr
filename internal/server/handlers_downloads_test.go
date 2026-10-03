package server

import (
	"io"
	"mediaforge/internal/database"
	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/qbt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func downloadsTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func TestFailedManualGrabPreservesWantedState(t *testing.T) {
	db := downloadsTestDB(t)
	db.Exec(`INSERT INTO media_items(id,type,title) VALUES(1,'movie','Movie')`)
	client := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer client.Close()
	s := &Server{db: db, grabber: grabber.New(client.Client(), client.Client(), client.URL, "key")}
	req := httptest.NewRequest("POST", "/api/releases/grab", strings.NewReader(`{"media_item_id":1,"release_url":"`+client.URL+`/nzb"}`))
	w := httptest.NewRecorder()
	s.handleGrabRelease(w, req)
	if w.Code == 200 {
		t.Fatal("failed grab reported successful")
	}
	var status string
	db.QueryRow(`SELECT status FROM media_items WHERE id=1`).Scan(&status)
	if status != "wanted" {
		t.Fatal(status)
	}
	db.QueryRow(`SELECT status FROM downloads`).Scan(&status)
	if status != "failed" {
		t.Fatal(status)
	}
}
func TestEnqueueRejectsEpisodeFromAnotherSeries(t *testing.T) {
	db := downloadsTestDB(t)
	db.Exec(`INSERT INTO media_items(id,type,title) VALUES(1,'series','One'),(2,'series','Two')`)
	db.Exec(`INSERT INTO seasons(id,media_item_id,number) VALUES(1,2,1)`)
	db.Exec(`INSERT INTO episodes(id,season_id,media_item_id,number) VALUES(1,1,2,1)`)
	s := &Server{db: db, grabber: grabber.New(http.DefaultClient, http.DefaultClient, "", "")}
	if _, err := s.enqueueRelease(1, 1, 0, indexer.Release{NZBURL: "https://example.test/nzb"}); err == nil {
		t.Fatal("accepted episode from another series")
	}
}
func TestCancelDiscoversTorrentBeforeFirstPoll(t *testing.T) {
	db := downloadsTestDB(t)
	db.Exec(`INSERT INTO media_items(id,type,title,status) VALUES(1,'movie','Movie','downloading')`)
	db.Exec(`INSERT INTO downloads(id,media_item_id,nzb_title,download_type) VALUES(1,1,'torrent','torrent')`)
	deleted := false
	client := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/info" {
			io.WriteString(w, `[{"hash":"found","tags":"other, dl_1"}]`)
			return
		}
		r.ParseForm()
		if r.FormValue("hashes") == "found" {
			deleted = true
		}
	}))
	defer client.Close()
	s := &Server{db: db, qbt: qbt.New(client.Client(), client.URL, "", "")}
	req := httptest.NewRequest("DELETE", "/api/downloads/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCancelDownload(w, req)
	if w.Code != 200 || !deleted {
		t.Fatalf("cancel=%d deleted=%v", w.Code, deleted)
	}
	var status string
	db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status)
	if status != "cancelled" {
		t.Fatal(status)
	}
}
func TestClientCancellationFailureDoesNotLieAboutState(t *testing.T) {
	db := downloadsTestDB(t)
	db.Exec(`INSERT INTO media_items(id,type,title,status) VALUES(1,'movie','Movie','available')`)
	db.Exec(`INSERT INTO downloads(id,media_item_id,nzb_title,download_type,qbt_hash,status) VALUES(1,1,'torrent','torrent','hash','seeding')`)
	client := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer client.Close()
	s := &Server{db: db, qbt: qbt.New(client.Client(), client.URL, "", "")}
	req := httptest.NewRequest("DELETE", "/api/downloads/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCancelDownload(w, req)
	if w.Code != 502 {
		t.Fatal(w.Code)
	}
	var status string
	db.QueryRow(`SELECT status FROM downloads WHERE id=1`).Scan(&status)
	if status != "seeding" {
		t.Fatal(status)
	}
	db.QueryRow(`SELECT status FROM media_items WHERE id=1`).Scan(&status)
	if status != "available" {
		t.Fatal(status)
	}
}

func TestEmptyDownloadsNeverPollsClients(t *testing.T) {
	db := downloadsTestDB(t)
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer remote.Close()
	s := &Server{db: db, grabber: grabber.New(remote.Client(), remote.Client(), remote.URL, "")}
	w := httptest.NewRecorder()
	s.handleGetDownloads(w, httptest.NewRequest("GET", "/api/downloads", nil))
	if w.Code != 200 || calls != 0 {
		t.Fatalf("empty downloads performed %d network calls", calls)
	}
}

func TestAnimeMovieSearchRoutesUseAnimeOnlyIndexerWithMovieAPI(t *testing.T) {
	for _, mode := range []string{"manual", "search_all"} {
		t.Run(mode, func(t *testing.T) {
			db := downloadsTestDB(t)
			calls := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("t") == "movie" {
					calls++
					if r.URL.Query().Get("imdbid") != "1234567" {
						t.Error("IMDb movie identity lost")
					}
				}
				if r.URL.Query().Get("t") == "tvsearch" {
					t.Error("anime film routed to episodic search")
				}
				io.WriteString(w, `<rss><channel></channel></rss>`)
			}))
			defer remote.Close()
			db.Exec(`INSERT INTO media_items(id,type,title,year,anime,imdb_id,quality_profile_id) VALUES(1,'movie','Chainsaw Man The Movie Reze Arc',2025,1,'tt1234567',2)`)
			db.Exec(`INSERT INTO indexers(name,url,api_key,type,content_types) VALUES('Anime only',?,'test','newznab','["anime"]')`, remote.URL)
			s := &Server{db: db, newznab: indexer.NewNewznabClient(remote.Client())}
			w := httptest.NewRecorder()
			if mode == "manual" {
				s.handleSearchReleases(w, httptest.NewRequest("GET", "/api/releases?media_item_id=1", nil))
			} else {
				r := httptest.NewRequest("POST", "/api/library/1/search", nil)
				r.SetPathValue("id", "1")
				s.handleSearchAll(w, r)
			}
			if w.Code != 200 || calls != 1 {
				t.Fatalf("mode=%s status=%d typed_movie_calls=%d", mode, w.Code, calls)
			}
		})
	}
}
