package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AniListClient struct {
	client *http.Client
}

func NewAniListClient(client *http.Client) *AniListClient {
	return &AniListClient{client: client}
}

const anilistURL = "https://graphql.anilist.co"

// -- Response types --

type AniListMedia struct {
	ID          int              `json:"id"`
	IDMAL       int              `json:"idMal"`
	Title       AniListTitle     `json:"title"`
	Description string           `json:"description"`
	CoverImage  AniListCover     `json:"coverImage"`
	BannerImage string           `json:"bannerImage"`
	Format      string           `json:"format"`       // TV, MOVIE, OVA, ONA, SPECIAL
	Episodes    int              `json:"episodes"`
	Status      string           `json:"status"`       // RELEASING, FINISHED, etc.
	StartDate   AniListDate      `json:"startDate"`
	Genres      []string         `json:"genres"`
	Tags        []AniListTag     `json:"tags"`
	AverageScore int             `json:"averageScore"` // 0-100
	ExternalLinks []AniListExtLink `json:"externalLinks"`
}

type AniListTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

type AniListCover struct {
	Large      string `json:"large"`
	ExtraLarge string `json:"extraLarge"`
}

type AniListDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type AniListTag struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
}

type AniListExtLink struct {
	Site string `json:"site"`
	URL  string `json:"url"`
}

type anilistResponse struct {
	Data struct {
		Page struct {
			Media []AniListMedia `json:"media"`
		} `json:"Page"`
		Media AniListMedia `json:"Media"`
	} `json:"data"`
}

type anilistTrendingResponse struct {
	Data struct {
		Page struct {
			Media []AniListMedia `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

// -- Queries --

const searchQuery = `
query ($search: String, $type: MediaType, $page: Int) {
  Page(page: $page, perPage: 20) {
    media(search: $search, type: $type, sort: SEARCH_MATCH) {
      id
      idMal
      title { romaji english native }
      description(asHtml: false)
      coverImage { large extraLarge }
      bannerImage
      format
      episodes
      status
      startDate { year month day }
      genres
      tags { name rank }
      averageScore
    }
  }
}
`

const detailQuery = `
query ($id: Int) {
  Media(id: $id) {
    id
    idMal
    title { romaji english native }
    description(asHtml: false)
    coverImage { large extraLarge }
    bannerImage
    format
    episodes
    status
    startDate { year month day }
    genres
    tags { name rank }
    averageScore
    externalLinks { site url }
  }
}
`

const trendingQuery = `
query ($page: Int) {
  Page(page: $page, perPage: 20) {
    media(type: ANIME, sort: TRENDING_DESC) {
      id
      idMal
      title { romaji english native }
      description(asHtml: false)
      coverImage { large extraLarge }
      bannerImage
      format
      episodes
      status
      startDate { year month day }
      genres
      tags { name rank }
      averageScore
    }
  }
}
`

// -- Methods --

func (c *AniListClient) SearchAnime(query string, page int) ([]AniListMedia, error) {
	variables := map[string]interface{}{
		"search": query,
		"type":   "ANIME",
		"page":   page,
	}

	var resp anilistResponse
	if err := c.graphql(searchQuery, variables, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Page.Media, nil
}

func (c *AniListClient) GetAnime(id int) (*AniListMedia, error) {
	variables := map[string]interface{}{
		"id": id,
	}

	var resp anilistResponse
	if err := c.graphql(detailQuery, variables, &resp); err != nil {
		return nil, err
	}
	return &resp.Data.Media, nil
}

func (c *AniListClient) TrendingAnime(page int) ([]AniListMedia, error) {
	variables := map[string]interface{}{
		"page": page,
	}

	var resp anilistTrendingResponse
	if err := c.graphql(trendingQuery, variables, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Page.Media, nil
}

// DisplayTitle returns the best available title.
func (m *AniListMedia) DisplayTitle() string {
	if m.Title.English != "" {
		return m.Title.English
	}
	if m.Title.Romaji != "" {
		return m.Title.Romaji
	}
	return m.Title.Native
}

func (c *AniListClient) graphql(query string, variables map[string]interface{}, target interface{}) error {
	body := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal graphql: %w", err)
	}

	req, err := http.NewRequest("POST", anilistURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("anilist request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anilist %d: %s", resp.StatusCode, string(b))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("anilist decode: %w", err)
	}
	return nil
}
