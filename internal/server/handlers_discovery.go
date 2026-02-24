package server

import (
	"net/http"
	"strconv"
	"strings"

	"mediaforge/internal/metadata"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
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
		results, err := s.tmdb.SearchTV(query)
		if err != nil {
			writeError(w, 500, "TMDB search failed: "+err.Error())
			return
		}
		// Filter to anime only (Animation genre + JP origin)
		var animeResults []metadata.TMDBMediaEntry
		for _, e := range results.Results {
			if isAnimeEntry(e) {
				animeResults = append(animeResults, e)
			}
		}
		writeJSON(w, 200, s.formatTMDBResults(animeResults, "series"))

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

	switch mediaType {
	case "anime":
		if s.tmdb == nil {
			writeError(w, 500, "TMDB not configured")
			return
		}
		results, err := s.tmdb.DiscoverAnimeTV(page)
		if err != nil {
			writeError(w, 500, "TMDB anime trending failed: "+err.Error())
			return
		}
		writeJSON(w, 200, s.formatTMDBResults(results.Results, "series"))

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
		writeJSON(w, 200, movie)

	case "series", "":
		tv, err := s.tmdb.GetTV(tmdbID)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, tv)

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

	writeJSON(w, 200, media)
}

type tmdbResultEntry struct {
	TMDBID      int      `json:"tmdb_id"`
	Title       string   `json:"title"`
	Overview    string   `json:"overview"`
	PosterURL   string   `json:"poster_url"`
	BackdropURL string   `json:"backdrop_url,omitempty"`
	Year        int      `json:"year"`
	Rating      float64  `json:"rating"`
	Type        string   `json:"type"`
	InLibrary   bool     `json:"in_library"`
	LibraryID   int      `json:"library_id,omitempty"`
	IsAnime     bool     `json:"is_anime,omitempty"`
}

func isAnimeEntry(e metadata.TMDBMediaEntry) bool {
	hasAnimation := false
	for _, g := range e.GenreIDs {
		if g == 16 {
			hasAnimation = true
			break
		}
	}
	if !hasAnimation {
		return false
	}
	for _, c := range e.OriginCountry {
		if strings.ToUpper(c) == "JP" {
			return true
		}
	}
	return false
}

func (s *Server) formatTMDBResults(entries []metadata.TMDBMediaEntry, mediaType string) []tmdbResultEntry {
	var out []tmdbResultEntry
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
		}

		// Check if it might be anime
		if mediaType == "series" {
			for _, g := range e.GenreIDs {
				if g == 16 { // Animation genre ID in TMDB
					for _, c := range e.OriginCountry {
						if strings.ToUpper(c) == "JP" {
							r.IsAnime = true
						}
					}
				}
			}
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
