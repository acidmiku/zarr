package indexer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewznabNormalizesEndpointAndIMDB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" || r.URL.Query().Get("imdbid") != "0123456" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		io.WriteString(w, `<rss><channel><item><title>Movie.1080p</title><link>https://example.test/details</link><enclosure url="https://example.test/download" length="10"/></item></channel></rss>`)
	}))
	defer srv.Close()
	items, err := NewNewznabClient(srv.Client()).SearchMovieByIMDB(IndexerConfig{URL: srv.URL, Name: "test"}, "tt0123456")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Link != "https://example.test/download" {
		t.Fatalf("wrong download link: %+v", items)
	}
	parsed, _ := url.Parse(buildURL("https://example.test/custom/api?existing=yes", map[string]string{"apikey": "a&b"}))
	if parsed.Path != "/custom/api" || parsed.Query().Get("existing") != "yes" || parsed.Query().Get("apikey") != "a&b" {
		t.Fatal(parsed)
	}
}
func TestNewznabConnectionRequiresCaps(t *testing.T) {
	for _, body := range []string{`<error code="100" description="wrong API key"/>`, `<html>login</html>`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
		if err := NewNewznabClient(srv.Client()).TestConnection(IndexerConfig{URL: srv.URL, Name: "test"}); err == nil {
			t.Errorf("accepted %s", body)
		}
		srv.Close()
	}
}
func TestEpisodeMatchingDoesNotConfuseNumbers(t *testing.T) {
	if MatchesEpisode("Show.S01E100.1080p", 1, 10) {
		t.Error("episode 100 matched 10")
	}
	if !MatchesEpisode("Show.S01E10.1080p", 1, 10) {
		t.Error("correct episode missing")
	}
	if MatchesAbsoluteEpisode("[Group] Show - 10 [1080p]", "Show", 1) {
		t.Error("anime episode 10 matched 1")
	}
	if !MatchesAbsoluteEpisode("[Group] Show - 001v2 [1080p]", "Show", 1) {
		t.Error("anime versioned episode missing")
	}
}
