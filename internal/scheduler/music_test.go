package scheduler

import (
	"io"
	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/music"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCancelledMusicRequestCannotReserveLateResult(t *testing.T) {
	db := testSchedulerDB(t)
	db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');INSERT INTO albums(id,artist_id,title) VALUES(1,1,'Album')`)
	request, err := music.Request(db, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE music_search_requests SET status='searching' WHERE id=?`, request.RequestID)
	if err = music.Pause(db, 1); err != nil {
		t.Fatal(err)
	}
	s := &Scheduler{db: db, grabberSvc: grabber.New(nil, nil, "", "")}
	err = s.enqueueReleaseForRequest(request.RequestID, 0, 0, 1, indexer.Release{Title: "Artist - Album FLAC", NZBURL: "https://not-requested.test/file.nzb"})
	if err == nil {
		t.Fatal("cancelled request admitted a late release")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM downloads`).Scan(&n)
	if n != 0 {
		t.Fatal("late reservation persisted")
	}
}

func TestMusicWishlistExcludedAndOnlyMonitoredAlbumsSearch(t *testing.T) {
	db := testSchedulerDB(t)
	_, err := db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');INSERT INTO albums(id,artist_id,title,monitored) VALUES(1,1,'Saved',0),(2,1,'Monitored',1)`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Scheduler{db: db}
	s.searchWantedAlbums()
	s.searchWantedAlbums()
	var count, album int
	db.QueryRow(`SELECT COUNT(*),album_id FROM music_search_requests`).Scan(&count, &album)
	if count != 1 || album != 2 {
		t.Fatalf("requests=%d album=%d", count, album)
	}
	s.processMusicRequests()
	if a := music.Snapshot(db, 2); a.Status != "blocked" || a.Message == "" {
		t.Fatal(a)
	}
	if a := music.Snapshot(db, 1); a.Status != "saved" {
		t.Fatal(a)
	}
}

func TestMusicNoResultsDistinguishesUnavailableSourcesAndWrongIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"empty", `<rss><channel></channel></rss>`, "no_results", 200},
		{"wrong artist", `<rss><channel><item><title>Wrong Artist - Album FLAC</title><link>https://example.test/download</link></item></channel></rss>`, "no_results", 200},
		{"outage", `<error code="100" description="apikey=private"/>`, "blocked", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testSchedulerDB(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer srv.Close()
			_, err := db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');INSERT INTO albums(id,artist_id,title,track_count,quality_profile_id) SELECT 1,1,'Album',1,id FROM quality_profiles WHERE profile_type='music' LIMIT 1`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO indexers(name,url,api_key,type,enabled,content_types) VALUES('Music',?,'private','newznab',1,'["music"]')`, srv.URL); err != nil {
				t.Fatal(err)
			}
			s := &Scheduler{db: db, newznab: indexer.NewNewznabClient(srv.Client())}
			music.Request(db, 1, false)
			s.processMusicRequests()
			a := music.Snapshot(db, 1)
			if a.Status != tc.want {
				t.Fatal(a)
			}
			var downloads int
			db.QueryRow(`SELECT COUNT(*) FROM downloads`).Scan(&downloads)
			if downloads != 0 {
				t.Fatal("unexpected transfer")
			}
		})
	}
}
