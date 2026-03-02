package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"

	"golang.org/x/time/rate"
)

const mbBase = "https://musicbrainz.org/ws/2"
const mbUserAgent = "Zarr/1.0 (https://github.com/zarr)"

// MusicBrainzClient queries the MusicBrainz API.
type MusicBrainzClient struct {
	client  *http.Client
	limiter *rate.Limiter
}

func NewMusicBrainzClient(client *http.Client) *MusicBrainzClient {
	return &MusicBrainzClient{
		client:  client,
		limiter: rate.NewLimiter(1, 1), // 1 request per second
	}
}

// -- Response types --

type MBArtistSearch struct {
	Artists []MBArtist `json:"artists"`
	Count   int        `json:"count"`
}

type MBArtist struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SortName string `json:"sort-name"`
	Type     string `json:"type"`
	Country  string `json:"country"`
	Score    int    `json:"score"`
	// life-span
	LifeSpan struct {
		Begin string `json:"begin"`
		End   string `json:"end"`
		Ended bool   `json:"ended"`
	} `json:"life-span"`
}

type MBReleaseGroupSearch struct {
	ReleaseGroups []MBReleaseGroup `json:"release-groups"`
	Count         int              `json:"count"`
}

type MBReleaseGroup struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	PrimaryType    string     `json:"primary-type"`
	SecondaryTypes []string   `json:"secondary-types"`
	FirstRelease   string     `json:"first-release-date"`
	Score          int        `json:"score"`
	ArtistCredit   []MBCredit `json:"artist-credit"`
	Releases       []MBRelease `json:"releases"`
}

type MBCredit struct {
	Artist MBArtist `json:"artist"`
}

type MBRelease struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Date        string    `json:"date"`
	Country     string    `json:"country"`
	TrackCount  int       `json:"track-count"`
	Media       []MBMedia `json:"media"`
	ReleaseGroup *MBReleaseGroup `json:"release-group"`
}

type MBMedia struct {
	Position   int       `json:"position"`
	Format     string    `json:"format"`
	TrackCount int       `json:"track-count"`
	Tracks     []MBTrack `json:"tracks"`
}

type MBTrack struct {
	ID       string      `json:"id"`
	Number   string      `json:"number"`
	Title    string      `json:"title"`
	Position int         `json:"position"`
	Length   int         `json:"length"` // milliseconds
	Recording MBRecording `json:"recording"`
}

type MBRecording struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Length        int              `json:"length"`
	Score         int              `json:"score"`
	ArtistCredit  []MBCredit       `json:"artist-credit"`
	Releases      []MBRelease      `json:"releases"`
}

type MBRecordingSearch struct {
	Recordings []MBRecording `json:"recordings"`
	Count      int           `json:"count"`
}

// -- Methods --

func (c *MusicBrainzClient) SearchArtists(query string) (*MBArtistSearch, error) {
	u := fmt.Sprintf("%s/artist?query=%s&fmt=json&limit=25",
		mbBase, url.QueryEscape(query))
	return mbGet[MBArtistSearch](c, u)
}

func (c *MusicBrainzClient) SearchReleaseGroups(query string) (*MBReleaseGroupSearch, error) {
	u := fmt.Sprintf("%s/release-group?query=%s&fmt=json&limit=25",
		mbBase, url.QueryEscape(query))
	return mbGet[MBReleaseGroupSearch](c, u)
}

func (c *MusicBrainzClient) SearchRecordings(query string) (*MBRecordingSearch, error) {
	u := fmt.Sprintf("%s/recording?query=%s&fmt=json&limit=25",
		mbBase, url.QueryEscape(query))
	return mbGet[MBRecordingSearch](c, u)
}

func (c *MusicBrainzClient) SearchArtistReleaseGroups(artistMBID string) (*MBReleaseGroupSearch, error) {
	u := fmt.Sprintf("%s/release-group?artist=%s&type=album&fmt=json&limit=100",
		mbBase, url.QueryEscape(artistMBID))
	return mbGet[MBReleaseGroupSearch](c, u)
}

func (c *MusicBrainzClient) GetReleasesForGroup(rgID string) ([]MBRelease, error) {
	u := fmt.Sprintf("%s/release?release-group=%s&inc=media+recordings&fmt=json&limit=100",
		mbBase, url.QueryEscape(rgID))
	type relResp struct {
		Releases []MBRelease `json:"releases"`
	}
	resp, err := mbGet[relResp](c, u)
	if err != nil {
		return nil, err
	}
	return resp.Releases, nil
}

// GetBestRelease picks the "Official" CD/Digital release with the most tracks.
func (c *MusicBrainzClient) GetBestRelease(rgID string) (*MBRelease, error) {
	releases, err := c.GetReleasesForGroup(rgID)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases for group %s", rgID)
	}

	// Prefer Official status, then highest track count
	sort.Slice(releases, func(i, j int) bool {
		iOff := releases[i].Status == "Official"
		jOff := releases[j].Status == "Official"
		if iOff != jOff {
			return iOff
		}
		return totalTracks(releases[i]) > totalTracks(releases[j])
	})

	return &releases[0], nil
}

func (c *MusicBrainzClient) GetArtist(mbid string) (*MBArtist, error) {
	u := fmt.Sprintf("%s/artist/%s?fmt=json", mbBase, url.QueryEscape(mbid))
	return mbGet[MBArtist](c, u)
}

func (c *MusicBrainzClient) GetReleaseGroup(rgID string) (*MBReleaseGroup, error) {
	u := fmt.Sprintf("%s/release-group/%s?inc=artist-credits&fmt=json", mbBase, url.QueryEscape(rgID))
	return mbGet[MBReleaseGroup](c, u)
}

func totalTracks(r MBRelease) int {
	n := 0
	for _, m := range r.Media {
		n += m.TrackCount
	}
	return n
}

func mbGet[T any](c *MusicBrainzClient, rawURL string) (*T, error) {
	// Rate limit
	c.limiter.Wait(context.Background())

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz request: %w", err)
	}
	req.Header.Set("User-Agent", mbUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("musicbrainz %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("musicbrainz decode: %w", err)
	}
	return &result, nil
}
