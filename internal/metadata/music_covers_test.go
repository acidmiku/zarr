package metadata

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFallbackCoverRequiresBothExactArtistAndAlbum(t *testing.T) {
	for _, test := range []struct {
		artist, title, candidateArtist, candidateTitle string
		want                                           bool
	}{
		{"Muse", "Origin of Symmetry", "Museum", "Origin of Symmetry", false},
		{"Muse", "Origin of Symmetry", "Muse", "Origin of Symmetry (Live)", false},
		{"Björk", "Homogenic", "BJORK", "Homogenic", true},
		{"AC/DC", "Back in Black", "AC DC", "Back In Black", true},
		{"", "Album", "", "Album", false},
	} {
		if got := confidentCoverMatch(test.artist, test.title, test.candidateArtist, test.candidateTitle); got != test.want {
			t.Fatalf("match %#v = %v", test, got)
		}
	}
}

func TestFallbackCoverRejectsUntrustedImageHosts(t *testing.T) {
	for _, raw := range []string{"http://e-cdns-images.dzcdn.net/a.jpg", "https://e-cdns-images.dzcdn.net.evil.test/a.jpg", "https://evil.test/dzcdn.net/a.jpg", "https://user@e-cdns-images.dzcdn.net/a.jpg", "https://e-cdns-images.dzcdn.net:8443/a.jpg"} {
		if verifiedMusicImageURL(raw, "dzcdn.net") {
			t.Fatal("accepted untrusted image", raw)
		}
	}
	if !verifiedMusicImageURL("https://e-cdns-images.dzcdn.net/a.jpg", "dzcdn.net") {
		t.Fatal("valid CDN rejected")
	}
}

func TestFallbackCoverSkipsLooseMatchThenCachesVerifiedItunes(t *testing.T) {
	db := musicTestDB(t)
	var requests atomic.Int32
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Host == "api.deezer.com" {
			return musicJSON(200, `{"data":[{"title":"Absolution","artist":{"name":"Museum"},"cover_xl":"https://e-cdns-images.dzcdn.net/wrong.jpg"}]}`), nil
		}
		if r.URL.Host != "itunes.apple.com" {
			t.Error("resolver downloaded an image", r.URL.Host)
		}
		return musicJSON(200, `{"results":[{"artistName":"Muse","collectionName":"Absolution","artworkUrl100":"https://is1-ssl.mzstatic.com/image/thumb/cover/100x100bb.jpg"}]}`), nil
	})}
	for i := 0; i < 2; i++ {
		got, err := ResolveFallbackCover(context.Background(), client, db, "Muse", "Absolution")
		if err != nil || !strings.Contains(got, "600x600bb.jpg") {
			t.Fatalf("verified fallback missing: %s %v", got, err)
		}
	}
	if requests.Load() != 2 {
		t.Fatal("provider responses not cached", requests.Load())
	}
}

func TestSimilarArtistsCachedLocallyAndErrorsRedacted(t *testing.T) {
	db := musicTestDB(t)
	var requests atomic.Int32
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return musicJSON(200, `{"similarartists":{"artist":[{"name":"Other Artist","mbid":"other"}]}}`), nil
	})}
	c := NewLastFMClient(client, "private-api-key", db, t.TempDir())
	if got := c.CachedSimilarArtists("Artist", 12); len(got) != 0 || requests.Load() != 0 {
		t.Fatal("local read called upstream")
	}
	if got, err := c.SimilarArtists("Artist", 12); err != nil || len(got) != 1 {
		t.Fatalf("similar artists %#v %v", got, err)
	}
	c = NewLastFMClient(client, "private-api-key", db, t.TempDir())
	if got := c.CachedSimilarArtists("Artist", 12); len(got) != 1 || requests.Load() != 1 {
		t.Fatal("cached similar artists unavailable")
	}
	var key string
	db.QueryRow(`SELECT cache_key FROM cache LIMIT 1`).Scan(&key)
	if strings.Contains(key, "private-api-key") {
		t.Fatal("raw key stored in cache key")
	}
	c = NewLastFMClient(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		return musicJSON(200, `{"error":10,"message":"invalid key private-api-key"}`), nil
	})}, "different-key", db, t.TempDir())
	if _, err := c.SimilarArtists("Artist", 12); err == nil || strings.Contains(err.Error(), "private-api-key") {
		t.Fatalf("API error unhandled or leaked: %v", err)
	}
}

func TestLastFMListeningHistoryIsOptionalCachedAndScoped(t *testing.T) {
	db := musicTestDB(t)
	var requests atomic.Int32
	client := &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		q := r.URL.Query()
		if q.Get("method") != "user.gettopalbums" || q.Get("period") != "6month" || q.Get("limit") != "50" || q.Get("user") != "listener" {
			t.Error("incorrect history scope", q.Get("method"), q.Get("period"), q.Get("limit"), q.Get("user"))
		}
		return musicJSON(200, `{"topalbums":{"album":[{"name":"Album","mbid":"edition-not-group","artist":{"name":"Artist","mbid":"artist"},"playcount":"12"}]}}`), nil
	})}
	c := NewLastFMClient(client, "private-key", db, t.TempDir())
	if result, err := c.UserTopAlbumsContext(context.Background(), ""); err != nil || len(result) != 0 || requests.Load() != 0 {
		t.Fatal("empty username made a request")
	}
	if result, err := c.UserTopAlbumsContext(context.Background(), "listener"); err != nil || len(result) != 1 {
		t.Fatalf("history unavailable %v %#v", err, result)
	}
	c = NewLastFMClient(client, "private-key", db, t.TempDir())
	if got := c.CachedUserTopAlbums("listener"); len(got) != 1 || got[0].Playcount != "12" || requests.Load() != 1 {
		t.Fatal("history not persisted", got)
	}
	if got := c.CachedUserTopAlbums("another-listener"); len(got) != 0 {
		t.Fatal("history leaked across usernames", got)
	}
}
