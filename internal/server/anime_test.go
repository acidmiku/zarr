package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaforge/internal/metadata"
)

func animeTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestAnimeDiscoveryPreservesMovieAndSeriesTypes(t *testing.T) {
	s := newConnectionTestServer(t)
	// TMDB movie and TV IDs occupy separate namespaces; intentionally use the
	// same synthetic ID to prevent accidental cross-type library matches.
	if _, err := s.db.Exec(`INSERT INTO media_items (type,title,tmdb_id,anime) VALUES ('series','Chainsaw Man',100,TRUE)`); err != nil {
		t.Fatal(err)
	}
	s.tmdb = metadata.NewTMDBClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/3/search/tv":
			return animeTestResponse(`{"results":[{"id":100,"name":"Chainsaw Man","genre_ids":[16],"origin_country":["JP"]},{"id":101,"name":"American cartoon","genre_ids":[16],"origin_country":["US"]}]}`), nil
		case "/3/search/movie":
			return animeTestResponse(`{"results":[{"id":100,"title":"Chainsaw Man: Reze Arc","genre_ids":[16],"original_language":"ja"},{"id":102,"title":"Japanese live action","genre_ids":[28],"original_language":"ja"},{"id":103,"title":"American cartoon","genre_ids":[16],"original_language":"en"}]}`), nil
		case "/3/discover/tv", "/3/discover/movie":
			q := r.URL.Query()
			if q.Get("with_genres") != "16" || q.Get("with_origin_country") != "JP" || q.Get("page") != "2" {
				t.Errorf("anime discovery filters missing: %s", r.URL.Path)
			}
			if strings.HasSuffix(r.URL.Path, "/tv") {
				return animeTestResponse(`{"results":[{"id":100,"name":"Chainsaw Man","genre_ids":[16],"origin_country":["JP"]}]}`), nil
			}
			// Discover's origin filter is authoritative even when a movie entry
			// omits origin_country or uses an English original language.
			return animeTestResponse(`{"results":[{"id":100,"title":"Chainsaw Man: Reze Arc","genre_ids":[16],"original_language":"en"}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	})}, "test")
	for _, handler := range []http.HandlerFunc{s.handleSearch, s.handleTrending} {
		w := callDomain(handler, "GET", "/?type=anime&q=Chainsaw%20Man&page=2", "", "", "")
		var items []tmdbResultEntry
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 2 {
			t.Fatalf("anime response %d: %s", w.Code, w.Body.String())
		}
		if items[0].Type != "series" || !items[0].IsAnime || !items[0].InLibrary || items[0].Title != "Chainsaw Man" {
			t.Fatalf("series classification: %+v", items[0])
		}
		if items[1].Type != "movie" || !items[1].IsAnime || items[1].InLibrary || items[1].Title != "Chainsaw Man: Reze Arc" {
			t.Fatalf("movie classification: %+v", items[1])
		}
	}
}

func TestAnimeDiscoveryReturnsEmptyArrayAndReportsPartialFailure(t *testing.T) {
	for _, failMovie := range []bool{false, true} {
		s := newConnectionTestServer(t)
		s.tmdb = metadata.NewTMDBClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
			response := animeTestResponse(`{"results":[]}`)
			if failMovie && strings.HasSuffix(r.URL.Path, "/movie") {
				response.StatusCode = 503
			}
			return response, nil
		})}, "test")
		w := callDomain(s.handleSearch, "GET", "/?type=anime&q=missing", "", "", "")
		if failMovie {
			if w.Code != 500 {
				t.Fatalf("partial results reported as complete: %s", w.Body.String())
			}
		} else if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatalf("empty results: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestAnimeMetadataAndLibraryCreationKeepMovieSeparateFromSeries(t *testing.T) {
	for _, kind := range []string{"movie", "series", "movie-override"} {
		t.Run(kind, func(t *testing.T) {
			s := newConnectionTestServer(t)
			mediaType := strings.Split(kind, "-")[0]
			s.tmdb = metadata.NewTMDBClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/3/movie/100":
					if kind == "movie-override" {
						return animeTestResponse(`{"id":100,"title":"Chainsaw Man: Reze Arc"}`), nil
					}
					return animeTestResponse(`{"id":100,"title":"Chainsaw Man: Reze Arc","release_date":"2025-09-19","genres":[{"id":16,"name":"Localized animation"}],"origin_country":["JP"]}`), nil
				case "/3/tv/100":
					return animeTestResponse(`{"id":100,"name":"Chainsaw Man","first_air_date":"2022-10-12","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"seasons":[{"season_number":1,"name":"Season 1","episode_count":1}]}`), nil
				case "/3/tv/100/season/1":
					return animeTestResponse(`{"season_number":1,"episodes":[{"id":1,"episode_number":1,"season_number":1,"name":"Dog & Chainsaw"}]}`), nil
				default:
					return nil, fmt.Errorf("wrong metadata endpoint %s", r.URL.Path)
				}
			})}, "test")
			w := callDomain(s.handleMetadata, "GET", "/?type="+mediaType, "", "tmdbID", "100")
			var detail struct {
				Type    string `json:"type"`
				IsAnime bool   `json:"is_anime"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.Type != mediaType || detail.IsAnime != (kind != "movie-override") {
				t.Fatalf("metadata classification: %s", w.Body.String())
			}
			w = callDomain(s.handleAddToLibrary, "POST", "/", fmt.Sprintf(`{"tmdb_id":100,"type":%q,"anime":%t}`, mediaType, kind == "movie-override"), "", "")
			if w.Code != 201 {
				t.Fatalf("library add: %d %s", w.Code, w.Body.String())
			}
			var savedType, rootPath string
			var anime bool
			if err := s.db.QueryRow(`SELECT type,anime,COALESCE(root_path,'') FROM media_items WHERE tmdb_id=100`).Scan(&savedType, &anime, &rootPath); err != nil {
				t.Fatal(err)
			}
			if savedType != mediaType || !anime || rootPath != "" {
				t.Fatalf("saved %s anime=%t path=%q", savedType, anime, rootPath)
			}
			var seasons, episodes int
			s.db.QueryRow(`SELECT count(*) FROM seasons`).Scan(&seasons)
			s.db.QueryRow(`SELECT count(*) FROM episodes`).Scan(&episodes)
			want := 0
			if mediaType == "series" {
				want = 1
			}
			if seasons != want || episodes != want {
				t.Fatalf("%s: seasons=%d episodes=%d", mediaType, seasons, episodes)
			}
			w = callDomain(s.handleListLibrary, "GET", "/?type=anime", "", "", "")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"`+mediaType+`"`) {
				t.Fatalf("missing from Anime library: %s", w.Body.String())
			}
		})
	}
}

func TestAniListMovieDoesNotCreateFakeSeason(t *testing.T) {
	s := newConnectionTestServer(t)
	s.anilist = metadata.NewAniListClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		return animeTestResponse(`{"data":{"Media":{"id":100,"title":{"english":"Chainsaw Man: Reze Arc"},"format":"MOVIE","episodes":1,"startDate":{"year":2025}}}}`), nil
	})})
	metadataResponse := callDomain(s.handleAnilistMetadata, "GET", "/", "", "anilistID", "100")
	var detail struct {
		Type    string `json:"type"`
		IsAnime bool   `json:"is_anime"`
	}
	if metadataResponse.Code != 200 || json.Unmarshal(metadataResponse.Body.Bytes(), &detail) != nil || detail.Type != "movie" || !detail.IsAnime {
		t.Fatalf("AniList metadata classification: %s", metadataResponse.Body.String())
	}
	w := callDomain(s.handleAddToLibrary, "POST", "/", `{"anilist_id":100}`, "", "")
	if w.Code != 201 {
		t.Fatalf("AniList add: %d %s", w.Code, w.Body.String())
	}
	var kind string
	var anime bool
	if err := s.db.QueryRow(`SELECT type,anime FROM media_items WHERE anilist_id=100`).Scan(&kind, &anime); err != nil {
		t.Fatal(err)
	}
	var seasons, episodes int
	s.db.QueryRow(`SELECT count(*) FROM seasons`).Scan(&seasons)
	s.db.QueryRow(`SELECT count(*) FROM episodes`).Scan(&episodes)
	if kind != "movie" || !anime || seasons != 0 || episodes != 0 {
		t.Fatalf("AniList movie: %s anime=%t seasons=%d episodes=%d", kind, anime, seasons, episodes)
	}
}

func TestImportAnimeMoviePreservesAnimeClassification(t *testing.T) {
	s := newConnectionTestServer(t)
	s.cfg.MediaRoot = t.TempDir()
	s.tmdb = metadata.NewTMDBClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		return animeTestResponse(`{"id":100,"title":"Chainsaw Man Reze Arc","release_date":"2025-09-19","genres":[{"id":16}],"original_language":"ja"}`), nil
	})}, "test")
	source := filepath.Join(t.TempDir(), "Reze.mkv")
	if err := os.WriteFile(source, []byte("test video"), 0600); err != nil {
		t.Fatal(err)
	}
	result := s.importMovie(importItem{SourcePath: source, TMDBID: 100, Type: "movie", QualityProfileID: 1})
	if result.Status != "imported" {
		t.Fatalf("import failed: %+v", result)
	}
	var kind, root string
	var anime bool
	if err := s.db.QueryRow(`SELECT type,anime,root_path FROM media_items WHERE id=?`, result.LibraryID).Scan(&kind, &anime, &root); err != nil {
		t.Fatal(err)
	}
	if kind != "movie" || !anime || filepath.Base(filepath.Dir(root)) != "movies" {
		t.Fatalf("import classification: %s anime=%t root=%s", kind, anime, root)
	}
}
