package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"mediaforge/internal/database"
)

type fallbackCacheIdentity struct {
	client *http.Client
	db     *database.DB
}

var fallbackMetadataCaches sync.Map

// ResolveFallbackCover returns a name-verified CDN URL for the existing server
// image proxy. It never downloads images in a browser, accepts artist substring
// matches, or treats a provider's first result as proof of album identity.
func ResolveFallbackCover(ctx context.Context, client *http.Client, db *database.DB, artist, title string) (string, error) {
	if NormalizeMusicText(artist) == "" || NormalizeMusicText(title) == "" {
		return "", fmt.Errorf("artist and album title required")
	}
	key := fallbackCacheIdentity{client, db}
	loaded, _ := fallbackMetadataCaches.LoadOrStore(key, newMusicMetadataCache(client, db, rate.NewLimiter(1, 1)))
	cache := loaded.(*musicMetadataCache)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	deezerURL := "https://api.deezer.com/search/album?q=" + url.QueryEscape(`artist:"`+artist+`" album:"`+title+`"`) + "&limit=10"
	if data, err := cache.get(ctx, deezerURL, 7*24*time.Hour); err == nil {
		var response struct {
			Data []struct {
				Title  string `json:"title"`
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
				CoverXL  string `json:"cover_xl"`
				CoverBig string `json:"cover_big"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &response) == nil {
			for _, album := range response.Data {
				if confidentCoverMatch(artist, title, album.Artist.Name, album.Title) {
					for _, imageURL := range []string{album.CoverXL, album.CoverBig} {
						if verifiedMusicImageURL(imageURL, "dzcdn.net") {
							return imageURL, nil
						}
					}
				}
			}
		}
	}
	itunesURL := "https://itunes.apple.com/search?media=music&entity=album&limit=10&term=" + url.QueryEscape(artist+" "+title)
	if data, err := cache.get(ctx, itunesURL, 7*24*time.Hour); err == nil {
		var response struct {
			Results []struct {
				Artist  string `json:"artistName"`
				Title   string `json:"collectionName"`
				Artwork string `json:"artworkUrl100"`
			} `json:"results"`
		}
		if json.Unmarshal(data, &response) == nil {
			for _, album := range response.Results {
				if confidentCoverMatch(artist, title, album.Artist, album.Title) && verifiedMusicImageURL(album.Artwork, "mzstatic.com") {
					return strings.Replace(album.Artwork, "100x100bb", "600x600bb", 1), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no verified alternate cover found")
}
func confidentCoverMatch(artist, title, candidateArtist, candidateTitle string) bool {
	a, b := NormalizeMusicText(artist), NormalizeMusicText(candidateArtist)
	x, y := NormalizeMusicText(title), NormalizeMusicText(candidateTitle)
	return a != "" && x != "" && a == b && x == y
}
func verifiedMusicImageURL(raw, domain string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == domain || strings.HasSuffix(host, "."+domain)
}
