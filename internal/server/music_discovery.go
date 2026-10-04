package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mediaforge/internal/database"
	"mediaforge/internal/metadata"
)

type musicDiscoveryAlbum struct {
	ID             string   `json:"id"`
	ReleaseGroupID string   `json:"release_group_id"`
	LibraryID      int64    `json:"library_id"`
	ArtistID       string   `json:"artist_id"`
	ArtistMBID     string   `json:"artist_mbid"`
	Title          string   `json:"title"`
	Artist         string   `json:"artist"`
	Year           int      `json:"year"`
	Type           string   `json:"type"`
	Genres         []string `json:"genres"`
	CoverURL       string   `json:"cover_url"`
	Reason         string   `json:"reason"`
	InLibrary      bool     `json:"in_library"`
	Favorite       bool     `json:"favorite"`
}
type musicDiscoveryArtist struct {
	ID         string `json:"id"`
	ArtistMBID string `json:"artist_mbid"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
	CoverURL   string `json:"cover_url"`
}
type musicDiscoverySeed struct {
	artistID, artist, title string
	genres                  []string
	weight                  int
	history                 bool
}
type musicDiscoveryResponse struct {
	Recommendations []musicDiscoveryAlbum  `json:"recommendations"`
	RecentlySaved   []musicDiscoveryAlbum  `json:"recently_saved"`
	SimilarArtists  []musicDiscoveryArtist `json:"similar_artists"`
	PlayedNotOwned  []musicDiscoveryAlbum  `json:"played_not_owned"`
	Configured      map[string]bool        `json:"configured"`
	CatalogCount    int                    `json:"catalog_count"`
	Enriching       bool                   `json:"enriching"`
}

// A shelf request uses the local catalog only. Network enrichment runs with a
// short independent deadline, once per library at a time, and rotates seeds.
func (s *Server) handleMusicDiscover(w http.ResponseWriter, r *http.Request) {
	username, _ := s.db.GetSetting("lastfm_username")
	username = strings.TrimSpace(username)
	result := musicDiscoveryResponse{Recommendations: []musicDiscoveryAlbum{}, RecentlySaved: []musicDiscoveryAlbum{}, SimilarArtists: []musicDiscoveryArtist{}, PlayedNotOwned: []musicDiscoveryAlbum{}, Configured: map[string]bool{"musicbrainz": s.musicbrainz != nil, "lastfm": s.lastfm != nil, "lastfm_history": s.lastfm != nil && username != ""}}
	rows, err := s.db.QueryContext(r.Context(), `SELECT a.id,COALESCE(a.release_group_id,''),a.title,ar.name,COALESCE(ar.mbid,''),COALESCE(a.year,0),COALESCE(a.album_type,'Album'),COALESCE(a.rating,0),a.favorite,COALESCE(cr.genres,'[]'),COALESCE(ca.genres,'[]') FROM albums a JOIN artists ar ON ar.id=a.artist_id LEFT JOIN music_catalog_releases cr ON cr.release_group_id=a.release_group_id LEFT JOIN music_catalog_artists ca ON ca.mbid=ar.mbid ORDER BY a.created_at DESC,a.id DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not load music library"})
		return
	}
	seeds := []musicDiscoverySeed{}
	saved := map[string]bool{}
	for rows.Next() {
		var a musicDiscoveryAlbum
		var rating int
		var albumGenres, artistGenres string
		if err = rows.Scan(&a.LibraryID, &a.ReleaseGroupID, &a.Title, &a.Artist, &a.ArtistMBID, &a.Year, &a.Type, &rating, &a.Favorite, &albumGenres, &artistGenres); err != nil {
			break
		}
		a.ID = a.ReleaseGroupID
		a.ArtistID = a.ArtistMBID
		a.InLibrary = true
		a.Genres = discoveryGenres(albumGenres, artistGenres)
		a.CoverURL = discoveryCover(a.ReleaseGroupID)
		a.Reason = "Recently saved to your library"
		if len(result.RecentlySaved) < 20 {
			result.RecentlySaved = append(result.RecentlySaved, a)
		}
		saved[a.ReleaseGroupID] = true
		if a.ArtistMBID != "" && (rating == 0 || rating >= 3 || a.Favorite) {
			weight := 1
			if rating >= 4 {
				weight += 2
			}
			if a.Favorite {
				weight += 3
			}
			seeds = append(seeds, musicDiscoverySeed{a.ArtistMBID, a.Artist, a.Title, a.Genres, weight, false})
		}
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		writeJSON(w, 500, map[string]string{"error": "could not load music library"})
		return
	}
	played := []metadata.LFMAlbum{}
	if s.lastfm != nil && username != "" {
		played = s.lastfm.CachedUserTopAlbums(username)
		for _, album := range played {
			var name, genres string
			if album.Artist.MBID != "" && s.db.QueryRowContext(r.Context(), `SELECT name,genres FROM music_catalog_artists WHERE mbid=?`, album.Artist.MBID).Scan(&name, &genres) == nil && metadata.NormalizeMusicText(name) == metadata.NormalizeMusicText(album.Artist.Name) {
				seeds = append(seeds, musicDiscoverySeed{album.Artist.MBID, name, album.Name, discoveryGenres(genres), 2, true})
			}
		}
	}
	sort.SliceStable(seeds, func(i, j int) bool { return seeds[i].weight > seeds[j].weight })
	artistSeeds, genreSeeds := map[string]musicDiscoverySeed{}, map[string]musicDiscoverySeed{}
	for _, seed := range seeds {
		if _, exists := artistSeeds[seed.artistID]; !exists {
			artistSeeds[seed.artistID] = seed
		}
		for _, genre := range seed.genres {
			key := metadata.NormalizeMusicText(genre)
			if _, exists := genreSeeds[key]; !exists && key != "" {
				genreSeeds[key] = seed
			}
		}
	}
	rows, err = s.db.QueryContext(r.Context(), `SELECT r.release_group_id,r.title,r.artist,r.artist_mbid,r.year,r.release_type,r.genres,COALESCE(a.genres,'[]') FROM music_catalog_releases r LEFT JOIN music_catalog_artists a ON a.mbid=r.artist_mbid ORDER BY r.year DESC,r.title COLLATE NOCASE,r.release_group_id LIMIT 10000`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not load music catalog"})
		return
	}
	type scoredAlbum struct {
		album musicDiscoveryAlbum
		score int
	}
	candidates := []scoredAlbum{}
	catalog := []musicDiscoveryAlbum{}
	for rows.Next() {
		var a musicDiscoveryAlbum
		var albumGenres, artistGenres string
		if err = rows.Scan(&a.ReleaseGroupID, &a.Title, &a.Artist, &a.ArtistMBID, &a.Year, &a.Type, &albumGenres, &artistGenres); err != nil {
			break
		}
		result.CatalogCount++
		a.ID = a.ReleaseGroupID
		a.ArtistID = a.ArtistMBID
		a.Genres = discoveryGenres(albumGenres, artistGenres)
		a.CoverURL = discoveryCover(a.ReleaseGroupID)
		catalog = append(catalog, a)
		if saved[a.ReleaseGroupID] || musicDiscoveryTypePriority(a.Type) == 0 {
			continue
		}
		score := 0
		if seed, exists := artistSeeds[a.ArtistMBID]; exists {
			score = 100 + seed.weight*10
			a.Reason = "More from " + seed.artist + ", an artist in your library"
			if seed.history {
				a.Reason = "More from " + seed.artist + ", from your Last.fm listening history"
			}
			if seed.weight >= 3 {
				a.Reason = "Because you liked " + seed.title + " by " + seed.artist
			}
		} else {
			for _, genre := range a.Genres {
				if seed, exists := genreSeeds[metadata.NormalizeMusicText(genre)]; exists && seed.weight*5 > score {
					score = seed.weight * 5
					a.Reason = "Shares " + genre + " with " + seed.title + " by " + seed.artist
					if seed.history {
						a.Reason += " in your Last.fm listening history"
					}
				}
			}
		}
		if score == 0 {
			if len(seeds) > 0 {
				continue
			}
			a.Reason = "From your cached catalog"
		}
		candidates = append(candidates, scoredAlbum{a, score})
	}
	rowErr = rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		writeJSON(w, 500, map[string]string{"error": "could not load music catalog"})
		return
	}
	result.PlayedNotOwned = playedCatalogAlbums(played, catalog, saved)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return musicDiscoveryTypePriority(candidates[i].album.Type) > musicDiscoveryTypePriority(candidates[j].album.Type)
	})
	for _, candidate := range candidates {
		result.Recommendations = append(result.Recommendations, candidate.album)
		if len(result.Recommendations) == 18 {
			break
		}
	}
	if s.lastfm != nil {
		seen := map[string]bool{}
		queried := map[string]bool{}
		for _, seed := range seeds {
			seen[seed.artistID] = true
		}
		for _, seed := range seeds {
			if queried[seed.artistID] {
				continue
			}
			queried[seed.artistID] = true
			for _, artist := range s.lastfm.CachedSimilarArtists(seed.artist, 12) {
				if artist.MBID == "" || seen[artist.MBID] {
					continue
				}
				seen[artist.MBID] = true
				result.SimilarArtists = append(result.SimilarArtists, musicDiscoveryArtist{ID: artist.MBID, ArtistMBID: artist.MBID, Name: artist.Name, Reason: "Similar to " + seed.artist})
				if len(result.SimilarArtists) == 12 {
					break
				}
			}
			if len(queried) == 3 || len(result.SimilarArtists) == 12 {
				break
			}
		}
	}
	result.Enriching = startMusicDiscoveryEnrichment(s.db, s.musicbrainz, s.lastfm, seeds, username)
	writeJSON(w, 200, result)
}

// Discographies retain broadcasts and miscellaneous releases for explicit
// browsing. Recommendations favor albums and EPs instead of letting a recent
// radio recording or a run of singles crowd out an artist's albums.
func musicDiscoveryTypePriority(kind string) int {
	switch strings.ToLower(kind) {
	case "album":
		return 3
	case "ep":
		return 2
	case "single":
		return 1
	default:
		return 0
	}
}

func discoveryCover(id string) string {
	if id == "" {
		return ""
	}
	return "/api/music/cover?rgid=" + url.QueryEscape(id)
}
func discoveryGenres(groups ...string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range groups {
		var values []string
		json.Unmarshal([]byte(raw), &values)
		for _, g := range values {
			key := metadata.NormalizeMusicText(g)
			if key != "" && !seen[key] {
				seen[key] = true
				out = append(out, strings.TrimSpace(g))
			}
		}
	}
	return out
}

type musicEnrichmentState struct {
	sync.Mutex
	running bool
	last    time.Time
	next    int
}

var musicEnrichments sync.Map

func startMusicDiscoveryEnrichment(db *database.DB, mb *metadata.MusicBrainzClient, lfm *metadata.LastFMClient, seeds []musicDiscoverySeed, username string) bool {
	if (mb == nil || len(seeds) == 0) && (lfm == nil || username == "") {
		return false
	}
	value, _ := musicEnrichments.LoadOrStore(db, &musicEnrichmentState{})
	state := value.(*musicEnrichmentState)
	state.Lock()
	defer state.Unlock()
	if state.running {
		return true
	}
	if time.Since(state.last) < time.Minute {
		return false
	}
	unique := []musicDiscoverySeed{}
	seen := map[string]bool{}
	for _, seed := range seeds {
		if !seen[seed.artistID] {
			seen[seed.artistID] = true
			unique = append(unique, seed)
		}
	}
	var seed musicDiscoverySeed
	if len(unique) > 0 {
		seed = unique[state.next%len(unique)]
	}
	state.next++
	state.last = time.Now()
	state.running = true
	go func() {
		defer func() { state.Lock(); state.running = false; state.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		var group sync.WaitGroup
		if lfm != nil && seed.artist != "" {
			group.Add(1)
			go func() { defer group.Done(); lfm.SimilarArtistsContext(ctx, seed.artist, 12) }()
		}
		if lfm != nil && username != "" {
			group.Add(1)
			go func() { defer group.Done(); lfm.UserTopAlbumsContext(ctx, username) }()
		}
		if mb != nil && seed.artistID != "" {
			if _, err := mb.GetArtistContext(ctx, seed.artistID); err == nil {
				mb.SearchArtistReleaseGroupsContext(ctx, seed.artistID)
			}
		}
		group.Wait()
	}()
	return true
}

// Provider album IDs are deliberately ignored: only a unique exact local
// artist/title match establishes the canonical release-group identity.
func playedCatalogAlbums(played []metadata.LFMAlbum, catalog []musicDiscoveryAlbum, saved map[string]bool) []musicDiscoveryAlbum {
	identity := func(artist, title string) string {
		return metadata.NormalizeMusicText(artist) + "\x00" + metadata.NormalizeMusicText(title)
	}
	byIdentity := map[string][]musicDiscoveryAlbum{}
	for _, album := range catalog {
		key := identity(album.Artist, album.Title)
		byIdentity[key] = append(byIdentity[key], album)
	}
	out := []musicDiscoveryAlbum{}
	seen := map[string]bool{}
	for _, item := range played {
		matches := byIdentity[identity(item.Artist.Name, item.Name)]
		if len(matches) != 1 {
			continue
		}
		album := matches[0]
		if saved[album.ReleaseGroupID] || seen[album.ReleaseGroupID] {
			continue
		}
		seen[album.ReleaseGroupID] = true
		album.Reason = "From your last six months on Last.fm"
		if plays, err := strconv.Atoi(item.Playcount); err == nil && plays > 0 {
			album.Reason = "Played " + strconv.Itoa(plays) + " times on Last.fm in the last six months"
		}
		out = append(out, album)
		if len(out) == 18 {
			break
		}
	}
	return out
}
