package metadata

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"mediaforge/internal/database"
)

const lastfmBase = "https://ws.audioscrobbler.com/2.0/"

// LastFMClient queries the Last.fm API.
type LastFMClient struct {
	client   *http.Client
	apiKey   string
	db       *database.DB
	cacheDir string
}

func NewLastFMClient(client *http.Client, apiKey string, db *database.DB, configDir string) *LastFMClient {
	cacheDir := filepath.Join(configDir, "cache", "covers")
	os.MkdirAll(cacheDir, 0755)
	return &LastFMClient{
		client:   client,
		apiKey:   apiKey,
		db:       db,
		cacheDir: cacheDir,
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
	Name string     `json:"name"`
	MBID string     `json:"mbid"`
	URL  string     `json:"url"`
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
	// Last.fm has no chart.getTopAlbums — use tag.getTopAlbums with rotating popular tags
	tag := trendingTags[(page-1)%len(trendingTags)]

	key := fmt.Sprintf("lastfm:topalbums:%s:%d", tag, page)
	var result LFMTopAlbums
	if c.getCached(key, &result) {
		return &result, nil
	}

	// tag.getTopAlbums returns albums tagged with a given tag, sorted by popularity
	// Response wraps in "albums" not "topalbums", so we decode into LFMTagTopAlbums
	u := fmt.Sprintf("%s?method=tag.getTopAlbums&tag=%s&api_key=%s&format=json&page=%d&limit=50",
		lastfmBase, url.QueryEscape(tag), c.apiKey, page)
	tagResp, err := lastfmGet[LFMTagTopAlbums](c.client, u)
	if err != nil {
		return nil, err
	}

	// Convert to LFMTopAlbums so callers don't need to change
	result.Albums.Album = tagResp.Albums.Album

	c.setCache(key, &result)
	return &result, nil
}

func (c *LastFMClient) TopArtists(page int) (*LFMTopArtists, error) {
	key := fmt.Sprintf("lastfm:topartists:%d", page)
	var result LFMTopArtists
	if c.getCached(key, &result) {
		return &result, nil
	}

	u := fmt.Sprintf("%s?method=chart.getTopArtists&api_key=%s&format=json&page=%d&limit=50",
		lastfmBase, c.apiKey, page)
	resp, err := lastfmGet[LFMTopArtists](c.client, u)
	if err != nil {
		return nil, err
	}

	c.setCache(key, resp)
	return resp, nil
}

func (c *LastFMClient) ArtistTopAlbums(artist string, page int) (*LFMArtistTopAlbums, error) {
	key := fmt.Sprintf("lastfm:artistalbums:%s:%d", artist, page)
	var result LFMArtistTopAlbums
	if c.getCached(key, &result) {
		return &result, nil
	}

	u := fmt.Sprintf("%s?method=artist.getTopAlbums&artist=%s&api_key=%s&format=json&page=%d&limit=50",
		lastfmBase, url.QueryEscape(artist), c.apiKey, page)
	resp, err := lastfmGet[LFMArtistTopAlbums](c.client, u)
	if err != nil {
		return nil, err
	}

	c.setCache(key, resp)
	return resp, nil
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

// -- DB cache helpers --

func (c *LastFMClient) getCached(key string, target interface{}) bool {
	var data string
	var expiresAt string
	err := c.db.QueryRow(`SELECT data, expires_at FROM cache WHERE cache_key = ?`, key).Scan(&data, &expiresAt)
	if err != nil {
		return false
	}

	t, err := time.Parse("2006-01-02 15:04:05", expiresAt)
	if err != nil || time.Now().After(t) {
		c.db.Exec(`DELETE FROM cache WHERE cache_key = ?`, key)
		return false
	}

	if err := json.Unmarshal([]byte(data), target); err != nil {
		return false
	}
	return true
}

func (c *LastFMClient) setCache(key string, value interface{}) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	expires := time.Now().Add(48 * time.Hour).Format("2006-01-02 15:04:05")
	c.db.Exec(`INSERT OR REPLACE INTO cache (cache_key, data, expires_at) VALUES (?, ?, ?)`,
		key, string(data), expires)
}

func lastfmGet[T any](client *http.Client, rawURL string) (*T, error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("lastfm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lastfm %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lastfm decode: %w", err)
	}
	return &result, nil
}
