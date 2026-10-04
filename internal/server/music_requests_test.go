package server

import (
	"encoding/json"
	"mediaforge/internal/music"
	"testing"
)

func TestMusicSaveIsNotARequestAndDownloadDeduplicates(t *testing.T) {
	s := newConnectionTestServer(t)
	w := callDomain(s.handleAddMusicToLibrary, "POST", "/", `{"release_group_id":"group","artist_mbid":"artist","artist_name":"Artist","album_title":"Album"}`, "", "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var saved struct{ ID int }
	json.Unmarshal(w.Body.Bytes(), &saved)
	var monitored, count int
	s.db.QueryRow(`SELECT monitored FROM albums WHERE id=?`, saved.ID).Scan(&monitored)
	s.db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&count)
	if monitored != 0 || count != 0 {
		t.Fatalf("save queued download monitored=%d requests=%d", monitored, count)
	}
	s.db.Exec(`UPDATE albums SET track_count=1 WHERE id=?`, saved.ID)
	for i := 0; i < 2; i++ {
		w = callDomain(s.handleDownloadMusicAlbum, "POST", "/", "", "id", "1")
		if w.Code != 202 {
			t.Fatal(w.Body.String())
		}
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM music_search_requests`).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
	w = callDomain(s.handleGetMusicLibraryItem, "GET", "/", "", "id", "1")
	var details struct {
		Album struct {
			Monitored   bool
			Acquisition music.Acquisition
		}
	}
	json.Unmarshal(w.Body.Bytes(), &details)
	if details.Album.Monitored || details.Album.Acquisition.Status != "queued" {
		t.Fatal(w.Body.String())
	}
}

func TestMusicMonitorAndFavoriteAreExplicit(t *testing.T) {
	s := newConnectionTestServer(t)
	s.db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');INSERT INTO albums(id,artist_id,title,track_count) VALUES(1,1,'Album',1)`)
	if w := callDomain(s.handleMonitorMusicAlbum, "PATCH", "/", `{}`, "id", "1"); w.Code != 400 {
		t.Fatal("missing monitoring intent accepted")
	}
	w := callDomain(s.handleMonitorMusicAlbum, "PATCH", "/", `{"monitored":true}`, "id", "1")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a := music.Snapshot(s.db, 1); a.Status != "queued" {
		t.Fatal(a)
	}
	w = callDomain(s.handleMonitorMusicAlbum, "PATCH", "/", `{"monitored":false}`, "id", "1")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a := music.Snapshot(s.db, 1); a.Status != "cancelled" {
		t.Fatal(a)
	}
	w = callDomain(s.handleFavoriteMusicAlbum, "PATCH", "/", `{"favorite":true}`, "id", "1")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var favorite, monitored bool
	s.db.QueryRow(`SELECT favorite,monitored FROM albums WHERE id=1`).Scan(&favorite, &monitored)
	if !favorite || monitored {
		t.Fatal("favorite changed monitoring")
	}
}

func TestManualMusicGrabRejectsWrongAlbumBeforeContactingClient(t *testing.T) {
	s := newConnectionTestServer(t)
	s.db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Correct Artist');INSERT INTO albums(id,artist_id,title,quality_profile_id) SELECT 1,1,'Album',id FROM quality_profiles WHERE profile_type='music' LIMIT 1`)
	w := callDomain(s.handleGrabMusicRelease, "POST", "/", `{"album_id":1,"release_url":"https://example.test/a.nzb","title":"Different Artist - Album FLAC"}`, "", "")
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM downloads`).Scan(&n)
	if n != 0 {
		t.Fatal("wrong identity reserved download")
	}
}
