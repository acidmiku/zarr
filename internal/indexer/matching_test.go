package indexer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChainsawManSeriesRejectsRezeArcMovieAudioNumbers(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Chainsaw.Man.The.Movie.Reze.Arc.2025.1080p.BluRay.DDP5.1", false},
		{"Chainsaw.Man.Reze.Arc.2025.1080p.AAC.2.0", false},
		{"Chainsaw.Man.The.Movie.Reze.Arc.S01E01.1080p", false},
		{"[SubsPlease] Chainsaw Man - 01 (1080p) [ABCD1234]", true},
		{"Chainsaw.Man.01.1080p.WEB-DL", true},
		{"Chainsaw.Man.S01E01.1080p.WEB-DL", true},
		{"Chainsaw Man (2022) - S01E01 - Dog & Chainsaw.mkv", true},
		{"Chainsaw Man - 10 (1080p)", false},
		{"Chainsaw Man - 01-12 [Complete]", false},
		{"Chainsaw Man - 01v2-03 [1080p]", false},
		{"Chainsaw Man S01E01E02 1080p", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesSeriesEpisode(tt.name, "Chainsaw Man", 1, 1, 1); got != tt.want {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
	if MatchesAbsoluteEpisode("Chainsaw.Man.Reze.Arc.2025.DDP5.1", "Chainsaw Man", 1) {
		t.Fatal("audio channel parsed as episode")
	}
}
func TestAbsoluteEpisodeMatchingSupportsUnicodeAndRejectsAudio(t *testing.T) {
	if !MatchesAbsoluteEpisode("[Group] チェンソーマン - 01 [1080p]", "チェンソーマン", 1) {
		t.Fatal("unicode rejected")
	}
	if _, ok := AbsoluteEpisodeNumber("チェンソーマン", "チェンソーマン"); ok {
		t.Fatal("title alone matched episode")
	}
	if !MatchesAbsoluteEpisode("[Group] 86 - 01 [1080p]", "86", 1) {
		t.Fatal("numeric title rejected")
	}
	if MatchesAbsoluteEpisode("Chainsaw Man 5.1 Audio", "Chainsaw Man", 5) {
		t.Fatal("audio matched episode")
	}
}
func TestTypedSearchesSeparateMovieAndAnimeSeries(t *testing.T) {
	const movie = "Chainsaw.Man.The.Movie.Reze.Arc.2025.1080p.WEB-DL.DDP5.1"
	const episode = "Chainsaw.Man.S01E01.2022.1080p.WEB-DL"
	const absolute = "[Group] Chainsaw Man - 01 [1080p]"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<rss><channel><item><title>%s</title><link>http://test/movie</link></item><item><title>%s</title><link>http://test/episode</link></item><item><title>%s</title><link>http://test/absolute</link></item></channel></rss>`, movie, episode, absolute)
	}))
	defer srv.Close()
	c := NewNewznabClient(srv.Client())
	idx := []IndexerConfig{{Name: "test", URL: srv.URL, Enabled: true}}
	episodes := c.SearchAnimeEpisode(idx, []string{"Chainsaw Man"}, 1, 1, 1)
	if len(episodes) != 2 {
		t.Fatalf("anime search = %+v", episodes)
	}
	for _, r := range episodes {
		if r.Title == movie {
			t.Fatal("movie leaked into series search")
		}
	}
	movies := c.SearchMovie(idx, "", "Chainsaw Man The Movie Reze Arc", 2025)
	if len(movies) != 1 || movies[0].Title != movie {
		t.Fatalf("movie search = %+v", movies)
	}
	movies = c.SearchMovie(idx, "tt0001", "Localized movie title", 2025)
	for _, r := range movies {
		if r.Title == episode {
			t.Fatal("episodic result accepted as movie")
		}
	}
}
func TestTextEpisodeSearchRejectsDifferentSeries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel><item><title>Unrelated.Show.S01E01.1080p</title><link>http://test/wrong</link></item><item><title>Chainsaw.Man.S01E01.1080p</title><link>http://test/right</link></item></channel></rss>`)
	}))
	defer srv.Close()
	got := NewNewznabClient(srv.Client()).SearchEpisode([]IndexerConfig{{URL: srv.URL, Enabled: true}}, 0, 1, 1, "Chainsaw Man")
	if len(got) != 1 || got[0].NZBURL != "http://test/right" {
		t.Fatalf("wrong series matched: %+v", got)
	}
}
