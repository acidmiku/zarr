package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mediaforge/internal/database"

	"golang.org/x/time/rate"
)

const mbBase = "https://musicbrainz.org/ws/2"
const mbUserAgent = "Zarr/1.0 (https://github.com/acidmiku/zarr)"

// MusicBrainzClient queries the MusicBrainz API.
type MusicBrainzClient struct {
	client  *http.Client
	limiter *rate.Limiter
	cache   *musicMetadataCache
	db      *database.DB
}

func NewMusicBrainzClient(client *http.Client, databases ...*database.DB) *MusicBrainzClient {
	var db *database.DB
	if len(databases) > 0 {
		db = databases[0]
	}
	c := &MusicBrainzClient{
		client:  client,
		limiter: rate.NewLimiter(1, 1), // 1 request per second
		db:      db,
	}
	c.cache = newMusicMetadataCache(client, db, c.limiter)
	return c
}

// -- Response types --

type MBArtistSearch struct {
	Artists []MBArtist `json:"artists"`
	Count   int        `json:"count"`
}

type MBArtist struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	SortName       string    `json:"sort-name"`
	Type           string    `json:"type"`
	Country        string    `json:"country"`
	Score          int       `json:"score"`
	Aliases        []MBAlias `json:"aliases,omitempty"`
	Genres         []MBTag   `json:"genres,omitempty"`
	Tags           []MBTag   `json:"tags,omitempty"`
	Disambiguation string    `json:"disambiguation,omitempty"`
	// life-span
	LifeSpan struct {
		Begin string `json:"begin"`
		End   string `json:"end"`
		Ended bool   `json:"ended"`
	} `json:"life-span"`
}

type MBAlias struct {
	Name     string `json:"name"`
	SortName string `json:"sort-name,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Ended    bool   `json:"ended,omitempty"`
}
type MBTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MBReleaseGroupSearch struct {
	ReleaseGroups     []MBReleaseGroup `json:"release-groups"`
	Count             int              `json:"count"`
	ReleaseGroupCount int              `json:"release-group-count,omitempty"`
	Complete          bool             `json:"complete"`
}

type MBReleaseGroup struct {
	ID             string      `json:"id"`
	Title          string      `json:"title"`
	PrimaryType    string      `json:"primary-type"`
	SecondaryTypes []string    `json:"secondary-types"`
	FirstRelease   string      `json:"first-release-date"`
	Score          int         `json:"score"`
	ArtistCredit   []MBCredit  `json:"artist-credit"`
	Releases       []MBRelease `json:"releases"`
	Genres         []MBTag     `json:"genres,omitempty"`
	Tags           []MBTag     `json:"tags,omitempty"`
	Disambiguation string      `json:"disambiguation,omitempty"`
}

type MBCredit struct {
	Artist     MBArtist `json:"artist"`
	Name       string   `json:"name,omitempty"`
	JoinPhrase string   `json:"joinphrase,omitempty"`
}

type MBRelease struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Date           string          `json:"date"`
	Country        string          `json:"country"`
	TrackCount     int             `json:"track-count"`
	Media          []MBMedia       `json:"media"`
	ReleaseGroup   *MBReleaseGroup `json:"release-group"`
	Disambiguation string          `json:"disambiguation,omitempty"`
	Packaging      string          `json:"packaging,omitempty"`
	Barcode        string          `json:"barcode,omitempty"`
	ArtistCredit   []MBCredit      `json:"artist-credit,omitempty"`
}

type MBMedia struct {
	Position   int       `json:"position"`
	Format     string    `json:"format"`
	TrackCount int       `json:"track-count"`
	Tracks     []MBTrack `json:"tracks"`
}

type MBTrack struct {
	ID        string      `json:"id"`
	Number    string      `json:"number"`
	Title     string      `json:"title"`
	Position  int         `json:"position"`
	Length    int         `json:"length"` // milliseconds
	Recording MBRecording `json:"recording"`
}

type MBRecording struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Length       int         `json:"length"`
	Score        int         `json:"score"`
	ArtistCredit []MBCredit  `json:"artist-credit"`
	Releases     []MBRelease `json:"releases"`
}

type MBRecordingSearch struct {
	Recordings []MBRecording `json:"recordings"`
	Count      int           `json:"count"`
}

// -- Methods --

func (c *MusicBrainzClient) SearchArtists(query string) (*MBArtistSearch, error) {
	u := fmt.Sprintf("%s/artist?query=%s&fmt=json&limit=25",
		mbBase, url.QueryEscape(query))
	result, err := mbGet[MBArtistSearch](c, u)
	if err != nil {
		local := c.SearchCatalogArtists(query, 25)
		if len(local) > 0 {
			return &MBArtistSearch{Artists: local, Count: len(local)}, nil
		}
		return nil, err
	}
	for _, artist := range result.Artists {
		c.ingestArtist(artist, false)
	}
	seen := map[string]bool{}
	for _, artist := range result.Artists {
		seen[artist.ID] = true
	}
	for _, artist := range c.SearchCatalogArtists(query, 25) {
		if !seen[artist.ID] {
			result.Artists = append(result.Artists, artist)
			seen[artist.ID] = true
		}
	}
	if result.Count < len(result.Artists) {
		result.Count = len(result.Artists)
	}
	return result, nil
}

func (c *MusicBrainzClient) SearchReleaseGroups(query string) (*MBReleaseGroupSearch, error) {
	return c.SearchReleaseGroupsContext(context.Background(), query)
}
func (c *MusicBrainzClient) SearchReleaseGroupsContext(ctx context.Context, query string) (*MBReleaseGroupSearch, error) {
	u := fmt.Sprintf("%s/release-group?query=%s&fmt=json&limit=25",
		mbBase, url.QueryEscape(query))
	result, err := mbGetContext[MBReleaseGroupSearch](ctx, c, u)
	if err != nil {
		local := c.SearchCatalogReleaseGroups(query, 50)
		if len(local) > 0 {
			return &MBReleaseGroupSearch{ReleaseGroups: local, Count: len(local)}, nil
		}
		return nil, err
	}
	for _, group := range result.ReleaseGroups {
		c.ingestReleaseGroup(group)
	}
	result.ReleaseGroups = mergeReleaseGroups(result.ReleaseGroups, c.SearchCatalogReleaseGroups(query, 50))
	if result.Count < len(result.ReleaseGroups) {
		result.Count = len(result.ReleaseGroups)
	}
	return result, nil
}

func (c *MusicBrainzClient) SearchRecordings(query string) (*MBRecordingSearch, error) {
	u := fmt.Sprintf("%s/recording?query=%s&fmt=json&limit=25",
		mbBase, url.QueryEscape(query))
	result, err := mbGet[MBRecordingSearch](c, u)
	if err == nil {
		for _, recording := range result.Recordings {
			for _, release := range recording.Releases {
				if release.ReleaseGroup != nil {
					group := *release.ReleaseGroup
					if len(group.ArtistCredit) == 0 {
						group.ArtistCredit = release.ArtistCredit
					}
					c.ingestReleaseGroup(group)
				}
			}
		}
	}
	return result, err
}

func (c *MusicBrainzClient) SearchArtistReleaseGroups(artistMBID string) (*MBReleaseGroupSearch, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return c.SearchArtistReleaseGroupsContext(ctx, artistMBID)
}

func (c *MusicBrainzClient) SearchArtistReleaseGroupsContext(ctx context.Context, artistMBID string) (*MBReleaseGroupSearch, error) {
	result := &MBReleaseGroupSearch{ReleaseGroups: []MBReleaseGroup{}, Complete: true}
	seen := map[string]bool{}
	for offset := 0; ; {
		u := fmt.Sprintf("%s/release-group?artist=%s&inc=artist-credits&fmt=json&limit=100&offset=%d", mbBase, url.QueryEscape(artistMBID), offset)
		page, err := mbGetContext[MBReleaseGroupSearch](ctx, c, u)
		if err != nil {
			return nil, err
		}
		added := 0
		for _, group := range page.ReleaseGroups {
			if group.ID == "" || seen[group.ID] {
				continue
			}
			seen[group.ID] = true
			added++
			if len(group.ArtistCredit) == 0 {
				if artist := c.cachedArtist(artistMBID); artist != nil {
					group.ArtistCredit = []MBCredit{{Artist: *artist}}
				}
			}
			c.ingestReleaseGroup(group)
			result.ReleaseGroups = append(result.ReleaseGroups, group)
		}
		offset += len(page.ReleaseGroups)
		if page.ReleaseGroupCount > 0 && (len(page.ReleaseGroups) == 0 && offset < page.ReleaseGroupCount || offset >= page.ReleaseGroupCount && len(result.ReleaseGroups) < page.ReleaseGroupCount) {
			return nil, fmt.Errorf("MusicBrainz returned an incomplete discography")
		}
		if len(page.ReleaseGroups) == 0 || (page.ReleaseGroupCount > 0 && offset >= page.ReleaseGroupCount) || (page.ReleaseGroupCount == 0 && len(page.ReleaseGroups) < 100) {
			break
		}
		if added == 0 {
			return nil, fmt.Errorf("MusicBrainz repeated a discography page")
		}
	}
	result.Count = len(result.ReleaseGroups)
	result.ReleaseGroupCount = result.Count
	sort.SliceStable(result.ReleaseGroups, func(i, j int) bool {
		a, b := result.ReleaseGroups[i], result.ReleaseGroups[j]
		if a.FirstRelease != b.FirstRelease {
			return a.FirstRelease > b.FirstRelease
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
	return result, nil
}

func (c *MusicBrainzClient) GetReleasesForGroup(rgID string) ([]MBRelease, error) {
	type relResp struct {
		Releases []MBRelease `json:"releases"`
		Count    int         `json:"release-count"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out := []MBRelease{}
	seen := map[string]bool{}
	for offset := 0; ; {
		u := fmt.Sprintf("%s/release?release-group=%s&inc=media+recordings+artist-credits&fmt=json&limit=100&offset=%d", mbBase, url.QueryEscape(rgID), offset)
		resp, err := mbGetContext[relResp](ctx, c, u)
		if err != nil {
			return nil, err
		}
		added := 0
		for _, release := range resp.Releases {
			if release.ID != "" && !seen[release.ID] {
				seen[release.ID] = true
				out = append(out, release)
				added++
			}
		}
		offset += len(resp.Releases)
		if resp.Count > 0 && (len(resp.Releases) == 0 && offset < resp.Count || offset >= resp.Count && len(out) < resp.Count) {
			return nil, fmt.Errorf("MusicBrainz returned an incomplete edition list")
		}
		if len(resp.Releases) == 0 || (resp.Count > 0 && offset >= resp.Count) || (resp.Count == 0 && len(resp.Releases) < 100) {
			break
		}
		if added == 0 {
			return nil, fmt.Errorf("MusicBrainz repeated an edition page")
		}
	}
	SortReleases(out)
	return out, nil
}

// GetBestRelease chooses a deterministic official standard digital/CD edition.
func (c *MusicBrainzClient) GetBestRelease(rgID string) (*MBRelease, error) {
	releases, err := c.GetReleasesForGroup(rgID)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases for group %s", rgID)
	}

	SortReleases(releases)
	best := releases[0]
	normalizeTracks(&best)
	if completeTracks(best) {
		return &best, nil
	}
	return c.GetReleaseForGroup(rgID, best.ID)
}

func (c *MusicBrainzClient) GetArtist(mbid string) (*MBArtist, error) {
	return c.GetArtistContext(context.Background(), mbid)
}
func (c *MusicBrainzClient) GetArtistContext(ctx context.Context, mbid string) (*MBArtist, error) {
	u := fmt.Sprintf("%s/artist/%s?inc=aliases+genres+tags&fmt=json", mbBase, url.QueryEscape(mbid))
	artist, err := mbGetContext[MBArtist](ctx, c, u)
	if err == nil {
		c.ingestArtist(*artist, true)
	}
	return artist, err
}

func (c *MusicBrainzClient) GetReleaseGroup(rgID string) (*MBReleaseGroup, error) {
	return c.GetReleaseGroupContext(context.Background(), rgID)
}
func (c *MusicBrainzClient) GetReleaseGroupContext(ctx context.Context, rgID string) (*MBReleaseGroup, error) {
	u := fmt.Sprintf("%s/release-group/%s?inc=artist-credits+genres+tags&fmt=json", mbBase, url.QueryEscape(rgID))
	group, err := mbGetContext[MBReleaseGroup](ctx, c, u)
	if err == nil {
		c.ingestReleaseGroup(*group)
	}
	return group, err
}

func (c *MusicBrainzClient) GetRelease(id string) (*MBRelease, error) {
	u := fmt.Sprintf("%s/release/%s?inc=recordings+release-groups+artist-credits&fmt=json", mbBase, url.QueryEscape(id))
	release, err := mbGet[MBRelease](c, u)
	if err != nil {
		return nil, err
	}
	normalizeTracks(release)
	return release, nil
}
func (c *MusicBrainzClient) GetReleaseForGroup(groupID, releaseID string) (*MBRelease, error) {
	release, err := c.GetRelease(releaseID)
	if err != nil {
		return nil, err
	}
	if release.ID != releaseID || release.ReleaseGroup == nil || release.ReleaseGroup.ID != groupID {
		return nil, fmt.Errorf("selected edition does not belong to this release group")
	}
	if !completeTracks(*release) {
		return nil, fmt.Errorf("selected edition has an incomplete tracklist")
	}
	return release, nil
}

func totalTracks(r MBRelease) int {
	n := 0
	for _, m := range r.Media {
		n += m.TrackCount
	}
	return n
}

func mbGet[T any](c *MusicBrainzClient, rawURL string) (*T, error) {
	return mbGetContext[T](context.Background(), c, rawURL)
}

func mbGetContext[T any](ctx context.Context, c *MusicBrainzClient, rawURL string) (*T, error) {
	ttl := 24 * time.Hour
	if !strings.Contains(rawURL, "query=") && !strings.Contains(rawURL, "offset=") {
		ttl = 7 * 24 * time.Hour
	}
	data, err := c.cache.get(ctx, rawURL, ttl)
	if err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid MusicBrainz response")
	}
	return &result, nil
}

func completeTracks(release MBRelease) bool {
	if len(release.Media) == 0 {
		return false
	}
	discs := map[int]bool{}
	for _, medium := range release.Media {
		if medium.Position < 1 || discs[medium.Position] || medium.TrackCount < 0 || len(medium.Tracks) == 0 || (medium.TrackCount > 0 && len(medium.Tracks) != medium.TrackCount) {
			return false
		}
		discs[medium.Position] = true
		positions := map[int]bool{}
		for _, track := range medium.Tracks {
			if strings.TrimSpace(track.Title) == "" || track.Position < 1 || track.Length < 0 || positions[track.Position] {
				return false
			}
			positions[track.Position] = true
		}
	}
	return true
}
func normalizeTracks(release *MBRelease) {
	for mi := range release.Media {
		m := &release.Media[mi]
		if m.Position == 0 {
			m.Position = mi + 1
		}
		for ti := range m.Tracks {
			track := &m.Tracks[ti]
			if track.Position == 0 {
				track.Position = ti + 1
			}
			if strings.TrimSpace(track.Title) == "" {
				track.Title = track.Recording.Title
			}
			if track.Length <= 0 {
				track.Length = track.Recording.Length
			}
		}
		if m.TrackCount == 0 {
			m.TrackCount = len(m.Tracks)
		}
	}
	release.TrackCount = totalTracks(*release)
}

func SortReleases(releases []MBRelease) {
	formatRank := func(r MBRelease) int {
		if len(r.Media) == 0 {
			return 3
		}
		rank := 0
		for _, m := range r.Media {
			switch strings.ToLower(m.Format) {
			case "digital media":
			case "cd":
				if rank < 1 {
					rank = 1
				}
			default:
				if rank < 2 {
					rank = 2
				}
			}
		}
		return rank
	}
	editionRank := func(r MBRelease) int {
		text := strings.ToLower(r.Title + " " + r.Disambiguation)
		for _, word := range []string{"deluxe", "expanded", "anniversary", "bonus", "special edition", "collector", "remaster", "box set"} {
			if strings.Contains(text, word) {
				return 1
			}
		}
		return 0
	}
	sort.SliceStable(releases, func(i, j int) bool {
		a, b := releases[i], releases[j]
		ao, bo := strings.EqualFold(a.Status, "Official"), strings.EqualFold(b.Status, "Official")
		if ao != bo {
			return ao
		}
		if x, y := editionRank(a), editionRank(b); x != y {
			return x < y
		}
		if x, y := formatRank(a), formatRank(b); x != y {
			return x < y
		}
		if a.Date != b.Date {
			if a.Date == "" {
				return false
			}
			if b.Date == "" {
				return true
			}
			return a.Date < b.Date
		}
		if a.Country != b.Country {
			return a.Country < b.Country
		}
		return a.ID < b.ID
	})
}
