package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"mediaforge/internal/database"
)

func musicTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func musicJSON(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestMusicMetadataPersistentCacheStaleAndCooldown(t *testing.T) {
	db := musicTestDB(t)
	var requests atomic.Int32
	var failing atomic.Bool
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if failing.Load() {
			resp := musicJSON(503, `{"error":"outage"}`)
			resp.Header.Set("Retry-After", "60")
			return resp, nil
		}
		return musicJSON(200, `{"artists":[{"id":"one","name":"Artist"}],"count":1}`), nil
	})}
	c := NewMusicBrainzClient(client, db)
	if _, err := c.SearchArtists("Artist"); err != nil {
		t.Fatal(err)
	}
	// A second client simulates a process restart: no shared memory cache.
	c = NewMusicBrainzClient(client, db)
	if _, err := c.SearchArtists("Artist"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("cache did not survive a fresh client", requests.Load())
	}
	if _, err := db.Exec(`UPDATE cache SET expires_at='2000-01-01 00:00:00'`); err != nil {
		t.Fatal(err)
	}
	failing.Store(true)
	if result, err := c.SearchArtists("Artist"); err != nil || len(result.Artists) != 1 {
		t.Fatalf("stale snapshot lost: %#v %v", result, err)
	}
	c = NewMusicBrainzClient(client, db)
	if _, err := c.SearchArtists("Artist"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("Retry-After not persisted across clients", requests.Load())
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM cache WHERE cache_key LIKE 'music-metadata:%'`).Scan(&count)
	if count != 1 {
		t.Fatal("stale record was deleted", count)
	}
}

func TestMusicMetadataDeduplicatesConcurrentRequests(t *testing.T) {
	var requests atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			close(entered)
		}
		<-release
		return musicJSON(200, `{"artists":[],"count":0}`), nil
	})}
	c := NewMusicBrainzClient(client)
	c.limiter.SetLimit(rate.Inf)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := c.SearchArtists("same"); errs <- err }()
	}
	<-entered
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("duplicate upstream requests", requests.Load())
	}
}

func TestMusicCachePermanentErrorAndCancellationDoNotReturnStale(t *testing.T) {
	cache := newMusicMetadataCache(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) { return musicJSON(404, `{}`), nil })}, nil, nil)
	u := "https://musicbrainz.org/missing"
	cache.save(musicCacheKey(u), []byte(`{"id":"old"}`), -time.Hour)
	if _, err := cache.get(context.Background(), u, time.Hour); err == nil {
		t.Fatal("404 incorrectly returned deleted metadata")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache.client = &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	if _, err := cache.get(ctx, u, time.Hour); err == nil {
		t.Fatal("canceled caller received stale success")
	}
	if until := cache.backoff["musicbrainz.org"]; !until.IsZero() {
		t.Fatal("caller cancellation imposed provider cooldown", until)
	}
}

func TestDiscographyPaginatesSinglesAndEPsAndPreservesAliases(t *testing.T) {
	db := musicTestDB(t)
	var offsets []string
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("type") != "" {
			t.Error("discography still restricted to albums")
		}
		offsets = append(offsets, r.URL.Query().Get("offset"))
		groups := []MBReleaseGroup{}
		start, end := 0, 100
		if r.URL.Query().Get("offset") == "100" {
			start, end = 100, 102
		}
		for i := start; i < end; i++ {
			kind := "Album"
			if i == 100 {
				kind = "Single"
			}
			if i == 101 {
				kind = "EP"
			}
			groups = append(groups, MBReleaseGroup{ID: fmt.Sprint(i), Title: fmt.Sprintf("Release %d", i), PrimaryType: kind, FirstRelease: "2020"})
		}
		body, _ := json.Marshal(map[string]any{"release-group-count": 102, "release-groups": groups})
		return musicJSON(200, string(body)), nil
	})}
	c := NewMusicBrainzClient(client, db)
	c.limiter.SetLimit(rate.Inf)
	c.ingestArtist(MBArtist{ID: "artist", Name: "Björk", Aliases: []MBAlias{{Name: "ビョーク"}}, Genres: []MBTag{{Name: "art pop", Count: 5}}}, true)
	result, err := c.SearchArtistReleaseGroups("artist")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ReleaseGroups) != 102 || !result.Complete || strings.Join(offsets, ",") != "0,100" {
		t.Fatalf("incomplete discography: %d %#v", len(result.ReleaseGroups), offsets)
	}
	if groups := c.SearchCatalogReleaseGroups("ビョーク", 1000); len(groups) != 102 {
		t.Fatalf("aliases lost through stub ingestion: %d", len(groups))
	}
	if got := c.cachedArtist("artist"); len(got.Aliases) != 1 || len(got.Genres) != 1 {
		t.Fatalf("artist enrichment overwritten: %#v", got)
	}
	kinds := map[string]bool{}
	for _, group := range result.ReleaseGroups {
		kinds[group.PrimaryType] = true
	}
	if !kinds["Single"] || !kinds["EP"] {
		t.Fatal("non-album releases omitted")
	}
}

func TestDiscographyDoesNotClaimCompleteWhenUpstreamTruncates(t *testing.T) {
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"release-group-count":2,"release-groups":[{"id":"one","title":"One"}]}`
		if r.URL.Query().Get("offset") == "1" {
			body = `{"release-group-count":2,"release-groups":[]}`
		}
		return musicJSON(200, body), nil
	})}
	c := NewMusicBrainzClient(client)
	c.limiter.SetLimit(rate.Inf)
	if _, err := c.SearchArtistReleaseGroups("artist"); err == nil {
		t.Fatal("truncated upstream response reported as complete")
	}
}

func TestDefaultEditionPrefersOfficialStandardAndLoadsTrackDetails(t *testing.T) {
	var detailRequests int
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/release") {
			return musicJSON(200, `{"release-count":4,"releases":[{"id":"deluxe","title":"Album (Deluxe)","status":"Official","media":[{"format":"Digital Media","track-count":30}]},{"id":"bootleg","title":"Album","status":"Bootleg","media":[{"format":"Digital Media","track-count":20}]},{"id":"cd","title":"Album","status":"Official","date":"2000","media":[{"format":"CD","track-count":10}]},{"id":"standard","title":"Album","status":"Official","date":"2001","media":[{"format":"Digital Media","track-count":1}]}]}`), nil
		}
		detailRequests++
		if !strings.HasSuffix(r.URL.Path, "/standard") {
			t.Error("wrong edition chosen", r.URL.Path)
		}
		return musicJSON(200, `{"id":"standard","title":"Album","status":"Official","release-group":{"id":"group"},"media":[{"format":"Digital Media","track-count":1,"tracks":[{"recording":{"id":"recording","title":"Song","length":123456}}]}]}`), nil
	})}
	c := NewMusicBrainzClient(client)
	c.limiter.SetLimit(rate.Inf)
	release, err := c.GetBestRelease("group")
	if err != nil {
		t.Fatal(err)
	}
	if release.ID != "standard" || release.TrackCount != 1 || release.Media[0].Tracks[0].Title != "Song" || release.Media[0].Tracks[0].Length != 123456 || detailRequests != 1 {
		t.Fatalf("edition details incomplete: %#v", release)
	}
	if _, err := c.GetReleaseForGroup("other-group", "standard"); err == nil {
		t.Fatal("selected edition from wrong album accepted")
	}
	if detailRequests != 1 {
		t.Fatal("edition lookup did not use cache")
	}
}

func TestEditionSortStableAcrossInputOrders(t *testing.T) {
	for _, ids := range [][]string{{"z", "a"}, {"a", "z"}} {
		releases := []MBRelease{}
		for _, id := range ids {
			releases = append(releases, MBRelease{ID: id, Title: "Album", Status: "Official", Date: "2020", Media: []MBMedia{{Format: "CD"}}})
		}
		SortReleases(releases)
		if releases[0].ID != "a" {
			t.Fatal("unstable edition choice")
		}
	}
}

func TestArtistSearchMergesLocalHistoricalAliases(t *testing.T) {
	db := musicTestDB(t)
	c := NewMusicBrainzClient(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) { return musicJSON(200, `{"artists":[],"count":0}`), nil })}, db)
	c.ingestArtist(MBArtist{ID: "artist", Name: "Current Name", Aliases: []MBAlias{{Name: "Old Name", Ended: true}}}, true)
	got, err := c.SearchArtists("Old Name")
	if err != nil || len(got.Artists) != 1 || got.Artists[0].ID != "artist" {
		t.Fatalf("historical alias disappeared %#v %v", got, err)
	}
}

func TestSelectedEditionRejectsInvalidTrackIdentity(t *testing.T) {
	for _, test := range []struct{ name, media string }{
		{"empty title", `[{"position":1,"track-count":1,"tracks":[{"position":1,"title":" "}]}]`},
		{"duplicate positions", `[{"position":1,"track-count":2,"tracks":[{"position":1,"title":"First"},{"position":1,"title":"Second"}]}]`},
		{"duplicate discs", `[{"position":1,"track-count":1,"tracks":[{"position":1,"title":"First"}]},{"position":1,"track-count":1,"tracks":[{"position":1,"title":"Second"}]}]`},
		{"incomplete count", `[{"position":1,"track-count":2,"tracks":[{"position":1,"title":"First"}]}]`},
		{"negative position", `[{"position":1,"track-count":1,"tracks":[{"position":-1,"title":"First"}]}]`},
		{"negative disc", `[{"position":-1,"track-count":1,"tracks":[{"position":1,"title":"First"}]}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewMusicBrainzClient(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
				return musicJSON(200, `{"id":"edition","release-group":{"id":"group"},"media":`+test.media+`}`), nil
			})})
			if _, err := c.GetReleaseForGroup("group", "edition"); err == nil {
				t.Fatal("invalid tracklist accepted")
			}
		})
	}
}
