package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mediaforge/internal/metadata"
)

// Last.fm album MBIDs may identify editions rather than release groups. Resolve
// provider names to an unambiguous MusicBrainz group before opening or saving.
func (s *Server) handleResolveMusicAlbum(w http.ResponseWriter, r *http.Request) {
	artist, title := strings.TrimSpace(r.URL.Query().Get("artist")), strings.TrimSpace(r.URL.Query().Get("title"))
	if metadata.NormalizeMusicText(artist) == "" || metadata.NormalizeMusicText(title) == "" || len(artist) > 300 || len(title) > 300 {
		writeError(w, 400, "artist and title are required (maximum 300 characters each)")
		return
	}
	year := 0
	if raw := r.URL.Query().Get("year"); raw != "" {
		var err error
		year, err = strconv.Atoi(raw)
		if err != nil || year < 1000 || year > 9999 {
			writeError(w, 400, "year must be a four-digit year")
			return
		}
	}
	if s.musicbrainz == nil {
		writeError(w, 503, "MusicBrainz client not initialized")
		return
	}
	matches := exactMusicGroups(s.musicbrainz.SearchCatalogReleaseGroups(title, 1000), artist, title, year)
	if len(matches) == 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		query := `releasegroup:"` + escapeMusicQuery(title) + `" AND artist:"` + escapeMusicQuery(artist) + `"`
		if year > 0 {
			query += ` AND firstreleasedate:` + strconv.Itoa(year) + `*`
		}
		result, err := s.musicbrainz.SearchReleaseGroupsContext(ctx, query)
		if err != nil {
			writeError(w, 502, "Could not verify album identity with MusicBrainz. Try again or use album search.")
			return
		}
		matches = exactMusicGroups(result.ReleaseGroups, artist, title, year)
		if result.Count > len(result.ReleaseGroups) {
			writeJSON(w, 409, map[string]any{"error": "More than one possible album. Use album search to choose the release group.", "matches": s.releaseGroupsToAlbumResults(matches, "")})
			return
		}
	}
	if len(matches) == 0 {
		writeError(w, 404, "No exact artist and album match found. Use album search to choose the release group.")
		return
	}
	if len(matches) > 1 {
		writeJSON(w, 409, map[string]any{"error": "Several albums share this artist and title. Use album search to choose the release group.", "matches": s.releaseGroupsToAlbumResults(matches, "")})
		return
	}
	album := s.releaseGroupsToAlbumResults(matches, "")[0]
	writeJSON(w, 200, struct {
		albumResult
		ReleaseGroupID string `json:"release_group_id"`
		ArtistMBID     string `json:"artist_mbid"`
		Provider       string `json:"provider"`
	}{album, matches[0].ID, album.ArtistID, "musicbrainz"})
}

func exactMusicGroups(groups []metadata.MBReleaseGroup, artist, title string, year int) []metadata.MBReleaseGroup {
	out := []metadata.MBReleaseGroup{}
	seen := map[string]bool{}
	artistKey, titleKey := metadata.NormalizeMusicText(artist), metadata.NormalizeMusicText(title)
	for _, group := range groups {
		if group.ID == "" || seen[group.ID] || metadata.NormalizeMusicText(group.Title) != titleKey || len(group.ArtistCredit) == 0 {
			continue
		}
		if year > 0 && (len(group.FirstRelease) < 4 || group.FirstRelease[:4] != strconv.Itoa(year)) {
			continue
		}
		var credit strings.Builder
		for _, c := range group.ArtistCredit {
			name := c.Name
			if name == "" {
				name = c.Artist.Name
			}
			credit.WriteString(name)
			credit.WriteString(c.JoinPhrase)
		}
		matched := metadata.NormalizeMusicText(credit.String()) == artistKey
		if len(group.ArtistCredit) == 1 {
			credited := group.ArtistCredit[0].Artist
			matched = matched || metadata.NormalizeMusicText(credited.Name) == artistKey
			for _, alias := range credited.Aliases {
				matched = matched || metadata.NormalizeMusicText(alias.Name) == artistKey
			}
		}
		if matched {
			seen[group.ID] = true
			out = append(out, group)
		}
	}
	return out
}
func escapeMusicQuery(value string) string {
	var out strings.Builder
	for _, r := range value {
		if strings.ContainsRune(`+-&|!(){}[]^"~*?:\/`, r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}
