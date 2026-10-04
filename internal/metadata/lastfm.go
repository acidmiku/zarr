package metadata

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/time/rate"
	"mediaforge/internal/database"
)

const lastfmBase = "https://ws.audioscrobbler.com/2.0/"

// LastFMClient queries the Last.fm API.
type LastFMClient struct {
	client   *http.Client
	apiKey   string
	db       *database.DB
	cacheDir string
	cache    *musicMetadataCache
}

func NewLastFMClient(client *http.Client, apiKey string, db *database.DB, configDir string) *LastFMClient {
	cacheDir := filepath.Join(configDir, "cache", "covers")
	os.MkdirAll(cacheDir, 0755)
	return &LastFMClient{
		client:   client,
		apiKey:   apiKey,
		db:       db,
		cacheDir: cacheDir,
		cache:    newMusicMetadataCache(client, db, rate.NewLimiter(3, 1)),
	}
}

// -- Response types --

type LFMTopAlbums struct {
	Albums struct {
		Album []LFMAlbum `json:"album"`
	} `json:"topalbums"`
}

type LFMTopArtists struct {
	Artists struct {
		Artist []LFMArtist `json:"artist"`
	} `json:"topartists"`
}

type LFMArtistTopAlbums struct {
	Albums struct {
		Album []LFMAlbum `json:"album"`
	} `json:"topalbums"`
}

type LFMTagTopAlbums struct {
	Albums struct {
		Album []LFMAlbum `json:"album"`
	} `json:"albums"`
}

type LFMAlbum struct {
	Name      string     `json:"name"`
	Artist    LFMArtist  `json:"artist"`
	MBID      string     `json:"mbid"`
	URL       string     `json:"url"`
	Playcount string     `json:"playcount"`
	Image     []LFMImage `json:"image"`
}

type LFMArtist struct {
	Name  string     `json:"name"`
	MBID  string     `json:"mbid"`
	URL   string     `json:"url"`
	Image []LFMImage `json:"image"`
}

type LFMImage struct {
	Text string `json:"#text"`
	Size string `json:"size"`
}

// -- Methods --

// trendingTags are rotated by page to give diverse trending results.
var trendingTags = []string{"pop", "rock", "electronic", "hip-hop", "indie", "r&b", "alternative", "metal", "jazz", "folk"}

func (c *LastFMClient) TopAlbums(page int) (*LFMTopAlbums, error) {
	if page < 1 {
		page = 1
	}
	// Last.fm has no chart.getTopAlbums — use tag.getTopAlbums with rotating popular tags
	tag := trendingTags[(page-1)%len(trendingTags)]

	var result LFMTopAlbums

	// tag.getTopAlbums returns albums tagged with a given tag, sorted by popularity
	// Response wraps in "albums" not "topalbums", so we decode into LFMTagTopAlbums
	u := fmt.Sprintf("%s?method=tag.getTopAlbums&tag=%s&api_key=%s&format=json&page=%d&limit=50",
		lastfmBase, url.QueryEscape(tag), c.apiKey, page)
	tagResp, err := lastfmCached[LFMTagTopAlbums](context.Background(), c, u)
	if err != nil {
		return nil, err
	}

	// Convert to LFMTopAlbums so callers don't need to change
	result.Albums.Album = tagResp.Albums.Album

	return &result, nil
}

func (c *LastFMClient) TopArtists(page int) (*LFMTopArtists, error) {
	if page < 1 {
		page = 1
	}

	u := fmt.Sprintf("%s?method=chart.getTopArtists&api_key=%s&format=json&page=%d&limit=50",
		lastfmBase, c.apiKey, page)
	resp, err := lastfmCached[LFMTopArtists](context.Background(), c, u)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *LastFMClient) ArtistTopAlbums(artist string, page int) (*LFMArtistTopAlbums, error) {
	if page < 1 {
		page = 1
	}

	u := fmt.Sprintf("%s?method=artist.getTopAlbums&artist=%s&api_key=%s&format=json&page=%d&limit=50",
		lastfmBase, url.QueryEscape(artist), c.apiKey, page)
	resp, err := lastfmCached[LFMArtistTopAlbums](context.Background(), c, u)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

type LFMSimilarArtists struct {
	Similar struct {
		Artist []LFMArtist `json:"artist"`
	} `json:"similarartists"`
}

func (c *LastFMClient) similarURL(artist string, limit int) string {
	if limit < 1 || limit > 50 {
		limit = 12
	}
	return fmt.Sprintf("%s?method=artist.getsimilar&artist=%s&api_key=%s&format=json&limit=%d", lastfmBase, url.QueryEscape(artist), url.QueryEscape(c.apiKey), limit)
}
func (c *LastFMClient) SimilarArtists(artist string, limit int) ([]LFMArtist, error) {
	return c.SimilarArtistsContext(context.Background(), artist, limit)
}
func (c *LastFMClient) SimilarArtistsContext(ctx context.Context, artist string, limit int) ([]LFMArtist, error) {
	result, err := lastfmCached[LFMSimilarArtists](ctx, c, c.similarURL(artist, limit))
	if err != nil {
		return nil, err
	}
	return result.Similar.Artist, nil
}
func (c *LastFMClient) CachedSimilarArtists(artist string, limit int) []LFMArtist {
	snapshot := c.cache.read(musicCacheKey(c.similarURL(artist, limit)))
	var result LFMSimilarArtists
	if json.Unmarshal(snapshot.data, &result) != nil {
		return []LFMArtist{}
	}
	return result.Similar.Artist
}

func (c *LastFMClient) userTopAlbumsURL(username string) string {
	return fmt.Sprintf("%s?method=user.gettopalbums&user=%s&api_key=%s&format=json&period=6month&limit=50", lastfmBase, url.QueryEscape(strings.TrimSpace(username)), url.QueryEscape(c.apiKey))
}
func (c *LastFMClient) UserTopAlbumsContext(ctx context.Context, username string) ([]LFMAlbum, error) {
	if strings.TrimSpace(username) == "" {
		return []LFMAlbum{}, nil
	}
	result, err := lastfmCached[LFMTopAlbums](ctx, c, c.userTopAlbumsURL(username))
	if err != nil {
		return nil, err
	}
	return result.Albums.Album, nil
}
func (c *LastFMClient) CachedUserTopAlbums(username string) []LFMAlbum {
	if strings.TrimSpace(username) == "" {
		return []LFMAlbum{}
	}
	snapshot := c.cache.read(musicCacheKey(c.userTopAlbumsURL(username)))
	var result LFMTopAlbums
	if json.Unmarshal(snapshot.data, &result) != nil {
		return []LFMAlbum{}
	}
	return result.Albums.Album
}
func lastfmCached[T any](ctx context.Context, c *LastFMClient, rawURL string) (*T, error) {
	data, err := c.cache.get(ctx, rawURL, 48*time.Hour)
	if err != nil {
		return nil, err
	}
	var result T
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid Last.fm response")
	}
	return &result, nil
}

// CacheImage downloads and caches an image URL from Last.fm.
// Returns the local file path.
func (c *LastFMClient) CacheImage(imageURL string) (string, error) {
	if imageURL == "" {
		return "", nil
	}

	hash := fmt.Sprintf("%x", md5.Sum([]byte(imageURL)))
	cached := filepath.Join(c.cacheDir, "lastfm_"+hash+".jpg")

	if _, err := os.Stat(cached); err == nil {
		return cached, nil
	}

	resp, err := c.client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("lastfm image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lastfm image %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("lastfm image read: %w", err)
	}

	if err := os.WriteFile(cached, data, 0644); err != nil {
		slog.Warn("lastfm image cache failed", "error", err)
		return "", err
	}

	return cached, nil
}

// BestImage returns the largest image URL from a set of Last.fm images.
func BestImage(images []LFMImage) string {
	// Prefer extralarge > large > medium > small
	for _, size := range []string{"extralarge", "large", "medium", "small"} {
		for _, img := range images {
			if img.Size == size && img.Text != "" {
				return img.Text
			}
		}
	}
	return ""
}
