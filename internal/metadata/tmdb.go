package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type TMDBClient struct {
	client *http.Client
	apiKey string
}

func NewTMDBClient(client *http.Client, apiKey string) *TMDBClient {
	return &TMDBClient{client: client, apiKey: apiKey}
}

const tmdbBase = "https://api.themoviedb.org/3"

// -- Response types --

type TMDBSearchResult struct {
	Page         int              `json:"page"`
	TotalPages   int              `json:"total_pages"`
	TotalResults int              `json:"total_results"`
	Results      []TMDBMediaEntry `json:"results"`
}

type TMDBMediaEntry struct {
	ID               int      `json:"id"`
	Title            string   `json:"title"`             // movies
	Name             string   `json:"name"`              // tv
	OriginalTitle    string   `json:"original_title"`
	OriginalName     string   `json:"original_name"`
	Overview         string   `json:"overview"`
	PosterPath       string   `json:"poster_path"`
	BackdropPath     string   `json:"backdrop_path"`
	ReleaseDate      string   `json:"release_date"`      // movies
	FirstAirDate     string   `json:"first_air_date"`    // tv
	GenreIDs         []int    `json:"genre_ids"`
	VoteAverage      float64  `json:"vote_average"`
	OriginCountry    []string `json:"origin_country"`
	MediaType        string   `json:"media_type"`
}

type TMDBMovieDetail struct {
	ID           int         `json:"id"`
	Title        string      `json:"title"`
	Overview     string      `json:"overview"`
	PosterPath   string      `json:"poster_path"`
	BackdropPath string      `json:"backdrop_path"`
	ReleaseDate  string      `json:"release_date"`
	Genres       []TMDBGenre `json:"genres"`
	VoteAverage  float64     `json:"vote_average"`
	IMDbID       string      `json:"imdb_id"`
	Runtime      int         `json:"runtime"`
}

type TMDBTVDetail struct {
	ID              int          `json:"id"`
	Name            string       `json:"name"`
	Overview        string       `json:"overview"`
	PosterPath      string       `json:"poster_path"`
	BackdropPath    string       `json:"backdrop_path"`
	FirstAirDate    string       `json:"first_air_date"`
	Genres          []TMDBGenre  `json:"genres"`
	VoteAverage     float64      `json:"vote_average"`
	OriginCountry   []string     `json:"origin_country"`
	Seasons         []TMDBSeason `json:"seasons"`
	ExternalIDs     TMDBExtIDs   `json:"external_ids"`
	NumberOfSeasons int          `json:"number_of_seasons"`
}

type TMDBGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TMDBSeason struct {
	ID           int    `json:"id"`
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"poster_path"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date"`
}

type TMDBSeasonDetail struct {
	ID           int           `json:"id"`
	SeasonNumber int           `json:"season_number"`
	Name         string        `json:"name"`
	Overview     string        `json:"overview"`
	PosterPath   string        `json:"poster_path"`
	Episodes     []TMDBEpisode `json:"episodes"`
}

type TMDBEpisode struct {
	ID            int     `json:"id"`
	EpisodeNumber int     `json:"episode_number"`
	SeasonNumber  int     `json:"season_number"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	AirDate       string  `json:"air_date"`
	VoteAverage   float64 `json:"vote_average"`
}

type TMDBExtIDs struct {
	IMDBID  string `json:"imdb_id"`
	TVDBID  int    `json:"tvdb_id"`
	TVRAGEID int   `json:"tvrage_id"`
}

type TMDBTrending struct {
	Page         int              `json:"page"`
	TotalPages   int              `json:"total_pages"`
	Results      []TMDBMediaEntry `json:"results"`
}

// -- Methods --

func (c *TMDBClient) SearchMovies(query string) (*TMDBSearchResult, error) {
	u := fmt.Sprintf("%s/search/movie?api_key=%s&query=%s&include_adult=false",
		tmdbBase, c.apiKey, url.QueryEscape(query))
	return tmdbGet[TMDBSearchResult](c.client, u)
}

func (c *TMDBClient) SearchTV(query string) (*TMDBSearchResult, error) {
	u := fmt.Sprintf("%s/search/tv?api_key=%s&query=%s&include_adult=false",
		tmdbBase, c.apiKey, url.QueryEscape(query))
	return tmdbGet[TMDBSearchResult](c.client, u)
}

func (c *TMDBClient) GetMovie(id int) (*TMDBMovieDetail, error) {
	u := fmt.Sprintf("%s/movie/%d?api_key=%s", tmdbBase, id, c.apiKey)
	return tmdbGet[TMDBMovieDetail](c.client, u)
}

func (c *TMDBClient) GetTV(id int) (*TMDBTVDetail, error) {
	u := fmt.Sprintf("%s/tv/%d?api_key=%s&append_to_response=external_ids", tmdbBase, id, c.apiKey)
	return tmdbGet[TMDBTVDetail](c.client, u)
}

func (c *TMDBClient) GetSeason(tvID, seasonNum int) (*TMDBSeasonDetail, error) {
	u := fmt.Sprintf("%s/tv/%d/season/%d?api_key=%s", tmdbBase, tvID, seasonNum, c.apiKey)
	return tmdbGet[TMDBSeasonDetail](c.client, u)
}

func (c *TMDBClient) TrendingMovies(page int) (*TMDBTrending, error) {
	u := fmt.Sprintf("%s/trending/movie/week?api_key=%s&page=%d", tmdbBase, c.apiKey, page)
	return tmdbGet[TMDBTrending](c.client, u)
}

func (c *TMDBClient) TrendingTV(page int) (*TMDBTrending, error) {
	u := fmt.Sprintf("%s/trending/tv/week?api_key=%s&page=%d", tmdbBase, c.apiKey, page)
	return tmdbGet[TMDBTrending](c.client, u)
}

func (c *TMDBClient) DiscoverAnimeTV(page int) (*TMDBSearchResult, error) {
	u := fmt.Sprintf("%s/discover/tv?api_key=%s&with_genres=16&with_origin_country=JP&sort_by=popularity.desc&page=%d",
		tmdbBase, c.apiKey, page)
	return tmdbGet[TMDBSearchResult](c.client, u)
}

// IsAnime checks if a TMDB TV show is likely anime.
func (c *TMDBClient) IsAnime(detail *TMDBTVDetail) bool {
	hasAnimation := false
	for _, g := range detail.Genres {
		if g.Name == "Animation" {
			hasAnimation = true
			break
		}
	}
	if !hasAnimation {
		return false
	}
	for _, country := range detail.OriginCountry {
		if strings.ToUpper(country) == "JP" {
			return true
		}
	}
	return false
}

func PosterURL(path string) string {
	if path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w500" + path
}

func BackdropURL(path string) string {
	if path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w1280" + path
}

func tmdbGet[T any](client *http.Client, url string) (*T, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("tmdb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tmdb %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tmdb decode: %w", err)
	}
	return &result, nil
}
