package indexer

import "testing"

func TestAnimeMovieIncludesAnimeOnlySources(t *testing.T) {
	sources := []IndexerConfig{
		{ID: 1, Enabled: true, Type: "newznab", ContentTypes: []string{"anime"}},
		{ID: 2, Enabled: true, Type: "newznab", ContentTypes: []string{"movie"}},
		{ID: 3, Enabled: true, Type: "newznab", ContentTypes: []string{"movie", "anime"}},
		{ID: 4, Enabled: true, Type: "newznab", ContentTypes: []string{"series"}},
		{ID: 5, Enabled: true, Type: "rutracker", ContentTypes: []string{"anime"}},
		{ID: 6, Enabled: false, Type: "newznab", ContentTypes: []string{"anime"}},
	}
	got := MovieIndexers(sources, "newznab", true)
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Fatalf("anime movie sources: %+v", got)
	}
	got = MovieIndexers(sources, "newznab", false)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("ordinary movie sources: %+v", got)
	}
	got = MovieIndexers(sources, "rutracker", true)
	if len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("anime torrent movie sources: %+v", got)
	}
}
