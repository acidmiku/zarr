package metadata

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Mapping handles AniDB <-> TMDB ID cross-referencing.
type Mapping struct {
	client  *http.Client
	mu      sync.RWMutex
	entries []MappingEntry
	lastRefresh time.Time
}

type MappingEntry struct {
	AniDBID    int
	TMDBID     int
	TVDBId     int
	IMDbID     string
	Type       string // "movie" or "tv"
	DefaultSeason int
	EpisodeOffset int
}

// anime-list XML structure
type animeList struct {
	XMLName xml.Name    `xml:"anime-list"`
	Anime   []animeItem `xml:"anime"`
}

type animeItem struct {
	AniDBID       string `xml:"anidbid,attr"`
	TVDBId        string `xml:"tvdbid,attr"`
	DefaultSeason string `xml:"defaulttvdbseason,attr"`
	EpisodeOffset string `xml:"episodeoffset,attr"`
	IMDbID        string `xml:"imdbid,attr"`
	TMDbID        string `xml:"tmdbid,attr"`
	Type          string `xml:"type,attr"`
}

const mappingURL = "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-list.xml"

func NewMapping(client *http.Client) *Mapping {
	return &Mapping{client: client}
}

// Refresh downloads the mapping data.
func (m *Mapping) Refresh() error {
	slog.Info("refreshing anime ID mapping")

	data, err := m.fetchMapping()
	if err != nil {
		return err
	}

	var list animeList
	if err := xml.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse mapping: %w", err)
	}

	entries := make([]MappingEntry, 0, len(list.Anime))
	for _, a := range list.Anime {
		anidbID, _ := strconv.Atoi(a.AniDBID)
		if anidbID == 0 {
			continue
		}

		entry := MappingEntry{
			AniDBID: anidbID,
		}

		if a.TMDbID != "" && a.TMDbID != "unknown" {
			entry.TMDBID, _ = strconv.Atoi(a.TMDbID)
		}
		if a.TVDBId != "" && a.TVDBId != "unknown" {
			entry.TVDBId, _ = strconv.Atoi(a.TVDBId)
		}
		entry.IMDbID = a.IMDbID
		entry.Type = a.Type
		entry.DefaultSeason, _ = strconv.Atoi(a.DefaultSeason)
		entry.EpisodeOffset, _ = strconv.Atoi(a.EpisodeOffset)

		entries = append(entries, entry)
	}

	m.mu.Lock()
	m.entries = entries
	m.lastRefresh = time.Now()
	m.mu.Unlock()

	slog.Info("anime mapping refreshed", "entries", len(entries))
	return nil
}

// FindByAniDBID looks up a mapping by AniDB ID.
func (m *Mapping) FindByAniDBID(anidbID int) *MappingEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.entries {
		if e.AniDBID == anidbID {
			return &e
		}
	}
	return nil
}

// FindByTMDBID looks up a mapping by TMDB ID.
func (m *Mapping) FindByTMDBID(tmdbID int) []MappingEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []MappingEntry
	for _, e := range m.entries {
		if e.TMDBID == tmdbID {
			results = append(results, e)
		}
	}
	return results
}

// FindByTVDBID looks up a mapping by TVDB ID.
func (m *Mapping) FindByTVDBID(tvdbID int) []MappingEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []MappingEntry
	for _, e := range m.entries {
		if e.TVDBId == tvdbID {
			results = append(results, e)
		}
	}
	return results
}

// fetchMapping GETs the mapping URL. Transport-level retry handles transient errors.
func (m *Mapping) fetchMapping() ([]byte, error) {
	resp, err := m.client.Get(mappingURL)
	if err != nil {
		return nil, fmt.Errorf("fetch mapping: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mapping HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	return body, nil
}

// NeedsRefresh returns true if the mapping data is stale.
func (m *Mapping) NeedsRefresh() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.lastRefresh) > 24*time.Hour
}
