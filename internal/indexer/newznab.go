package indexer

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
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
	Type         string // "newznab" or "rutracker"
	Username     string
	Password     string
	ContentTypes []string // e.g. ["movie","series","anime","music"]
}

// NewznabResponse represents the RSS/XML response from a Newznab API.
type NewznabResponse struct {
	XMLName xml.Name       `xml:"rss"`
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
	if u.Path == "" || u.Path == "/" {
		u.Path = "/api"
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
		"t": "movie", "imdbid": id, "apikey": idx.APIKey,
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
		return nil, fmt.Errorf("newznab request to %s failed", indexerName)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", indexerName, err)
	}

	if len(body) > 16<<20 {
		return nil, fmt.Errorf("newznab response exceeds 16 MiB")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newznab %s HTTP %d", indexerName, resp.StatusCode)
	}
	var document struct {
		XMLName xml.Name
		Code    int `xml:"code,attr"`
	}
	if err := xml.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("invalid XML from %s", indexerName)
	}
	if document.XMLName.Local == "error" {
		return nil, fmt.Errorf("newznab %s API error %d; check API key and request parameters", indexerName, document.Code)
	}

	return body, nil
}

// TestConnection tests connectivity to an indexer.
func (c *NewznabClient) TestConnection(idx IndexerConfig) error {
	u := buildURL(idx.URL, map[string]string{
		"t": "caps", "apikey": idx.APIKey,
	})
	data, err := c.fetchRaw(u, idx.Name)
	if err != nil {
		return err
	}
	var caps struct {
		XMLName xml.Name `xml:"caps"`
	}
	if err := xml.Unmarshal(data, &caps); err != nil {
		return fmt.Errorf("%s did not return Newznab capabilities", idx.Name)
	}
	return nil
}

func (c *NewznabClient) fetch(rawURL, indexerName string) ([]NewznabItem, error) {
	slog.Debug("newznab query", "indexer", indexerName)

	data, err := c.fetchRaw(rawURL, indexerName)
	if err != nil {
		return nil, err
	}

	var nzbResp NewznabResponse
	if err := xml.Unmarshal(data, &nzbResp); err != nil {
		return nil, fmt.Errorf("invalid search response from %s", indexerName)
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
		if item.Enclosure.URL != "" {
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
		textSearch := false

		// Try IMDB search first
		if imdbID != "" {
			items, err = c.SearchMovieByIMDB(idx, imdbID)
		}
		if err != nil || len(items) == 0 {
			textSearch = true
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
			if textSearch && !matchesMovieText(item.Title, title, year) {
				continue
			}
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
			if !MatchesEpisode(item.Title, season, episode) {
				continue
			}
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
				if !MatchesAbsoluteEpisode(item.Title, title, absoluteNum) {
					continue
				}
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

var episodeToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,3})e(\d{1,4})(?:[^0-9]|$)`)

func MatchesEpisode(name string, season, episode int) bool {
	m := episodeToken.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	s, _ := strconv.Atoi(m[1])
	e, _ := strconv.Atoi(m[2])
	return s == season && e == episode
}
func MatchesAbsoluteEpisode(name, title string, number int) bool {
	normalized := strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(name))
	normalizedTitle := strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(title))
	pos := strings.Index(normalized, normalizedTitle)
	if pos < 0 {
		return false
	}
	tail := normalized[pos+len(normalizedTitle):]
	pattern := regexp.MustCompile(fmt.Sprintf(`(?:^|[^0-9a-z])0*%d(?:v\d+)?(?:[^0-9a-z]|$)`, number))
	return pattern.MatchString(tail)
}

var titleSeparators = regexp.MustCompile(`[^\pL\pN]+`)

func matchesMovieText(name, title string, year int) bool {
	normalize := func(s string) string {
		return strings.TrimSpace(titleSeparators.ReplaceAllString(strings.ToLower(s), " "))
	}
	normalized := normalize(name)
	wanted := normalize(title)
	if wanted == "" || !strings.Contains(" "+normalized+" ", " "+wanted+" ") {
		return false
	}
	if year > 0 && !regexp.MustCompile(fmt.Sprintf(`(?:^|[^0-9])%d(?:[^0-9]|$)`, year)).MatchString(name) {
		return false
	}
	return true
}
