package indexer

import (
	"encoding/json"
	"fmt"
	"strings"
)

type MusicSearchError struct {
	Indexer string `json:"indexer"`
	Query   string `json:"query"`
	Error   string `json:"error"`
}

type MusicSearchResult struct {
	Releases []Release          `json:"releases"`
	Errors   []MusicSearchError `json:"errors"`
}

func musicSearchFormats(profile *QualityProfile) []string {
	if profile == nil {
		return []string{""}
	}
	var qualities []string
	if err := json.Unmarshal([]byte(profile.Qualities), &qualities); err != nil {
		return nil
	}
	var formats []string
	seen := map[string]bool{}
	for _, quality := range qualities {
		format := strings.ToUpper(strings.SplitN(quality, "-", 2)[0])
		if !seen[format] && (format == "FLAC" || format == "MP3" || format == "AAC" || format == "OGG" || format == "ALAC" || format == "OPUS") {
			formats = append(formats, format)
			seen[format] = true
		}
	}
	return formats
}

// SearchMusicAlbum progressively relaxes the year without weakening identity
// or the requested codec. Failures remain visible even if a fallback succeeds.
func (c *NewznabClient) SearchMusicAlbum(indexers []IndexerConfig, identity MusicIdentity, year int, profile *QualityProfile) MusicSearchResult {
	result := MusicSearchResult{Releases: []Release{}, Errors: []MusicSearchError{}}
	base := strings.TrimSpace(identity.Artist + " " + identity.Album)
	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}
		type query struct {
			text     string
			category bool
		}
		queries := []query{}
		if year > 0 {
			queries = append(queries, query{fmt.Sprintf("%s %d", base, year), true})
		}
		queries = append(queries, query{base, true})
		for _, format := range musicSearchFormats(profile) {
			queries = append(queries, query{strings.TrimSpace(base + " " + format), false})
		}
		for _, query := range queries {
			var items []NewznabItem
			var err error
			if query.category {
				items, err = c.SearchMusicByCategory(idx, query.text)
			} else {
				items, err = c.SearchMusicByText(idx, query.text)
			}
			if err != nil {
				// c.fetch sanitizes URLs; still return a controlled message rather
				// than forwarding arbitrary credential-bearing indexer responses.
				result.Errors = append(result.Errors, MusicSearchError{Indexer: idx.Name, Query: query.text, Error: "Indexer search failed; check the connection and indexer availability."})
				continue
			}
			matched := false
			for _, item := range items {
				parsed := ParseMusicReleaseName(item.Title)
				release := Release{Title: item.Title, NZBURL: item.Link, Size: item.Size, Quality: parsed.Quality, Tags: parsed.Tags, Indexer: idx.Name, Category: item.Category, DownloadType: "nzb"}
				if profile != nil {
					ScoreMusicRelease(&release, profile, identity)
				} else {
					release.RejectReason = MusicReleaseRejection(release.Title, identity)
					release.Acceptable = release.RejectReason == ""
				}
				matched = matched || release.Acceptable
				result.Releases = append(result.Releases, release)
			}
			if matched {
				break
			}
		}
	}
	result.Releases = DeduplicateReleases(result.Releases)
	return result
}
