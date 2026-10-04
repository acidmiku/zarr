package indexer

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMusicIdentityBeforeQuality(t *testing.T) {
	identity := MusicIdentity{Artist: "Björk", Album: "Debut", AlbumType: "album"}
	profile := &QualityProfile{ProfileType: "music", Qualities: `["flac-24bit","flac"]`, Language: "any"}
	for _, tt := range []struct {
		title string
		want  bool
	}{
		{"Björk - Debut (1993) FLAC", true},
		{"Björk-Debut-1993-FLAC-GROUP", true},
		{"Björk 1993 Debut [FLAC]", true},
		{"Björk - Post (1995) 24bit FLAC", false},
		{"Other Björk - Debut FLAC", false},
		{"Björk - Debut Deluxe Edition FLAC", false},
		{"Björk - Debut FLAC-Deluxe", false},
		{"Björk - Debut Live FLAC", false},
		{"Björk - Debut Remixes FLAC", false},
		{"Björk - Debut - Human Behaviour FLAC", false},
		{"Björk - Debut Single FLAC", false},
		{"Björk - Debut Discography 1993-2020 FLAC", false},
		{"Björk - Debut", false},
		{"Björk - Debut 24bit", false},
		{"Björk - Debut M4A", false},
	} {
		t.Run(tt.title, func(t *testing.T) {
			release := Release{Title: tt.title}
			ScoreMusicRelease(&release, profile, identity)
			if release.Acceptable != tt.want {
				t.Fatalf("acceptable=%v reason=%s quality=%s", release.Acceptable, release.RejectReason, release.Quality)
			}
		})
	}
}

func TestMusicAliasesAreExplicitAndEditionCannotBeBypassed(t *testing.T) {
	identity := MusicIdentity{Artist: "宇多田ヒカル", ArtistAliases: []string{"Hikaru Utada"}, Album: "First Love", Edition: "deluxe edition"}
	for _, tt := range []struct {
		title string
		want  bool
	}{
		{"Hikaru Utada - First Love Deluxe Edition FLAC", true},
		{"宇多田ヒカル First Love Deluxe FLAC", true},
		{"Utada - First Love Deluxe FLAC", false},
		{"Hikaru Utada - First Love FLAC", false},
		{"Hikaru Utada - First Love Deluxe Remix FLAC", false},
	} {
		if got := MusicReleaseRejection(tt.title, identity) == ""; got != tt.want {
			t.Errorf("%s: matched=%v", tt.title, got)
		}
	}
	if !MatchesMusicName("ＲＥＭ – What's New?", "rem whats new") || MatchesMusicName("Live Song", "Song") || MatchesMusicName("Björk", "Bjork") {
		t.Fatal("music normalization lost meaningful identity")
	}
}

func TestMusicFallbackDropsYearHonorsCodecAndKeepsErrors(t *testing.T) {
	queries := []string{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		queries = append(queries, r.URL.Query().Get("t")+":"+query)
		if strings.Contains(query, "2024") {
			w.WriteHeader(503)
			return
		}
		if r.URL.Query().Get("t") == "music" {
			io.WriteString(w, `<rss><channel><item><title>Artist - Different Album FLAC</title><link>https://test/wrong</link></item></channel></rss>`)
			return
		}
		fmt.Fprint(w, `<rss><channel><item><title>Artist - Album MP3 320kbps</title><link>https://test/right</link></item></channel></rss>`)
	}))
	defer remote.Close()
	profile := &QualityProfile{ProfileType: "music", Qualities: `["mp3-320"]`, Language: "any"}
	result := NewNewznabClient(remote.Client()).SearchMusicAlbum([]IndexerConfig{{Name: "test", URL: remote.URL, Enabled: true, APIKey: "never-echo-me"}}, MusicIdentity{Artist: "Artist", Album: "Album"}, 2024, profile)
	if len(queries) != 3 || queries[0] != "music:Artist Album 2024" || queries[1] != "music:Artist Album" || queries[2] != "search:Artist Album MP3" {
		t.Fatal(queries)
	}
	if len(result.Errors) != 1 || strings.Contains(fmt.Sprint(result.Errors), "never-echo-me") {
		t.Fatal(result.Errors)
	}
	best := BestRelease(result.Releases)
	if best == nil || best.Title != "Artist - Album MP3 320kbps" {
		t.Fatalf("wrong album/format won: %+v", best)
	}
}

func TestUnknownCodecCannotBypassStrictLossless(t *testing.T) {
	p := &QualityProfile{ProfileType: "music", Qualities: `["flac"]`, Language: "any"}
	for _, name := range []string{"Artist Album", "Artist Album 24bit", "Artist Album 320kbps", "Artist Album M4A", "Artist Album VBR"} {
		parsed := ParseMusicReleaseName(name)
		r := Release{Title: name, Quality: parsed.Quality}
		ScoreRelease(&r, p)
		if r.Acceptable {
			t.Fatalf("unknown codec accepted: %+v", r)
		}
	}
}
