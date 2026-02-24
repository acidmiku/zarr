package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

// handleListRatings returns all user ratings with media metadata.
func (s *Server) handleListRatings(w http.ResponseWriter, r *http.Request) {
	filterType := queryString(r, "type", "all")
	sortBy := queryString(r, "sort", "rating")
	order := queryString(r, "order", "desc")

	query := `SELECT r.id, r.tmdb_id, r.media_type, r.rating, r.comment, r.created_at, r.updated_at,
		COALESCE(r.title, m.title, '') as title,
		COALESCE(r.poster_url, m.poster_url, '') as poster_url,
		COALESCE(r.year, m.year, 0) as year,
		COALESCE(r.anime, m.anime, 0) as anime,
		COALESCE(m.id, 0) as media_id
		FROM user_ratings r
		LEFT JOIN media_items m ON m.tmdb_id = r.tmdb_id AND m.type = r.media_type`

	var conditions []string
	var args []interface{}

	switch filterType {
	case "movie":
		conditions = append(conditions, "r.media_type = 'movie'")
	case "series":
		conditions = append(conditions, "r.media_type = 'series'")
		conditions = append(conditions, "COALESCE(r.anime, m.anime, 0) = 0")
	case "anime":
		conditions = append(conditions, "COALESCE(r.anime, m.anime, 0) = 1")
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, c := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += c
		}
	}

	// Sort
	if order != "asc" {
		order = "desc"
	}
	switch sortBy {
	case "date":
		query += fmt.Sprintf(" ORDER BY r.updated_at %s", order)
	case "title":
		query += fmt.Sprintf(" ORDER BY title %s", order)
	default:
		query += fmt.Sprintf(" ORDER BY r.rating %s, r.updated_at DESC", order)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		writeError(w, 500, "database error: "+err.Error())
		return
	}
	defer rows.Close()

	type ratingItem struct {
		ID        int    `json:"id"`
		TMDBID    int    `json:"tmdb_id"`
		MediaType string `json:"media_type"`
		Rating    int    `json:"rating"`
		Comment   string `json:"comment"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Title     string `json:"title"`
		PosterURL string `json:"poster_url"`
		Year      int    `json:"year"`
		Anime     bool   `json:"anime"`
		MediaID   int    `json:"media_id"`
	}

	var items []ratingItem
	for rows.Next() {
		var item ratingItem
		var comment sql.NullString
		var anime int
		if err := rows.Scan(&item.ID, &item.TMDBID, &item.MediaType, &item.Rating,
			&comment, &item.CreatedAt, &item.UpdatedAt,
			&item.Title, &item.PosterURL, &item.Year, &anime, &item.MediaID); err != nil {
			continue
		}
		if comment.Valid {
			item.Comment = comment.String
		}
		item.Anime = anime == 1
		items = append(items, item)
	}

	if items == nil {
		items = []ratingItem{}
	}
	writeJSON(w, 200, items)
}

type upsertRatingRequest struct {
	TMDBID    int    `json:"tmdb_id"`
	MediaType string `json:"media_type"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
	Title     string `json:"title"`
	PosterURL string `json:"poster_url"`
	Year      int    `json:"year"`
	Anime     bool   `json:"anime"`
}

func (s *Server) handleUpsertRating(w http.ResponseWriter, r *http.Request) {
	var req upsertRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.TMDBID <= 0 {
		writeError(w, 400, "tmdb_id is required")
		return
	}
	if req.MediaType != "movie" && req.MediaType != "series" {
		writeError(w, 400, "media_type must be 'movie' or 'series'")
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		writeError(w, 400, "rating must be between 1 and 5")
		return
	}

	_, err := s.db.Exec(`INSERT INTO user_ratings (tmdb_id, media_type, rating, comment, title, poster_url, year, anime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tmdb_id, media_type) DO UPDATE SET
			rating = excluded.rating,
			comment = excluded.comment,
			title = COALESCE(excluded.title, user_ratings.title),
			poster_url = COALESCE(excluded.poster_url, user_ratings.poster_url),
			year = COALESCE(excluded.year, user_ratings.year),
			anime = excluded.anime,
			updated_at = CURRENT_TIMESTAMP`,
		req.TMDBID, req.MediaType, req.Rating, req.Comment,
		nilIfEmpty(req.Title), nilIfEmpty(req.PosterURL), nilIfZero(req.Year), req.Anime)
	if err != nil {
		writeError(w, 500, "database error: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "saved"})
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZero(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

func (s *Server) handleGetRating(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := parseID(r, "tmdbID")
	if err != nil {
		writeError(w, 400, "invalid tmdb_id")
		return
	}
	mediaType := queryString(r, "type", "movie")

	var rating struct {
		TMDBID    int    `json:"tmdb_id"`
		MediaType string `json:"media_type"`
		Rating    int    `json:"rating"`
		Comment   string `json:"comment"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var nullComment sql.NullString
	err = s.db.QueryRow(`SELECT tmdb_id, media_type, rating, comment, created_at, updated_at
		FROM user_ratings WHERE tmdb_id = ? AND media_type = ?`,
		tmdbID, mediaType).Scan(
		&rating.TMDBID, &rating.MediaType, &rating.Rating,
		&nullComment, &rating.CreatedAt, &rating.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, 200, nil)
			return
		}
		writeError(w, 500, "database error")
		return
	}

	if nullComment.Valid {
		rating.Comment = nullComment.String
	}

	writeJSON(w, 200, rating)
}

func (s *Server) handleDeleteRating(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := parseID(r, "tmdbID")
	if err != nil {
		writeError(w, 400, "invalid tmdb_id")
		return
	}
	mediaType := queryString(r, "type", "movie")

	s.db.Exec(`DELETE FROM user_ratings WHERE tmdb_id = ? AND media_type = ?`, tmdbID, mediaType)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
