package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mediaforge/internal/metadata"
)

func TestMusicDiscoveryEmptyArraysWithoutOptionalProviders(t *testing.T) {
	s := newConnectionTestServer(t)
	w := callConnection(t, s.handleMusicDiscover, "GET", "/api/music/discover", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var got musicDiscoveryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Recommendations == nil || got.RecentlySaved == nil || got.SimilarArtists == nil || got.Configured["lastfm"] {
		t.Fatal(w.Body.String())
	}
}

func TestMusicDiscoveryHistoryUsesUniqueCatalogIdentityExcludesOwned(t *testing.T) {
	s := newConnectionTestServer(t)
	s.db.SetSetting("lastfm_username", "listener")
	requests := 0
	client := &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return artworkResponse(200, []byte(`{"topalbums":{"album":[{"name":"Owned","artist":{"name":"Artist"}},{"name":"Ambiguous","artist":{"name":"Artist"}},{"name":"Unowned","mbid":"untrusted-edition-id","artist":{"name":"Muse"},"playcount":"42"},{"name":"Unknown","artist":{"name":"Artist"}}]}}`)), nil
	})}
	s.lastfm = metadata.NewLastFMClient(client, "key", s.db, t.TempDir())
	if _, err := s.lastfm.UserTopAlbumsContext(context.Background(), "listener"); err != nil {
		t.Fatal(err)
	}
	musicEnrichments.Store(s.db, &musicEnrichmentState{last: time.Now()})
	t.Cleanup(func() { musicEnrichments.Delete(s.db) })
	for _, q := range []string{
		`INSERT INTO artists(id,mbid,name) VALUES(1,'artist','Artist')`,
		`INSERT INTO albums(artist_id,title,release_group_id) VALUES(1,'Owned','owned')`,
		`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid) VALUES('owned','Owned','Artist','artist'),('ambiguous-a','Ambiguous','Artist','artist'),('ambiguous-b','Ambiguous','Artist','artist'),('canonical','Unowned','Muse','muse'),('wrong-artist','Unowned','Museum','museum')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w := callConnection(t, s.handleMusicDiscover, "GET", "/api/music/discover", "")
	var got musicDiscoveryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(got.PlayedNotOwned) != 1 || got.PlayedNotOwned[0].ID != "canonical" || !strings.Contains(got.PlayedNotOwned[0].Reason, "42 times") || !got.Configured["lastfm_history"] || requests != 1 {
		t.Fatal(w.Code, w.Body.String(), requests)
	}
}

func TestMusicDiscoveryUsesRatingIdentityAndGenreEvidence(t *testing.T) {
	s := newConnectionTestServer(t)
	queries := []string{
		`INSERT INTO artists(id,mbid,name) VALUES(1,'favorite-artist','Muse'),(2,'disliked-artist','Other')`,
		`INSERT INTO albums(id,artist_id,title,release_group_id,rating,favorite,year) VALUES(1,1,'Absolution','saved',5,1,2003),(2,2,'Disliked','disliked',1,0,2020)`,
		`INSERT INTO music_catalog_artists(mbid,name,genres) VALUES('favorite-artist','Muse','["alternative rock"]'),('similar-name','Museum','["classical"]')`,
		`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid,year,genres) VALUES('saved','Absolution','Muse','favorite-artist',2003,'[]'),('same-artist','Origin','Muse','favorite-artist',2001,'[]'),('same-genre','Different Artist Album','Another','another',2024,'["alternative rock"]'),('unrelated','Name Collision','Museum','similar-name',2025,'["classical"]'),('low-rated','Unwanted','Other','disliked-artist',2026,'[]')`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w := callConnection(t, s.handleMusicDiscover, "GET", "/api/music/discover", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var got musicDiscoveryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Recommendations) != 2 || got.Recommendations[0].ID != "same-artist" || got.Recommendations[1].ID != "same-genre" {
		t.Fatal(w.Body.String())
	}
	if !strings.Contains(got.Recommendations[0].Reason, "Absolution") || !strings.Contains(got.Recommendations[1].Reason, "alternative rock") {
		t.Fatal("missing evidence", w.Body.String())
	}
	if got.Recommendations[0].CoverURL != "/api/music/cover?rgid=same-artist" || got.Recommendations[0].ArtistMBID != "favorite-artist" || got.Recommendations[0].InLibrary {
		t.Fatal("noncanonical recommendation", w.Body.String())
	}
	if len(got.RecentlySaved) != 2 || got.RecentlySaved[1].LibraryID != 1 || !got.RecentlySaved[1].Favorite || !got.RecentlySaved[1].InLibrary {
		t.Fatal("saved status missing", w.Body.String())
	}
	var albums int
	s.db.QueryRow(`SELECT count(*) FROM albums`).Scan(&albums)
	if albums != 2 {
		t.Fatal("discovery changed library")
	}
}

func TestMusicDiscoveryWithoutLibraryOffersCachedCatalog(t *testing.T) {
	s := newConnectionTestServer(t)
	if _, err := s.db.Exec(`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid,release_type) VALUES('ep','An EP','Artist','artist','EP')`); err != nil {
		t.Fatal(err)
	}
	w := callConnection(t, s.handleMusicDiscover, "GET", "/api/music/discover", "")
	var got musicDiscoveryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Recommendations) != 1 || got.Recommendations[0].Type != "EP" || got.Recommendations[0].Reason != "From your cached catalog" || got.Enriching {
		t.Fatal(w.Body.String())
	}
}

func TestMusicDiscoveryPrefersAlbumsOverRecentBroadcastsAndSingles(t *testing.T) {
	s := newConnectionTestServer(t)
	if _, err := s.db.Exec(`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid,release_type,year) VALUES
		('broadcast','Radio session','Artist','artist','Broadcast',2026),
		('single','New single','Artist','artist','Single',2025),
		('ep','An EP','Artist','artist','EP',2024),
		('album','An album','Artist','artist','Album',2001)`); err != nil {
		t.Fatal(err)
	}
	w := callConnection(t, s.handleMusicDiscover, "GET", "/api/music/discover", "")
	var got musicDiscoveryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(got.Recommendations) != 3 || got.Recommendations[0].ID != "album" || got.Recommendations[1].ID != "ep" || got.Recommendations[2].ID != "single" || got.CatalogCount != 4 {
		t.Fatal(w.Code, w.Body.String())
	}
}
