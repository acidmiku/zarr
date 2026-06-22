package indexer

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// NewznabClient queries Usenet indexers via the Newznab API protocol.
type NewznabClient struct {
	client *http.Client
}

func NewNewznabClient(client *http.Client) *NewznabClient {
	return &NewznabClient{client: client}
}

// IndexerConfig holds the configuration for a single indexer.
type IndexerConfig struct {
	ID           int
	Name         string
	URL          string
	APIKey       string
	Priority     int
	Enabled      bool
	Type         string   // "newznab" or "rutracker"
	Username     string
	Password     string
	ContentTypes []string // e.g. ["movie","series","anime","music"]
}

// NewznabResponse represents the RSS/XML response from a Newznab API.
type NewznabResponse struct {
	XMLName xml.Name      `xml:"rss"`
	Channel NewznabChannel `xml:"channel"`
}

type NewznabChannel struct {
	Items []NewznabItem `xml:"item"`
}

type NewznabItem struct {
	Title     string           `xml:"title"`
	Link      string           `xml:"link"`
	Size      int64            `xml:"-"`
	Category  string           `xml:"-"`
	Enclosure NewznabEnclosure `xml:"enclosure"`
	Attrs     []NewznabAttr    `xml:"attr"`
}

type NewznabEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type NewznabAttr struct {
	XMLName xml.Name `xml:"attr"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:"value,attr"`
}

// buildURL constructs a Newznab API URL, handling base URLs that may or may
// not already contain query parameters or a trailing slash.
func buildURL(base string, params map[string]string) string {
	base = strings.TrimRight(base, "/")
	u, err := url.Parse(base)
	if err != nil {
		// Fallback: append directly
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		return base + "?" + q.Encode()
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// SearchMovieByIMDB searches for a movie by IMDB ID.
func (c *NewznabClient) SearchMovieByIMDB(idx IndexerConfig, imdbID string) ([]NewznabItem, error) {
	id := strings.TrimPrefix(imdbID, "tt")
	u := buildURL(idx.URL, map[string]string{
		"t": "movie", "imdbid": "tt" + id, "apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// SearchMovieByText searches for a movie by text query.
func (c *NewznabClient) SearchMovieByText(idx IndexerConfig, query string) ([]NewznabItem, error) {
	u := buildURL(idx.URL, map[string]string{
		"t": "search", "q": query, "apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// SearchTVByTVDB searches for a TV episode by TVDB ID.
func (c *NewznabClient) SearchTVByTVDB(idx IndexerConfig, tvdbID, season, episode int) ([]NewznabItem, error) {
	u := buildURL(idx.URL, map[string]string{
		"t": "tvsearch", "tvdbid": strconv.Itoa(tvdbID),
		"season": strconv.Itoa(season), "ep": strconv.Itoa(episode),
		"apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// SearchTVByText searches for a TV episode by text query.
func (c *NewznabClient) SearchTVByText(idx IndexerConfig, query string) ([]NewznabItem, error) {
	u := buildURL(idx.URL, map[string]string{
		"t": "tvsearch", "q": query, "apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// SearchByText does a general text search.
func (c *NewznabClient) SearchByText(idx IndexerConfig, query string) ([]NewznabItem, error) {
	u := buildURL(idx.URL, map[string]string{
		"t": "search", "q": query, "apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// fetchRaw performs an HTTP GET. Retries on transport errors / 5xx / 429 are
// handled by the retry transport wrapping c.client.
func (c *NewznabClient) fetchRaw(rawURL, indexerName string) ([]byte, error) {
	resp, err := c.client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("newznab request to %s: %w", indexerName, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", indexerName, err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("newznab %s HTTP %d: %s", indexerName, resp.StatusCode, snippet)
	}
	return body, nil
}

// TestConnection tests connectivity to an indexer.
func (c *NewznabClient) TestConnection(idx IndexerConfig) error {
	u := buildURL(idx.URL, map[string]string{
		"t": "caps", "apikey": idx.APIKey,
	})
	resp, err := c.client.Get(u)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", idx.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", idx.Name, resp.StatusCode)
	}
	return nil
}

func (c *NewznabClient) fetch(rawURL, indexerName string) ([]NewznabItem, error) {
	slog.Debug("newznab query", "indexer", indexerName, "url", rawURL)

	data, err := c.fetchRaw(rawURL, indexerName)
	if err != nil {
		return nil, err
	}

	var nzbResp NewznabResponse
	if err := xml.Unmarshal(data, &nzbResp); err != nil {
		// Log a snippet of the response body to help diagnose
		snippet := string(data)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		slog.Warn("unexpected response from indexer", "indexer", indexerName, "body_preview", snippet)
		return nil, fmt.Errorf("parse response from %s: %w", indexerName, err)
	}

	// Extract size and category from newznab attributes
	for i := range nzbResp.Channel.Items {
		item := &nzbResp.Channel.Items[i]
		for _, attr := range item.Attrs {
			switch attr.Name {
			case "size":
				item.Size, _ = strconv.ParseInt(attr.Value, 10, 64)
			case "category":
				item.Category = attr.Value
			}
		}
		// Fallback: use enclosure size
		if item.Size == 0 && item.Enclosure.Length > 0 {
			item.Size = item.Enclosure.Length
		}
		// Use enclosure URL if link is empty
		if item.Link == "" && item.Enclosure.URL != "" {
			item.Link = item.Enclosure.URL
		}
	}

	return nzbResp.Channel.Items, nil
}

// SearchMovie searches all indexers for a movie.
func (c *NewznabClient) SearchMovie(indexers []IndexerConfig, imdbID, title string, year int) []Release {
	var allReleases []Release

	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}

		var items []NewznabItem
		var err error

		// Try IMDB search first
		if imdbID != "" {
			items, err = c.SearchMovieByIMDB(idx, imdbID)
		}
		if err != nil || len(items) == 0 {
			// Fallback to text search
			query := title
			if year > 0 {
				query = fmt.Sprintf("%s %d", title, year)
			}
			items, err = c.SearchMovieByText(idx, query)
		}
		if err != nil {
			slog.Warn("indexer search failed", "indexer", idx.Name, "error", err)
			continue
		}

		for _, item := range items {
			parsed := ParseReleaseName(item.Title)
			allReleases = append(allReleases, Release{
				Title:   item.Title,
				NZBURL:  item.Link,
				Size:    item.Size,
				Quality: parsed.Quality,
				Tags:    parsed.Tags,
				Indexer: idx.Name,
			})
		}
	}

	return allReleases
}

// SearchEpisode searches all indexers for a TV episode.
func (c *NewznabClient) SearchEpisode(indexers []IndexerConfig, tvdbID, season, episode int, title string) []Release {
	var allReleases []Release

	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}

		var items []NewznabItem
		var err error

		// Try TVDB search first
		if tvdbID > 0 {
			items, err = c.SearchTVByTVDB(idx, tvdbID, season, episode)
		}
		if err != nil || len(items) == 0 {
			// Fallback to text search
			query := fmt.Sprintf("%s S%02dE%02d", title, season, episode)
			items, err = c.SearchTVByText(idx, query)
		}
		if err != nil {
			slog.Warn("indexer search failed", "indexer", idx.Name, "error", err)
			continue
		}

		for _, item := range items {
			parsed := ParseReleaseName(item.Title)
			allReleases = append(allReleases, Release{
				Title:   item.Title,
				NZBURL:  item.Link,
				Size:    item.Size,
				Quality: parsed.Quality,
				Tags:    parsed.Tags,
				Indexer: idx.Name,
			})
		}
	}

	return allReleases
}

// SearchMusicByCategory searches for music using the music category.
func (c *NewznabClient) SearchMusicByCategory(idx IndexerConfig, query string) ([]NewznabItem, error) {
	u := buildURL(idx.URL, map[string]string{
		"t": "music", "q": query, "apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// SearchMusicByText searches for music using general text search.
func (c *NewznabClient) SearchMusicByText(idx IndexerConfig, query string) ([]NewznabItem, error) {
	u := buildURL(idx.URL, map[string]string{
		"t": "search", "q": query, "apikey": idx.APIKey,
	})
	return c.fetch(u, idx.Name)
}

// SearchMusic searches all indexers for a music album.
// Tries music category first, falls back to text search with FLAC.
func (c *NewznabClient) SearchMusic(indexers []IndexerConfig, artist, album string, year int) []Release {
	var allReleases []Release

	query := fmt.Sprintf("%s %s", artist, album)
	if year > 0 {
		query = fmt.Sprintf("%s %s %d", artist, album, year)
	}

	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}

		var items []NewznabItem
		var err error

		// Try music category first
		items, err = c.SearchMusicByCategory(idx, query)
		if err != nil || len(items) == 0 {
			// Fallback to text search with FLAC keyword
			items, err = c.SearchMusicByText(idx, query+" FLAC")
		}
		if err != nil {
			slog.Warn("indexer music search failed", "indexer", idx.Name, "error", err)
			continue
		}

		for _, item := range items {
			parsed := ParseMusicReleaseName(item.Title)
			allReleases = append(allReleases, Release{
				Title:   item.Title,
				NZBURL:  item.Link,
				Size:    item.Size,
				Quality: parsed.Quality,
				Tags:    parsed.Tags,
				Indexer: idx.Name,
			})
		}
	}

	return allReleases
}

// SearchAnimeEpisode searches all indexers for an anime episode using absolute numbering.
func (c *NewznabClient) SearchAnimeEpisode(indexers []IndexerConfig, titles []string, absoluteNum int) []Release {
	var allReleases []Release

	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}

		for _, title := range titles {
			query := fmt.Sprintf("%s %d", title, absoluteNum)
			items, err := c.SearchByText(idx, query)
			if err != nil {
				slog.Warn("indexer search failed", "indexer", idx.Name, "error", err)
				continue
			}

			for _, item := range items {
				parsed := ParseReleaseName(item.Title)
				allReleases = append(allReleases, Release{
					Title:   item.Title,
					NZBURL:  item.Link,
					Size:    item.Size,
					Quality: parsed.Quality,
					Tags:    parsed.Tags,
					Indexer: idx.Name,
				})
			}
		}
	}

	return allReleases
}
