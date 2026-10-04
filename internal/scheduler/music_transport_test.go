package scheduler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/music"
	"mediaforge/internal/qbt"
	"mediaforge/internal/rutracker"
)

func TestUnavailablePreferredTorrentDoesNotHideUsableMusicNZB(t *testing.T) {
	for _, missingClient := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing_client_%v", missingClient), func(t *testing.T) {
			db := testSchedulerDB(t)
			_, err := db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');
 INSERT INTO albums(id,artist_id,title,track_count,quality_profile_id) SELECT 1,1,'Album',1,id FROM quality_profiles WHERE profile_type='music' LIMIT 1`)
			if err != nil {
				t.Fatal(err)
			}
			var torrentCalls, submitted atomic.Int32
			var remote *httptest.Server
			remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/forum/login.php":
					torrentCalls.Add(1)
					http.SetCookie(w, &http.Cookie{Name: "bb_session", Value: "test"})
				case "/forum/tracker.php":
					torrentCalls.Add(1)
					io.WriteString(w, `<table><tr class="tCenter"><td><a class="tLink" href="viewtopic.php?t=123">Artist - Album 24bit FLAC</a></td></tr></table>`)
				case "/nzb":
					io.WriteString(w, `<nzb/>`)
				default:
					if r.URL.Query().Get("mode") == "addfile" {
						submitted.Add(1)
						io.WriteString(w, `{"status":true,"nzo_ids":["music-job"]}`)
					} else {
						fmt.Fprintf(w, `<rss><channel><item><title>Artist - Album MP3 320kbps</title><link>%s/nzb</link></item></channel></rss>`, remote.URL)
					}
				}
			}))
			defer remote.Close()
			for _, kind := range []string{"newznab", "rutracker"} {
				if _, err := db.Exec(`INSERT INTO indexers(name,url,api_key,type,enabled,content_types) VALUES(?,?,'test',?,1,'["music"]')`, kind, remote.URL, kind); err != nil {
					t.Fatal(err)
				}
			}
			s := &Scheduler{db: db, newznab: indexer.NewNewznabClient(remote.Client()), grabberSvc: grabber.New(remote.Client(), remote.Client(), remote.URL, "test"), rtClient: rutracker.New(remote.Client(), remote.URL)}
			if missingClient {
				db.SetSetting("qbittorrent_enabled", "true")
			} else {
				db.SetSetting("qbittorrent_enabled", "false")
				s.qbt = qbt.New(remote.Client(), remote.URL, "", "")
			}
			var profileID int
			db.QueryRow(`SELECT quality_profile_id FROM albums WHERE id=1`).Scan(&profileID)
			profile := s.loadProfile(profileID)
			preferred, usable := indexer.Release{Title: "Artist - Album 24bit FLAC"}, indexer.Release{Title: "Artist - Album MP3 320kbps"}
			identity := indexer.MusicIdentity{Artist: "Artist", Album: "Album"}
			indexer.ScoreMusicRelease(&preferred, profile, identity)
			indexer.ScoreMusicRelease(&usable, profile, identity)
			if !preferred.Acceptable || !usable.Acceptable || indexer.CompareReleases(&preferred, &usable) <= 0 {
				t.Fatal("fixture must prefer the unavailable torrent")
			}
			if _, err := music.Request(db, 1, false); err != nil {
				t.Fatal(err)
			}
			s.processMusicRequests()
			var kind, title string
			if err := db.QueryRow(`SELECT download_type,nzb_title FROM downloads WHERE album_id=1`).Scan(&kind, &title); err != nil {
				t.Fatal(err, music.Snapshot(db, 1))
			}
			if kind != "nzb" || title != usable.Title || submitted.Load() != 1 || torrentCalls.Load() != 0 {
				t.Fatalf("kind=%s title=%s submitted=%d torrent_calls=%d", kind, title, submitted.Load(), torrentCalls.Load())
			}
		})
	}
}
