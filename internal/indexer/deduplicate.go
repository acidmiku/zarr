package indexer

import (
	"fmt"
	"strings"
)

func DeduplicateReleases(releases []Release) []Release {
	result := make([]Release, 0, len(releases))
	seenIDs, seenTitles := map[string]bool{}, map[string]bool{}
	for _, release := range releases {
		source := strings.ToLower(strings.TrimSpace(release.Indexer)) + ":" + release.DownloadType
		id := ""
		if release.TopicID > 0 {
			id = fmt.Sprintf("%s:topic:%d", source, release.TopicID)
		} else if release.NZBURL != "" {
			id = source + ":" + release.NZBURL
		}
		title := source + ":" + strings.ToLower(strings.TrimSpace(release.Title))
		if seenTitles[title] || (id != "" && seenIDs[id]) {
			continue
		}
		seenTitles[title] = true
		if id != "" {
			seenIDs[id] = true
		}
		result = append(result, release)
	}
	return result
}
