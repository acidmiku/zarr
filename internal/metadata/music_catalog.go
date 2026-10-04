package metadata

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func NormalizeMusicText(value string) string {
	var out strings.Builder
	for _, r := range norm.NFKD.String(strings.ToLower(value)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}
func GenreNames(genres, tags []MBTag) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, group := range [][]MBTag{genres, tags} {
		for _, tag := range group {
			name := strings.TrimSpace(tag.Name)
			key := NormalizeMusicText(name)
			if key != "" && !seen[key] && tag.Count >= 0 {
				seen[key] = true
				out = append(out, name)
				if len(out) >= 12 {
					return out
				}
			}
		}
	}
	return out
}
func (c *MusicBrainzClient) cachedArtist(id string) *MBArtist {
	if c.db == nil {
		return nil
	}
	var data string
	if c.db.QueryRow(`SELECT metadata FROM music_catalog_artists WHERE mbid=?`, id).Scan(&data) != nil {
		return nil
	}
	var artist MBArtist
	if json.Unmarshal([]byte(data), &artist) != nil {
		return nil
	}
	return &artist
}
func (c *MusicBrainzClient) ingestArtist(artist MBArtist, full bool) {
	if c.db == nil || artist.ID == "" || artist.Name == "" {
		return
	}
	if old := c.cachedArtist(artist.ID); old != nil && !full {
		if len(artist.Aliases) == 0 {
			artist.Aliases = old.Aliases
		}
		if len(artist.Genres) == 0 {
			artist.Genres = old.Genres
		}
		if len(artist.Tags) == 0 {
			artist.Tags = old.Tags
		}
		if artist.Disambiguation == "" {
			artist.Disambiguation = old.Disambiguation
		}
		if artist.SortName == "" {
			artist.SortName = old.SortName
		}
	}
	aliases := []string{}
	for _, alias := range artist.Aliases {
		if alias.Name != "" {
			aliases = append(aliases, alias.Name)
		}
	}
	aliasJSON, _ := json.Marshal(aliases)
	genres, _ := json.Marshal(GenreNames(artist.Genres, artist.Tags))
	data, _ := json.Marshal(artist)
	c.db.Exec(`INSERT INTO music_catalog_artists(mbid,name,sort_name,aliases,genres,metadata) VALUES(?,?,?,?,?,?) ON CONFLICT(mbid) DO UPDATE SET name=excluded.name,sort_name=excluded.sort_name,aliases=excluded.aliases,genres=excluded.genres,metadata=excluded.metadata,updated_at=CURRENT_TIMESTAMP`, artist.ID, artist.Name, artist.SortName, string(aliasJSON), string(genres), string(data))
}
func (c *MusicBrainzClient) ingestReleaseGroup(group MBReleaseGroup) {
	if c.db == nil || group.ID == "" || group.Title == "" || len(group.ArtistCredit) == 0 {
		return
	}
	var oldData string
	if c.db.QueryRow(`SELECT metadata FROM music_catalog_releases WHERE release_group_id=?`, group.ID).Scan(&oldData) == nil {
		var old MBReleaseGroup
		if json.Unmarshal([]byte(oldData), &old) == nil {
			if len(group.Genres) == 0 {
				group.Genres = old.Genres
			}
			if len(group.Tags) == 0 {
				group.Tags = old.Tags
			}
			if group.FirstRelease == "" {
				group.FirstRelease = old.FirstRelease
			}
			if group.PrimaryType == "" {
				group.PrimaryType = old.PrimaryType
			}
		}
	}
	artist := group.ArtistCredit[0].Artist
	if artist.ID == "" || artist.Name == "" {
		return
	}
	for _, credit := range group.ArtistCredit {
		c.ingestArtist(credit.Artist, false)
	}
	year := 0
	if len(group.FirstRelease) >= 4 {
		year, _ = strconv.Atoi(group.FirstRelease[:4])
	}
	genres, _ := json.Marshal(GenreNames(group.Genres, group.Tags))
	data, _ := json.Marshal(group)
	c.db.Exec(`INSERT INTO music_catalog_releases(release_group_id,title,artist,artist_mbid,year,release_type,genres,metadata) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(release_group_id) DO UPDATE SET title=excluded.title,artist=excluded.artist,artist_mbid=excluded.artist_mbid,year=excluded.year,release_type=excluded.release_type,genres=excluded.genres,metadata=excluded.metadata,updated_at=CURRENT_TIMESTAMP`, group.ID, group.Title, artist.Name, artist.ID, year, group.PrimaryType, string(genres), string(data))
}
func (c *MusicBrainzClient) CatalogReleaseGroups(limit int) []MBReleaseGroup {
	return c.SearchCatalogReleaseGroups("", limit)
}
func (c *MusicBrainzClient) SearchCatalogReleaseGroups(query string, limit int) []MBReleaseGroup {
	out := []MBReleaseGroup{}
	if c.db == nil {
		return out
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := c.db.Query(`SELECT r.metadata,r.title,r.artist,COALESCE(a.aliases,'[]'),COALESCE(a.sort_name,'') FROM music_catalog_releases r LEFT JOIN music_catalog_artists a ON a.mbid=r.artist_mbid ORDER BY r.year DESC,r.title COLLATE NOCASE,r.release_group_id LIMIT 10000`)
	if err != nil {
		return out
	}
	defer rows.Close()
	words := strings.Fields(NormalizeMusicText(query))
	for rows.Next() {
		var data, title, artist, aliases, sortName string
		if rows.Scan(&data, &title, &artist, &aliases, &sortName) != nil {
			continue
		}
		text := NormalizeMusicText(title + " " + artist + " " + aliases + " " + sortName)
		matched := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		var group MBReleaseGroup
		if json.Unmarshal([]byte(data), &group) == nil {
			out = append(out, group)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
func (c *MusicBrainzClient) SearchCatalogArtists(query string, limit int) []MBArtist {
	out := []MBArtist{}
	if c.db == nil {
		return out
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := c.db.Query(`SELECT metadata,name,sort_name,aliases FROM music_catalog_artists ORDER BY name COLLATE NOCASE,mbid LIMIT 10000`)
	if err != nil {
		return out
	}
	defer rows.Close()
	words := strings.Fields(NormalizeMusicText(query))
	for rows.Next() {
		var data, name, sortName, aliases string
		if rows.Scan(&data, &name, &sortName, &aliases) != nil {
			continue
		}
		text := NormalizeMusicText(name + " " + sortName + " " + aliases)
		matched := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				matched = false
				break
			}
		}
		if matched {
			var artist MBArtist
			if json.Unmarshal([]byte(data), &artist) == nil {
				out = append(out, artist)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}
func mergeReleaseGroups(primary, secondary []MBReleaseGroup) []MBReleaseGroup {
	out := []MBReleaseGroup{}
	seen := map[string]bool{}
	for _, set := range [][]MBReleaseGroup{primary, secondary} {
		for _, group := range set {
			if group.ID != "" && !seen[group.ID] {
				seen[group.ID] = true
				out = append(out, group)
			}
		}
	}
	return out
}
