package metadata

import "testing"

func TestAnimeClassificationUsesGenreAndOriginWithLanguageFallback(t *testing.T) {
	for _, test := range []struct {
		name      string
		genres    []int
		countries []string
		language  string
		want      bool
	}{
		{"Japanese TV animation", []int{28, 16}, []string{"JP"}, "ja", true},
		{"movie search omits country", []int{16}, nil, "ja", true},
		{"Japanese coproduction", []int{16}, []string{"US", "jp"}, "en", true},
		{"explicit non Japanese origin wins", []int{16}, []string{"US"}, "ja", false},
		{"American animation", []int{16}, nil, "en", false},
		{"Japanese live action", []int{28}, []string{"JP"}, "ja", false},
		{"unknown origin", []int{16}, nil, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := TMDBMediaEntry{GenreIDs: test.genres, OriginCountry: test.countries, OriginalLanguage: test.language}
			if got := IsAnimeEntry(entry); got != test.want {
				t.Fatalf("anime=%t want=%t", got, test.want)
			}
		})
	}
	c := &TMDBClient{}
	if !c.IsAnimeMovie(&TMDBMovieDetail{Genres: []TMDBGenre{{ID: 16, Name: "Localized name"}}, OriginalLanguage: "ja"}) {
		t.Fatal("movie genre ID was ignored")
	}
	if !c.IsAnime(&TMDBTVDetail{Genres: []TMDBGenre{{ID: 16, Name: "Localized name"}}, OriginCountry: []string{"JP"}}) {
		t.Fatal("TV genre ID was ignored")
	}
}
