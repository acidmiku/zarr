package indexer

// MovieIndexers includes anime sources for anime films while preserving movie
// search semantics (IMDb IDs and film titles, never episode searches).
func MovieIndexers(indexers []IndexerConfig, kind string, anime bool) []IndexerConfig {
	var result []IndexerConfig
	for _, idx := range indexers {
		idxKind := idx.Type
		if idxKind == "" {
			idxKind = "newznab"
		}
		if !idx.Enabled || idxKind != kind {
			continue
		}
		for _, content := range idx.ContentTypes {
			if content == "movie" || (anime && content == "anime") {
				result = append(result, idx)
				break
			}
		}
	}
	return result
}
