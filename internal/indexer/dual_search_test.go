package indexer

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnimeSearchCombinesAbsoluteAndSeasonQueries(t *testing.T) {
	for _, absoluteFails := range []bool{false, true} {
		t.Run(fmt.Sprint(absoluteFails), func(t *testing.T) {
			queries := map[string]int{}
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query().Get("q")
				queries[q]++
				if q == "Example 13" && absoluteFails {
					w.WriteHeader(503)
					return
				}
				io.WriteString(w, `<rss><channel><item><title>[SubsPlease] Example - 13 [1080p WEB-DL]</title><link>https://test/nzb/1</link></item>`)
				if q == "Example S02E01" {
					io.WriteString(w, `<item><title>Example.S02E01.1080p.WEB-DL</title><link>https://test/nzb/2</link></item><item><title>Example.The.Movie.2025.1080p</title><link>https://test/nzb/3</link></item>`)
				}
				io.WriteString(w, `</channel></rss>`)
			}))
			defer remote.Close()
			releases := NewNewznabClient(remote.Client()).SearchAnimeEpisode([]IndexerConfig{{Name: "test", URL: remote.URL, Enabled: true}}, []string{"Example", "Example"}, 13, 2, 1)
			if queries["Example 13"] != 1 || queries["Example S02E01"] != 1 || len(releases) != 2 {
				t.Fatalf("queries=%v releases=%+v", queries, releases)
			}
		})
	}
}

func TestDeduplicateKeepsAlternativeSources(t *testing.T) {
	releases := DeduplicateReleases([]Release{
		{Title: "Example 01", TopicID: 1, Indexer: "A", DownloadType: "torrent"},
		{Title: "Example S01E01", TopicID: 1, Indexer: "A", DownloadType: "torrent"},
		{Title: "Example 01", TopicID: 2, Indexer: "A", DownloadType: "torrent"},
		{Title: "Example 01", TopicID: 1, Indexer: "B", DownloadType: "torrent"},
		{Title: "Example 01", NZBURL: "https://test/nzb/1", Indexer: "A", DownloadType: "nzb"},
	})
	if len(releases) != 3 {
		t.Fatalf("deduplication merged independent sources: %+v", releases)
	}
}
