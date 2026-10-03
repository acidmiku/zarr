package server

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"mediaforge/internal/metadata"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	mediaType := r.URL.Query().Get("type")

	if query == "" {
		writeError(w, 400, "query parameter 'q' is required")
		return
	}

	switch mediaType {
	case "anime":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.animeResults(
			func() (*metadata.TMDBSearchResult, error) { return s.tmdb.SearchTV(query) },
			func() (*metadata.TMDBSearchResult, error) { return s.tmdb.SearchMovies(query) }, true)
		if err != nil {
			writeError(w, 500, "TMDB search failed: "+err.Error())
			return
		}
		writeJSON(w, 200, results)

	case "movie":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.tmdb.SearchMovies(query)
		if err != nil {
			writeError(w, 500, "TMDB search failed: "+err.Error())
			return
		}
		writeJSON(w, 200, s.formatTMDBResults(results.Results, "movie"))

	case "series", "":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.tmdb.SearchTV(query)
		if err != nil {
			writeError(w, 500, "TMDB search failed: "+err.Error())
			return
		}
		writeJSON(w, 200, s.formatTMDBResults(results.Results, "series"))

	default:
		writeError(w, 400, "invalid type: use movie, series, or anime")
	}
}

func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("type")
	page := queryInt(r, "page", 1)
	if page < 1 || page > 500 {
		writeError(w, 400, "page must be between 1 and 500")
		return
	}

	switch mediaType {
	case "anime":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.animeResults(
			func() (*metadata.TMDBSearchResult, error) { return s.tmdb.DiscoverAnimeTV(page) },
			func() (*metadata.TMDBSearchResult, error) { return s.tmdb.DiscoverAnimeMovies(page) }, false)
		if err != nil {
			writeError(w, 500, "TMDB anime trending failed: "+err.Error())
			return
		}
		writeJSON(w, 200, results)

	case "movie":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.tmdb.TrendingMovies(page)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, s.formatTMDBResults(results.Results, "movie"))

	case "series", "":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.tmdb.TrendingTV(page)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, s.formatTMDBResults(results.Results, "series"))

	default:
		writeError(w, 400, "invalid type")
	}
}

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := parseID(r, "tmdbID")
	if err != nil {
		writeError(w, 400, "invalid tmdb_id")
		return
	}
	mediaType := r.URL.Query().Get("type")

	if s.tmdb == nil {
		writeError(w, 500, "TMDB not configured")
		return
	}

	switch mediaType {
	case "movie":
		movie, err := s.tmdb.GetMovie(tmdbID)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, struct {
			*metadata.TMDBMovieDetail
			Type    string `json:"type"`
			IsAnime bool   `json:"is_anime"`
		}{movie, "movie", s.tmdb.IsAnimeMovie(movie)})

	case "series", "":
		tv, err := s.tmdb.GetTV(tmdbID)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, struct {
			*metadata.TMDBTVDetail
			Type    string `json:"type"`
			IsAnime bool   `json:"is_anime"`
		}{tv, "series", s.tmdb.IsAnime(tv)})

	default:
		writeError(w, 400, "invalid type")
	}
}

func (s *Server) handleAnilistMetadata(w http.ResponseWriter, r *http.Request) {
	anilistID, err := parseID(r, "anilistID")
	if err != nil {
		writeError(w, 400, "invalid anilist_id")
		return
	}

	if s.anilist == nil {
		writeError(w, 500, "AniList client not configured")
		return
	}

	media, err := s.anilist.GetAnime(anilistID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	mediaType := "series"
	if strings.EqualFold(media.Format, "MOVIE") {
		mediaType = "movie"
	}
	writeJSON(w, 200, struct {
		*metadata.AniListMedia
		Type    string `json:"type"`
		IsAnime bool   `json:"is_anime"`
	}{media, mediaType, true})
}

type tmdbResultEntry struct {
	TMDBID      int     `json:"tmdb_id"`
	Title       string  `json:"title"`
	Overview    string  `json:"overview"`
	PosterURL   string  `json:"poster_url"`
	BackdropURL string  `json:"backdrop_url,omitempty"`
	Year        int     `json:"year"`
	Rating      float64 `json:"rating"`
	Type        string  `json:"type"`
	InLibrary   bool    `json:"in_library"`
	LibraryID   int     `json:"library_id,omitempty"`
	IsAnime     bool    `json:"is_anime,omitempty"`
}

// Load both TMDB namespaces concurrently, then interleave their rankings so
// anime movies remain visible alongside series. Numeric TMDB IDs are only
// unique within a media type and must never be deduplicated across types.
func (s *Server) animeResults(tvFetch, movieFetch func() (*metadata.TMDBSearchResult, error), filter bool) ([]tmdbResultEntry, error) {
	var tv, movies *metadata.TMDBSearchResult
	var tvErr, movieErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); tv, tvErr = tvFetch() }()
	go func() { defer wg.Done(); movies, movieErr = movieFetch() }()
	wg.Wait()
	if tvErr != nil {
		return nil, tvErr
	}
	if movieErr != nil {
		return nil, movieErr
	}
	format := func(entries []metadata.TMDBMediaEntry, mediaType string) []tmdbResultEntry {
		filtered := make([]metadata.TMDBMediaEntry, 0, len(entries))
		for _, entry := range entries {
			if !filter || metadata.IsAnimeEntry(entry) {
				filtered = append(filtered, entry)
			}
		}
		results := s.formatTMDBResults(filtered, mediaType)
		// Discover requests already require JP origin and Animation, even
		// when TMDB omits those fields from an individual movie response.
		for i := range results {
			results[i].IsAnime = true
		}
		return results
	}
	tvResults := format(tv.Results, "series")
	movieResults := format(movies.Results, "movie")
	out := make([]tmdbResultEntry, 0, len(tvResults)+len(movieResults))
	for i := 0; i < len(tvResults) || i < len(movieResults); i++ {
		if i < len(tvResults) {
			out = append(out, tvResults[i])
		}
		if i < len(movieResults) {
			out = append(out, movieResults[i])
		}
	}
	return out, nil
}

func (s *Server) formatTMDBResults(entries []metadata.TMDBMediaEntry, mediaType string) []tmdbResultEntry {
	out := make([]tmdbResultEntry, 0, len(entries))
	for _, e := range entries {
		title := e.Title
		if title == "" {
			title = e.Name
		}

		year := 0
		dateStr := e.ReleaseDate
		if dateStr == "" {
			dateStr = e.FirstAirDate
		}
		if len(dateStr) >= 4 {
			year, _ = strconv.Atoi(dateStr[:4])
		}

		r := tmdbResultEntry{
			TMDBID:      e.ID,
			Title:       title,
			Overview:    e.Overview,
			PosterURL:   metadata.PosterURL(e.PosterPath),
			BackdropURL: metadata.BackdropURL(e.BackdropPath),
			Year:        year,
			Rating:      e.VoteAverage,
			Type:        mediaType,
			IsAnime:     metadata.IsAnimeEntry(e),
		}

		// Check if in library
		var libID int
		if s.db.QueryRow(`SELECT id FROM media_items WHERE tmdb_id = ? AND type = ?`, e.ID, mediaType).Scan(&libID) == nil {
			r.InLibrary = true
			r.LibraryID = libID
		}

		out = append(out, r)
	}
	return out
}
