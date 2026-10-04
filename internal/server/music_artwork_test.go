package server

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mediaforge/internal/metadata"
)

const artworkGroup = "11111111-1111-1111-1111-111111111111"

func artworkPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func artworkResponse(code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
}

func TestMusicArtworkFallbackPersistsResolutionAndDiskAcrossRestart(t *testing.T) {
	s := newConnectionTestServer(t)
	dir := t.TempDir()
	s.imgCache = newImageCache(dir)
	pngData := artworkPNG(t)
	if _, err := s.db.Exec(`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid) VALUES(?,'Absolution','Muse','artist')`, artworkGroup); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	counts := map[string]int{}
	s.proxyClient = &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		counts[r.URL.Host]++
		mu.Unlock()
		switch r.URL.Host {
		case "coverartarchive.org":
			return artworkResponse(404, []byte("missing")), nil
		case "api.deezer.com":
			return artworkResponse(200, []byte(`{"data":[{"title":"Absolution","artist":{"name":"Muse"},"cover_xl":"https://e-cdns-images.dzcdn.net/art.jpg"}]}`)), nil
		case "e-cdns-images.dzcdn.net":
			return artworkResponse(200, pngData), nil
		default:
			return nil, fmt.Errorf("unexpected origin %s", r.URL.Host)
		}
	})}
	w := callConnection(t, s.serveMusicArtwork, "GET", "/api/music/cover?rgid="+artworkGroup, "")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pngData) || w.Header().Get("ETag") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/api/music/cover?rgid="+artworkGroup, nil)
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	conditional := httptest.NewRecorder()
	s.serveMusicArtwork(conditional, r)
	if conditional.Code != 304 || conditional.Body.Len() != 0 {
		t.Fatal("conditional cache failed", conditional.Code)
	}
	// New in-memory caches must still reuse the stored resolution and image.
	s.imgCache = newImageCache(dir)
	w = callConnection(t, s.serveMusicArtwork, "GET", "/api/music/cover?rgid="+artworkGroup, "")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pngData) {
		t.Fatal("disk cache failed after restart", w.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, host := range []string{"coverartarchive.org", "api.deezer.com", "e-cdns-images.dzcdn.net"} {
		if counts[host] != 1 {
			t.Fatal("repeat fetch after cache hit", counts)
		}
	}
	var saved string
	s.db.QueryRow(`SELECT data FROM cache WHERE cache_key=?`, "music-cover:v1:"+artworkGroup).Scan(&saved)
	if !strings.Contains(saved, "e-cdns-images.dzcdn.net") {
		t.Fatal("fallback resolution not persisted", saved)
	}
}

func TestMusicArtworkIgnoresExpiredNegativeAndUnsafeLegacyFile(t *testing.T) {
	s := newConnectionTestServer(t)
	dir := t.TempDir()
	s.imgCache = newImageCache(dir)
	pngData := artworkPNG(t)
	requests := 0
	s.proxyClient = &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) { requests++; return artworkResponse(200, pngData), nil })}
	s.coverart = metadata.NewCoverArtClient(s.proxyClient, dir)
	if err := os.WriteFile(s.coverart.CoverPath(artworkGroup), []byte(`<html><script>alert("old cache")</script></html>`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO cache(cache_key,data,expires_at) VALUES(?,'{"missing":true}',?)`, "music-cover:v1:"+artworkGroup, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	w := callConnection(t, s.serveMusicArtwork, "GET", "/api/music/cover?rgid="+artworkGroup, "")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pngData) || requests != 1 {
		t.Fatal("expired negative or unsafe old body served", w.Code, w.Body.String())
	}
}

func TestMusicArtworkCachedResolutionRefetchesOnlyChosenProvider(t *testing.T) {
	s := newConnectionTestServer(t)
	s.imgCache = newImageCache(t.TempDir())
	pngData := artworkPNG(t)
	requests := 0
	s.proxyClient = &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "is1-ssl.mzstatic.com" {
			t.Error("cached resolution repeated provider search", r.URL.Host)
		}
		return artworkResponse(200, pngData), nil
	})}
	if _, err := s.db.Exec(`INSERT INTO cache(cache_key,data,expires_at) VALUES(?,'{"url":"https://is1-ssl.mzstatic.com/cover.jpg"}',datetime('now','+1 day'))`, "music-cover:v1:"+artworkGroup); err != nil {
		t.Fatal(err)
	}
	w := callConnection(t, s.serveMusicArtwork, "GET", "/api/music/cover?rgid="+artworkGroup, "")
	if w.Code != 200 || requests != 1 {
		t.Fatal(w.Code, requests)
	}
}

func TestMusicAlbumRejectsExplicitEditionFromDifferentGroup(t *testing.T) {
	s := newConnectionTestServer(t)
	s.musicbrainz = metadata.NewMusicBrainzClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"group","title":"Album"}`
		if strings.Contains(r.URL.Path, "/release/") {
			body = `{"id":"edition","release-group":{"id":"another"},"media":[{"track-count":1,"tracks":[{"title":"Song"}]}]}`
		}
		return artworkResponse(200, []byte(body)), nil
	})}, s.db)
	w := callDomain(s.handleMusicAlbum, "GET", "/api/music/album/group?release_id=edition", "", "rgid", "group")
	if w.Code != 422 || !strings.Contains(w.Body.String(), "Selected edition") {
		t.Fatal(w.Code, w.Body.String())
	}
}
