package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// handleListRatings returns all user ratings with media metadata.
func (s *Server) handleListRatings(w http.ResponseWriter, r *http.Request) {
	filterType := queryString(r, "type", "all")
	sortBy := queryString(r, "sort", "rating")
	order := queryString(r, "order", "desc")

	type ratingItem struct {
		ID             int    `json:"id"`
		TMDBID         int    `json:"tmdb_id"`
		MediaType      string `json:"media_type"`
		Rating         int    `json:"rating"`
		Comment        string `json:"comment"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
		Title          string `json:"title"`
		PosterURL      string `json:"poster_url"`
		Year           int    `json:"year"`
		Anime          bool   `json:"anime"`
		MediaID        int    `json:"media_id"`
		AlbumID        int    `json:"album_id,omitempty"`
		ReleaseGroupID string `json:"release_group_id,omitempty"`
	}

	var items []ratingItem

	// Query movie/series ratings (skip when filtering for music only)
	if filterType != "music" {
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

		rows, err := s.db.Query(query, args...)
		if err != nil {
			writeError(w, 500, "database error: "+err.Error())
			return
		}
		defer rows.Close()

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
	}

	// Query music ratings from albums table (when "all" or "music")
	if filterType == "all" || filterType == "music" {
		musicRows, err := s.db.Query(`SELECT a.id, a.title, a.year, a.rating, a.rating_comment,
			a.release_group_id, a.image_url, a.updated_at, ar.name
			FROM albums a JOIN artists ar ON a.artist_id = ar.id
			WHERE a.rating IS NOT NULL AND a.rating > 0`)
		if err == nil {
			defer musicRows.Close()
			for musicRows.Next() {
				var albumID, rating int
				var title, releaseGroupID, artistName string
				var year sql.NullInt64
				var comment, imageURL sql.NullString
				var updatedAt sql.NullString
				if err := musicRows.Scan(&albumID, &title, &year, &rating, &comment,
					&releaseGroupID, &imageURL, &updatedAt, &artistName); err != nil {
					continue
				}
				item := ratingItem{
					MediaType:      "music",
					Rating:         rating,
					Title:          artistName + " - " + title,
					AlbumID:        albumID,
					ReleaseGroupID: releaseGroupID,
				}
				if comment.Valid {
					item.Comment = comment.String
				}
				if year.Valid {
					item.Year = int(year.Int64)
				}
				if imageURL.Valid {
					item.PosterURL = imageURL.String
				}
				if updatedAt.Valid {
					item.CreatedAt = updatedAt.String
					item.UpdatedAt = updatedAt.String
				}
				items = append(items, item)
			}
		}
	}

	// Sort in Go since results come from two tables
	if order != "asc" {
		order = "desc"
	}
	sort.Slice(items, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "date":
			less = strings.Compare(items[i].UpdatedAt, items[j].UpdatedAt) < 0
		case "title":
			less = strings.Compare(strings.ToLower(items[i].Title), strings.ToLower(items[j].Title)) < 0
		default: // rating
			if items[i].Rating != items[j].Rating {
				less = items[i].Rating < items[j].Rating
			} else {
				less = strings.Compare(items[i].UpdatedAt, items[j].UpdatedAt) < 0
			}
		}
		if order == "desc" {
			return !less
		}
		return less
	})

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
	if req.MediaType != "movie" && req.MediaType != "series" && req.MediaType != "music" {
		writeError(w, 400, "media_type must be 'movie', 'series', or 'music'")
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		writeError(w, 400, "rating must be between 1 and 5")
		return
	}

	// For music, tmdb_id is actually the album_id — update albums table directly
	if req.MediaType == "music" {
		_, err := s.db.Exec(`UPDATE albums SET rating = ?, rating_comment = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			req.Rating, req.Comment, req.TMDBID)
		if err != nil {
			writeError(w, 500, "database error: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "saved"})
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

	// For music, tmdbID is actually album_id — clear rating on albums table
	if mediaType == "music" {
		s.db.Exec(`UPDATE albums SET rating = NULL, rating_comment = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, tmdbID)
		writeJSON(w, 200, map[string]string{"status": "deleted"})
		return
	}

	s.db.Exec(`DELETE FROM user_ratings WHERE tmdb_id = ? AND media_type = ?`, tmdbID, mediaType)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
