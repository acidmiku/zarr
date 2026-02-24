package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type JikanClient struct {
	client   *http.Client
	imgClient *http.Client // for CDN image fetches (may need proxy)
	cacheDir string
	lastReq  time.Time
}

func NewJikanClient(client *http.Client, imgClient *http.Client, cacheDir string) *JikanClient {
	os.MkdirAll(filepath.Join(cacheDir, "jikan"), 0755)
	if imgClient == nil {
		imgClient = client
	}
	return &JikanClient{
		client:   client,
		imgClient: imgClient,
		cacheDir: filepath.Join(cacheDir, "jikan"),
	}
}

const jikanBase = "https://api.jikan.moe/v4"

// Rate limit: 3 req/sec. Simple approach: ensure 350ms between requests.
func (c *JikanClient) rateLimit() {
	elapsed := time.Since(c.lastReq)
	if elapsed < 350*time.Millisecond {
		time.Sleep(350*time.Millisecond - elapsed)
	}
	c.lastReq = time.Now()
}

// -- Response types --

type JikanAnime struct {
	MalID         int           `json:"mal_id"`
	Title         string        `json:"title"`
	TitleEnglish  string        `json:"title_english"`
	TitleJapanese string        `json:"title_japanese"`
	Type          string        `json:"type"`
	Episodes      int           `json:"episodes"`
	Status        string        `json:"status"`
	Score         float64       `json:"score"`
	ScoredBy      int           `json:"scored_by"`
	Rank          int           `json:"rank"`
	Popularity    int           `json:"popularity"`
	Synopsis      string        `json:"synopsis"`
	Year          int           `json:"year"`
	Season        string        `json:"season"`
	Genres        []JikanTag    `json:"genres"`
	Themes        []JikanTag    `json:"themes"`
	Demographics  []JikanTag    `json:"demographics"`
	Studios       []JikanTag    `json:"studios"`
	Images        JikanImages   `json:"images"`
}

type JikanTag struct {
	MalID int    `json:"mal_id"`
	Name  string `json:"name"`
}

type JikanImages struct {
	JPG  JikanImageSet `json:"jpg"`
	WebP JikanImageSet `json:"webp"`
}

type JikanImageSet struct {
	ImageURL      string `json:"image_url"`
	LargeImageURL string `json:"large_image_url"`
}

type jikanSearchResponse struct {
	Data       []JikanAnime   `json:"data"`
	Pagination JikanPagination `json:"pagination"`
}

type jikanDetailResponse struct {
	Data JikanAnime `json:"data"`
}

type JikanPagination struct {
	LastVisiblePage int  `json:"last_visible_page"`
	HasNextPage     bool `json:"has_next_page"`
	CurrentPage     int  `json:"current_page"`
}

type JikanRecommendation struct {
	Entry JikanAnime `json:"entry"`
	Votes int        `json:"votes"`
}

type jikanRecsResponse struct {
	Data []JikanRecommendation `json:"data"`
}

// SearchAnime searches for anime on MAL.
type SearchParams struct {
	Query   string
	Type    string // tv, movie, ova, special, ona
	MinScore float64
	Genres  string // comma-separated genre IDs
	Status  string // airing, complete, upcoming
	OrderBy string
	Sort    string // asc, desc
	Limit   int
}

func (c *JikanClient) SearchAnime(params SearchParams) ([]JikanAnime, error) {
	c.rateLimit()

	u, _ := url.Parse(jikanBase + "/anime")
	q := u.Query()
	q.Set("sfw", "true")

	if params.Query != "" {
		q.Set("q", params.Query)
	}
	if params.Type != "" {
		q.Set("type", params.Type)
	}
	if params.MinScore > 0 {
		q.Set("min_score", strconv.FormatFloat(params.MinScore, 'f', 1, 64))
	}
	if params.Genres != "" {
		q.Set("genres", params.Genres)
	}
	if params.Status != "" {
		q.Set("status", params.Status)
	}
	if params.OrderBy != "" {
		q.Set("order_by", params.OrderBy)
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 25 {
		limit = 25
	}
	q.Set("limit", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	resp, err := c.doGet(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result jikanSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jikan decode: %w", err)
	}
	return result.Data, nil
}

// GetAnime gets a single anime by MAL ID.
func (c *JikanClient) GetAnime(malID int) (*JikanAnime, error) {
	c.rateLimit()

	resp, err := c.doGet(fmt.Sprintf("%s/anime/%d", jikanBase, malID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result jikanDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jikan decode: %w", err)
	}
	return &result.Data, nil
}

// GetRecommendations gets MAL recommendations for an anime.
func (c *JikanClient) GetRecommendations(malID int) ([]JikanRecommendation, error) {
	c.rateLimit()

	resp, err := c.doGet(fmt.Sprintf("%s/anime/%d/recommendations", jikanBase, malID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result jikanRecsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jikan decode: %w", err)
	}
	return result.Data, nil
}

// GetSeasonNow gets current season anime.
func (c *JikanClient) GetSeasonNow(limit int) ([]JikanAnime, error) {
	c.rateLimit()

	if limit <= 0 {
		limit = 10
	}
	resp, err := c.doGet(fmt.Sprintf("%s/seasons/now?limit=%d&sfw=true", jikanBase, limit))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result jikanSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jikan decode: %w", err)
	}
	return result.Data, nil
}

// GetPosterPath returns the cached poster path, fetching if needed.
func (c *JikanClient) GetPosterPath(malID int) (string, error) {
	cachePath := filepath.Join(c.cacheDir, fmt.Sprintf("mal_%d.jpg", malID))

	// Check cache
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	// Fetch anime detail to get image URL
	anime, err := c.GetAnime(malID)
	if err != nil {
		return "", err
	}

	imageURL := anime.Images.JPG.LargeImageURL
	if imageURL == "" {
		imageURL = anime.Images.JPG.ImageURL
	}
	if imageURL == "" {
		return "", fmt.Errorf("no image URL for mal_id %d", malID)
	}

	// Download image via proxy-capable client
	resp, err := c.imgClient.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("image fetch %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return "", fmt.Errorf("write cache: %w", err)
	}

	return cachePath, nil
}

// DisplayTitle returns the best available title.
func (a *JikanAnime) DisplayTitle() string {
	if a.TitleEnglish != "" {
		return a.TitleEnglish
	}
	return a.Title
}

func (c *JikanClient) doGet(url string) (*http.Response, error) {
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := c.client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("jikan request: %w", err)
		}
		if resp.StatusCode == 429 {
			resp.Body.Close()
			time.Sleep(time.Duration(attempt+1) * time.Second)
			c.lastReq = time.Now()
			continue
		}
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("jikan %d: %s", resp.StatusCode, string(b))
		}
		return resp, nil
	}
	return nil, fmt.Errorf("jikan: rate limited after 3 retries")
}
