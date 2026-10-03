package metadata

import (
	"encoding/json"
	"fmt"
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
	ID            int      `json:"id"`
	Title         string   `json:"title"` // movies
	Name          string   `json:"name"`  // tv
	OriginalTitle string   `json:"original_title"`
	OriginalName  string   `json:"original_name"`
	Overview      string   `json:"overview"`
	PosterPath    string   `json:"poster_path"`
	BackdropPath  string   `json:"backdrop_path"`
	ReleaseDate   string   `json:"release_date"`   // movies
	FirstAirDate  string   `json:"first_air_date"` // tv
	GenreIDs      []int    `json:"genre_ids"`
	VoteAverage   float64  `json:"vote_average"`
	OriginCountry []string `json:"origin_country"`
	MediaType     string   `json:"media_type"`
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
	IMDBID   string `json:"imdb_id"`
	TVDBID   int    `json:"tvdb_id"`
	TVRAGEID int    `json:"tvrage_id"`
}

type TMDBTrending struct {
	Page       int              `json:"page"`
	TotalPages int              `json:"total_pages"`
	Results    []TMDBMediaEntry `json:"results"`
}

// -- Methods --

func (c *TMDBClient) SearchMovies(query string) (*TMDBSearchResult, error) {
	u := fmt.Sprintf("%s/search/movie?api_key=%s&query=%s&include_adult=false",
		tmdbBase, url.QueryEscape(c.apiKey), url.QueryEscape(query))
	return tmdbGet[TMDBSearchResult](c.client, u)
}

func (c *TMDBClient) SearchTV(query string) (*TMDBSearchResult, error) {
	u := fmt.Sprintf("%s/search/tv?api_key=%s&query=%s&include_adult=false",
		tmdbBase, url.QueryEscape(c.apiKey), url.QueryEscape(query))
	return tmdbGet[TMDBSearchResult](c.client, u)
}

func (c *TMDBClient) GetMovie(id int) (*TMDBMovieDetail, error) {
	u := fmt.Sprintf("%s/movie/%d?api_key=%s", tmdbBase, id, url.QueryEscape(c.apiKey))
	return tmdbGet[TMDBMovieDetail](c.client, u)
}

func (c *TMDBClient) GetTV(id int) (*TMDBTVDetail, error) {
	u := fmt.Sprintf("%s/tv/%d?api_key=%s&append_to_response=external_ids", tmdbBase, id, url.QueryEscape(c.apiKey))
	return tmdbGet[TMDBTVDetail](c.client, u)
}

func (c *TMDBClient) GetSeason(tvID, seasonNum int) (*TMDBSeasonDetail, error) {
	u := fmt.Sprintf("%s/tv/%d/season/%d?api_key=%s", tmdbBase, tvID, seasonNum, url.QueryEscape(c.apiKey))
	return tmdbGet[TMDBSeasonDetail](c.client, u)
}

func (c *TMDBClient) TrendingMovies(page int) (*TMDBTrending, error) {
	u := fmt.Sprintf("%s/trending/movie/week?api_key=%s&page=%d", tmdbBase, url.QueryEscape(c.apiKey), page)
	return tmdbGet[TMDBTrending](c.client, u)
}

func (c *TMDBClient) TrendingTV(page int) (*TMDBTrending, error) {
	u := fmt.Sprintf("%s/trending/tv/week?api_key=%s&page=%d", tmdbBase, url.QueryEscape(c.apiKey), page)
	return tmdbGet[TMDBTrending](c.client, u)
}

func (c *TMDBClient) DiscoverAnimeTV(page int) (*TMDBSearchResult, error) {
	u := fmt.Sprintf("%s/discover/tv?api_key=%s&with_genres=16&with_origin_country=JP&sort_by=popularity.desc&page=%d",
		tmdbBase, url.QueryEscape(c.apiKey), page)
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

func tmdbGet[T any](client *http.Client, rawURL string) (*T, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid TMDB endpoint")
	}
	query := endpoint.Query()
	credential := query.Get("api_key")
	if credential == "" {
		return nil, fmt.Errorf("TMDB credential is not configured")
	}
	// TMDB accepts a 32-character v3 key in the query, or a read access token
	// as a bearer header. Match the Settings connection test for every endpoint.
	if len(credential) != 32 {
		query.Del("api_key")
		endpoint.RawQuery = query.Encode()
	}
	request, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid TMDB request")
	}
	if len(credential) != 32 {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	request.Header.Set("Accept", "application/json")
	resp, err := client.Do(request)
	if err != nil {
		// url.Error includes the entire URL, including the legacy query API key.
		return nil, fmt.Errorf("TMDB request failed; check the network or proxy")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB returned HTTP %d", resp.StatusCode)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tmdb decode: %w", err)
	}
	return &result, nil
}
