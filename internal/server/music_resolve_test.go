package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mediaforge/internal/metadata"
)

func seedResolveGroup(t *testing.T, s *Server, id, artist, title, date string) {
	t.Helper()
	group := metadata.MBReleaseGroup{ID: id, Title: title, FirstRelease: date, PrimaryType: "Album", ArtistCredit: []metadata.MBCredit{{Artist: metadata.MBArtist{ID: "artist-id", Name: artist}}}}
	data, _ := json.Marshal(group)
	if _, err := s.db.Exec(`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid,metadata) VALUES(?,?,?,'artist-id',?)`, id, title, artist, string(data)); err != nil {
		t.Fatal(err)
	}
}
func TestResolveProviderAlbumUsesExactLocalIdentityAndCanonicalGroup(t *testing.T) {
	s := newConnectionTestServer(t)
	requests := 0
	s.musicbrainz = metadata.NewMusicBrainzClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return artworkResponse(503, []byte(`{}`)), nil
	})}, s.db)
	seedResolveGroup(t, s, "canonical", "Muse", "Absolution", "2003")
	seedResolveGroup(t, s, "false-artist", "Museum", "Absolution", "2003")
	w := callConnection(t, s.handleResolveMusicAlbum, "GET", "/api/music/resolve?artist=Muse&title=Absolution&mbid=edition-id", "")
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got["id"] != "canonical" || got["release_group_id"] != "canonical" || got["provider"] != "musicbrainz" || requests != 0 {
		t.Fatal(w.Code, w.Body.String(), requests)
	}
}
func TestResolveProviderAlbumRejectsAmbiguityAndUsesYear(t *testing.T) {
	s := newConnectionTestServer(t)
	s.musicbrainz = metadata.NewMusicBrainzClient(http.DefaultClient, s.db)
	seedResolveGroup(t, s, "one", "Artist", "Album", "2001")
	seedResolveGroup(t, s, "two", "Artist", "Album", "2020")
	w := callConnection(t, s.handleResolveMusicAlbum, "GET", "/api/music/resolve?artist=Artist&title=Album", "")
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = callConnection(t, s.handleResolveMusicAlbum, "GET", "/api/music/resolve?artist=Artist&title=Album&year=2020", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"two"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = callConnection(t, s.handleResolveMusicAlbum, "GET", "/api/music/resolve?artist=Artist&title=Album&year=oops", "")
	if w.Code != 400 {
		t.Fatal("invalid year accepted")
	}
}
func TestResolveProviderAlbumVerifiesRemoteSearchResults(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"exact", `{"count":1,"release-groups":[{"id":"group","title":"Album","artist-credit":[{"artist":{"id":"artist","name":"Muse"}}]}]}`, 200},
		{"loose artist", `{"count":1,"release-groups":[{"id":"wrong","title":"Album","artist-credit":[{"artist":{"id":"other","name":"Museum"}}]}]}`, 404},
		{"different edition title", `{"count":1,"release-groups":[{"id":"wrong","title":"Album (Live)","artist-credit":[{"artist":{"id":"artist","name":"Muse"}}]}]}`, 404},
		{"truncated results", `{"count":26,"release-groups":[{"id":"group","title":"Album","artist-credit":[{"artist":{"id":"artist","name":"Muse"}}]}]}`, 409},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newConnectionTestServer(t)
			s.musicbrainz = metadata.NewMusicBrainzClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
				if q := r.URL.Query().Get("query"); q != `releasegroup:"Album" AND artist:"Muse"` {
					t.Error("unexpected identity query", q)
				}
				return artworkResponse(200, []byte(test.body)), nil
			})}, s.db)
			w := callConnection(t, s.handleResolveMusicAlbum, "GET", "/api/music/resolve?artist=Muse&title=Album", "")
			if w.Code != test.status {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestLocalMusicSearchNeverRequestsProvider(t *testing.T) {
	s := newConnectionTestServer(t)
	s.musicbrainz = metadata.NewMusicBrainzClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("local search reached upstream")
		return artworkResponse(503, []byte(`{}`)), nil
	})}, s.db)
	seedResolveGroup(t, s, "group", "Artist", "Album", "2020")
	w := callConnection(t, s.handleMusicSearch, "GET", "/api/music/search?q=Album&type=album&local=true", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"group"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = callConnection(t, s.handleMusicSearch, "GET", "/api/music/search?q=Song&type=track&local=true", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Code, w.Body.String())
	}
}
