package ai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"mediaforge/internal/metadata"
	"strings"
)

// ToolDefs returns the OpenAI-format tool definitions for the AI assistant.
func ToolDefs(braveAvailable bool) []Tool {
	tools := []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "show_recommendations",
				Description: "Display recommendation cards to the user with posters and personalized explanations. Use this whenever you want to suggest specific titles for the user to watch. Shows visual cards in the UI.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"recommendations": {
							"type": "array",
							"items": {
								"type": "object",
								"properties": {
									"mal_id": {"type": "integer", "description": "MyAnimeList ID of the anime/movie. Use search_mal first to find this."},
									"title": {"type": "string", "description": "Title of the recommendation"},
									"media_type": {"type": "string", "enum": ["anime", "movie", "series"], "description": "Type of media"},
									"reason": {"type": "string", "description": "2-3 sentence personalized explanation of why the user would enjoy this, referencing their taste/ratings where relevant"}
								},
								"required": ["title", "reason", "media_type"]
							},
							"minItems": 1,
							"maxItems": 6,
							"description": "3-5 recommendations is ideal. Each must have a personalized reason."
						}
					},
					"required": ["recommendations"]
				}`),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_mal",
				Description: "Search MyAnimeList for anime titles using the Jikan API. Use this to find specific anime, discover titles by genre/score/type, look up current season anime, or find MAL IDs for recommendations. Always use this before show_recommendations to get accurate MAL IDs and poster images.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {"type": "string", "description": "Search text (anime title, keywords). Optional if using genre/type filters."},
						"type": {"type": "string", "enum": ["tv", "movie", "ova", "special", "ona"], "description": "Filter by anime type"},
						"min_score": {"type": "number", "description": "Minimum MAL score (0-10), e.g. 7.5"},
						"genres": {"type": "string", "description": "Comma-separated genre IDs. Key genres: 1=Action, 2=Adventure, 4=Comedy, 8=Drama, 10=Fantasy, 14=Horror, 7=Mystery, 22=Romance, 24=Sci-Fi, 37=Supernatural, 40=Psychological, 41=Suspense, 62=Isekai"},
						"status": {"type": "string", "enum": ["airing", "complete", "upcoming"]},
						"order_by": {"type": "string", "enum": ["score", "rank", "popularity", "members", "start_date"]},
						"sort": {"type": "string", "enum": ["asc", "desc"], "description": "Sort direction, default desc"},
						"limit": {"type": "integer", "description": "Max results (1-25), default 10"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_user_ratings",
				Description: "Retrieve the user's ratings and comments from their Zarr library. Use this to understand the user's taste when they haven't provided their preferences directly, or to reference specific titles they've rated.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"min_rating": {"type": "integer", "description": "Minimum rating to filter (1-5), default 1"},
						"type": {"type": "string", "enum": ["all", "movie", "series", "anime"], "description": "Filter by media type, default all"}
					}
				}`),
			},
		},
	}

	tools = append(tools, Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        "save_ratings",
			Description: "Save user ratings for titles they've already watched. Use this when the user mentions they've seen something, provides a list of watched titles, or wants to rate something they've already seen. Each item is looked up on TMDB automatically — you only need to provide the title.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"ratings": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"title": {"type": "string", "description": "Title of the movie/series/anime"},
								"media_type": {"type": "string", "enum": ["movie", "series"], "description": "Type of media. Use 'series' for both TV shows and anime."},
								"score": {"type": "integer", "minimum": 1, "maximum": 5, "description": "Rating 1-5 stars. Default 5 if user doesn't specify. Infer from sentiment: loved=5, great=4, ok=3, meh=2, bad=1."},
								"comment": {"type": "string", "description": "Optional user comment or note about the title"},
								"anime": {"type": "boolean", "description": "Whether this is anime. Default false."}
							},
							"required": ["title", "media_type"]
						},
						"description": "Array of ratings to save. Can be 1 item or many for batch imports."
					}
				},
				"required": ["ratings"]
			}`),
		},
	})

	if braveAvailable {
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_search",
				Description: "Search the web for current information about anime, movies, TV series. Use for: current season discussions, recent news, reviews, comparisons, rankings that need fresh data. Only available if the user has configured a Brave API key.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {"type": "string", "description": "Search query"}
					},
					"required": ["query"]
				}`),
			},
		})
	}

	return tools
}

// Recommendation is a single recommendation from the AI.
type Recommendation struct {
	MalID     int    `json:"mal_id,omitempty"`
	Title     string `json:"title"`
	MediaType string `json:"media_type"`
	Reason    string `json:"reason"`
	PosterURL string `json:"poster_url,omitempty"`
	Score     float64 `json:"score,omitempty"`
}

// ToolExecutor handles tool call execution.
type ToolExecutor struct {
	Jikan *JikanClient
	Brave *BraveClient
	DB    *sql.DB
	TMDB  *metadata.TMDBClient
}

// ExecuteTool runs a tool call and returns the result string plus any recommendations to emit.
func (e *ToolExecutor) ExecuteTool(name, argsJSON string) (result string, recs []Recommendation, err error) {
	switch name {
	case "show_recommendations":
		return e.execShowRecommendations(argsJSON)
	case "search_mal":
		return e.execSearchMAL(argsJSON)
	case "web_search":
		return e.execWebSearch(argsJSON)
	case "get_user_ratings":
		return e.execGetUserRatings(argsJSON)
	case "save_ratings":
		return e.execSaveRatings(argsJSON)
	default:
		return fmt.Sprintf("Unknown tool: %s", name), nil, nil
	}
}

func (e *ToolExecutor) execShowRecommendations(argsJSON string) (string, []Recommendation, error) {
	var args struct {
		Recommendations []struct {
			MalID     int    `json:"mal_id"`
			Title     string `json:"title"`
			MediaType string `json:"media_type"`
			Reason    string `json:"reason"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "Failed to parse recommendations arguments", nil, nil
	}

	var recs []Recommendation
	for _, r := range args.Recommendations {
		rec := Recommendation{
			MalID:     r.MalID,
			Title:     r.Title,
			MediaType: r.MediaType,
			Reason:    r.Reason,
		}

		// Set poster URL for lazy loading via image proxy (no API call here to avoid rate limits)
		if r.MalID > 0 {
			rec.PosterURL = fmt.Sprintf("/api/image/jikan/%d", r.MalID)
		}

		recs = append(recs, rec)
	}

	return fmt.Sprintf("Displayed %d recommendation cards to the user.", len(recs)), recs, nil
}

func (e *ToolExecutor) execSearchMAL(argsJSON string) (string, []Recommendation, error) {
	var args struct {
		Query    string  `json:"query"`
		Type     string  `json:"type"`
		MinScore float64 `json:"min_score"`
		Genres   string  `json:"genres"`
		Status   string  `json:"status"`
		OrderBy  string  `json:"order_by"`
		Sort     string  `json:"sort"`
		Limit    int     `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "Failed to parse search arguments", nil, nil
	}

	results, err := e.Jikan.SearchAnime(SearchParams{
		Query:    args.Query,
		Type:     args.Type,
		MinScore: args.MinScore,
		Genres:   args.Genres,
		Status:   args.Status,
		OrderBy:  args.OrderBy,
		Sort:     args.Sort,
		Limit:    args.Limit,
	})
	if err != nil {
		return fmt.Sprintf("MAL search error: %s", err.Error()), nil, nil
	}

	if len(results) == 0 {
		return "No results found.", nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results:\n\n", len(results)))
	for _, a := range results {
		synopsis := a.Synopsis
		if len(synopsis) > 200 {
			synopsis = synopsis[:200] + "..."
		}

		genres := make([]string, len(a.Genres))
		for i, g := range a.Genres {
			genres[i] = g.Name
		}

		sb.WriteString(fmt.Sprintf("- **%s** (MAL ID: %d)\n", a.DisplayTitle(), a.MalID))
		sb.WriteString(fmt.Sprintf("  Type: %s | Score: %.2f | Year: %d | Status: %s\n", a.Type, a.Score, a.Year, a.Status))
		if len(genres) > 0 {
			sb.WriteString(fmt.Sprintf("  Genres: %s\n", strings.Join(genres, ", ")))
		}
		if synopsis != "" {
			sb.WriteString(fmt.Sprintf("  Synopsis: %s\n", synopsis))
		}
		sb.WriteByte('\n')
	}

	return sb.String(), nil, nil
}

func (e *ToolExecutor) execWebSearch(argsJSON string) (string, []Recommendation, error) {
	if e.Brave == nil || e.Brave.apiKey == "" {
		return "Web search is not available. The user hasn't configured a Brave API key. Suggest they add one in Settings, or work with your existing knowledge.", nil, nil
	}

	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "Failed to parse search arguments", nil, nil
	}

	results, err := e.Brave.Search(args.Query, 5)
	if err != nil {
		return fmt.Sprintf("Web search error: %s", err.Error()), nil, nil
	}

	if len(results) == 0 {
		return "No web results found.", nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Web search results for '%s':\n\n", args.Query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		sb.WriteString(fmt.Sprintf("   %s\n\n", r.Description))
	}

	return sb.String(), nil, nil
}

func (e *ToolExecutor) execGetUserRatings(argsJSON string) (string, []Recommendation, error) {
	var args struct {
		MinRating int    `json:"min_rating"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args.MinRating = 1
		args.Type = "all"
	}
	if args.MinRating < 1 {
		args.MinRating = 1
	}
	if args.Type == "" {
		args.Type = "all"
	}

	query := `SELECT r.tmdb_id, r.media_type, r.rating, r.comment,
		COALESCE(m.title, 'Unknown Title') as title
		FROM user_ratings r
		LEFT JOIN media_items m ON m.tmdb_id = r.tmdb_id AND m.type = r.media_type
		WHERE r.rating >= ?`
	qargs := []interface{}{args.MinRating}

	if args.Type != "all" {
		query += " AND r.media_type = ?"
		qargs = append(qargs, args.Type)
	}
	query += " ORDER BY r.rating DESC, r.updated_at DESC"

	rows, err := e.DB.Query(query, qargs...)
	if err != nil {
		return fmt.Sprintf("Database error: %s", err.Error()), nil, nil
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("User ratings:\n\n")
	count := 0
	for rows.Next() {
		var tmdbID, rating int
		var mediaType, title string
		var comment sql.NullString
		if err := rows.Scan(&tmdbID, &mediaType, &rating, &comment, &title); err != nil {
			continue
		}
		stars := strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
		commentStr := ""
		if comment.Valid && comment.String != "" {
			commentStr = fmt.Sprintf(` — "%s"`, comment.String)
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s%s\n", title, mediaType, stars, commentStr))
		count++
	}

	if count == 0 {
		return "No ratings found. The user hasn't rated any titles yet.", nil, nil
	}

	return sb.String(), nil, nil
}

func (e *ToolExecutor) execSaveRatings(argsJSON string) (string, []Recommendation, error) {
	var args struct {
		Ratings []struct {
			Title     string `json:"title"`
			MediaType string `json:"media_type"`
			Score     int    `json:"score"`
			Comment   string `json:"comment"`
			Anime     bool   `json:"anime"`
		} `json:"ratings"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "Failed to parse save_ratings arguments", nil, nil
	}

	if len(args.Ratings) == 0 {
		return "No ratings provided.", nil, nil
	}

	if e.TMDB == nil {
		return "TMDB client not available. Cannot look up titles.", nil, nil
	}

	var saved []string
	var failed []string

	for _, r := range args.Ratings {
		if r.Title == "" {
			continue
		}
		if r.MediaType != "movie" && r.MediaType != "series" {
			r.MediaType = "series"
		}
		if r.Score < 1 || r.Score > 5 {
			r.Score = 5
		}

		// Search TMDB for the title
		var tmdbID int
		var resolvedTitle, posterURL string
		var year int

		if r.MediaType == "movie" {
			result, err := e.TMDB.SearchMovies(r.Title)
			if err != nil || len(result.Results) == 0 {
				failed = append(failed, r.Title)
				continue
			}
			entry := result.Results[0]
			tmdbID = entry.ID
			resolvedTitle = entry.Title
			if entry.PosterPath != "" {
				posterURL = "https://image.tmdb.org/t/p/w300" + entry.PosterPath
			}
			if len(entry.ReleaseDate) >= 4 {
				fmt.Sscanf(entry.ReleaseDate[:4], "%d", &year)
			}
		} else {
			result, err := e.TMDB.SearchTV(r.Title)
			if err != nil || len(result.Results) == 0 {
				failed = append(failed, r.Title)
				continue
			}
			entry := result.Results[0]
			tmdbID = entry.ID
			resolvedTitle = entry.Name
			if entry.PosterPath != "" {
				posterURL = "https://image.tmdb.org/t/p/w300" + entry.PosterPath
			}
			if len(entry.FirstAirDate) >= 4 {
				fmt.Sscanf(entry.FirstAirDate[:4], "%d", &year)
			}
		}

		// Upsert rating
		_, err := e.DB.Exec(`INSERT INTO user_ratings (tmdb_id, media_type, rating, comment, title, poster_url, year, anime, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
			ON CONFLICT(tmdb_id, media_type) DO UPDATE SET
				rating = excluded.rating,
				comment = CASE WHEN excluded.comment != '' THEN excluded.comment ELSE user_ratings.comment END,
				title = excluded.title,
				poster_url = excluded.poster_url,
				year = excluded.year,
				anime = excluded.anime,
				updated_at = datetime('now')`,
			tmdbID, r.MediaType, r.Score, r.Comment, resolvedTitle, posterURL, year, r.Anime)
		if err != nil {
			failed = append(failed, r.Title)
			continue
		}

		stars := strings.Repeat("★", r.Score) + strings.Repeat("☆", 5-r.Score)
		saved = append(saved, fmt.Sprintf("%s %s", resolvedTitle, stars))
	}

	var sb strings.Builder
	if len(saved) > 0 {
		sb.WriteString(fmt.Sprintf("Saved %d rating(s):\n", len(saved)))
		for _, s := range saved {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}
	}
	if len(failed) > 0 {
		sb.WriteString(fmt.Sprintf("\nCouldn't find %d title(s) on TMDB: %s", len(failed), strings.Join(failed, ", ")))
	}

	return sb.String(), nil, nil
}
