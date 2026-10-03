package scheduler

import (
	"database/sql"
	"fmt"
	"mediaforge/internal/indexer"
	"mediaforge/internal/rutracker"
)

func (s *Scheduler) searchTorrentReleases(sources []indexer.IndexerConfig, queries []string, contentType string) []indexer.Release {
	if s.rtClient == nil {
		return nil
	}
	var releases []indexer.Release
	for _, source := range sources {
		for _, query := range queries {
			found, err := s.rtClient.Search(query, rutracker.DefaultForumIDs[contentType], source.Username, source.Password)
			if err != nil {
				continue
			}
			for _, result := range found {
				parsed := indexer.ParseReleaseName(result.Title)
				releases = append(releases, indexer.Release{Title: result.Title, Size: result.Size, Quality: parsed.Quality, Tags: parsed.Tags, Indexer: source.Name, DownloadType: "torrent", Seeders: result.Seeders, Leechers: result.Leechers, TopicID: result.TopicID})
			}
		}
	}
	return indexer.DeduplicateReleases(releases)
}
func (s *Scheduler) findTorrentEpisodes(sources []indexer.IndexerConfig, title, contentType string, season, episode int, absolute sql.NullInt64) []indexer.Release {
	queries := []string{fmt.Sprintf("%s S%02dE%02d", title, season, episode)}
	number := 0
	if contentType == "anime" && absolute.Valid {
		number = int(absolute.Int64)
		queries = append(queries, fmt.Sprintf("%s %d", title, number))
	}
	return indexer.FilterEpisodeReleases(s.searchTorrentReleases(sources, queries, contentType), title, season, episode, number)
}
