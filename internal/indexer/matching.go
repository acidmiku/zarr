package indexer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var episodeToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,3})e(\d{1,4})(?:[^0-9]|$)`)
var alternateEpisodeToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,3})x(\d{1,4})(?:[^0-9]|$)`)
var combinedEpisodeToken = regexp.MustCompile(`(?i)(?:s\d+e\d+(?:e|[ _]*-[ _]*(?:s\d+)?e?)\d+|\d+x\d+[ _]*-[ _]*(?:\d+x)?\d+)`)
var titleSeparators = regexp.MustCompile(`[^\pL\pN]+`)
var leadingYear = regexp.MustCompile(`^\((?:19|20)\d{2}\)[ ._-]*`)
var namedEpisode = regexp.MustCompile(`(?i)^(?:episode|ep|e)[ ._-]*`)
var absoluteToken = regexp.MustCompile(`(?i)^((\d{1,4})(?:v\d+)?)(?:$|[ \[(_.-])`)
var releaseYear = regexp.MustCompile(`(?:^|[^0-9])(?:19|20)\d{2}(?:[^0-9]|$)`)

// EpisodeNumbers recognizes only explicit season/episode identifiers, never
// resolution, codec, audio-channel, or release-year numbers.
func EpisodeNumbers(name string) (season, episode int, ok bool) {
	m := episodeToken.FindStringSubmatch(name)
	if m == nil {
		m = alternateEpisodeToken.FindStringSubmatch(name)
	}
	if m == nil {
		return 0, 0, false
	}
	season, _ = strconv.Atoi(m[1])
	episode, _ = strconv.Atoi(m[2])
	return season, episode, true
}
func MatchesEpisode(name string, season, episode int) bool {
	s, e, ok := EpisodeNumbers(name)
	return ok && !IsMultiEpisodeRelease(name) && s == season && e == episode
}

// TitleTail keeps the original punctuation after the title. Flattening the
// entire release into tokens would turn audio "5.1" into an episode number.
func TitleTail(name, title string) (string, bool) {
	words := strings.Fields(titleSeparators.ReplaceAllString(strings.ToLower(title), " "))
	if len(words) == 0 {
		return "", false
	}
	for i := range words {
		words[i] = regexp.QuoteMeta(words[i])
	}
	pattern := regexp.MustCompile(`(?i)(?:^|[^\pL\pN])` + strings.Join(words, `[^\pL\pN]+`) + `($|[^\pL\pN])`)
	loc := pattern.FindStringSubmatchIndex(name)
	if loc == nil {
		return "", false
	}
	end := loc[2]

	tail := strings.TrimLeft(name[end:], " ._:-")
	tail = leadingYear.ReplaceAllString(tail, "")
	return tail, true
}

func AbsoluteEpisodeNumber(name, title string) (int, bool) {
	tail, ok := TitleTail(name, title)
	if !ok {
		return 0, false
	}
	tail = namedEpisode.ReplaceAllString(tail, "")
	m := absoluteToken.FindStringSubmatch(tail)
	if m == nil {
		return 0, false
	}
	// A release year cannot be used as an absolute episode number.
	n, _ := strconv.Atoi(m[2])
	if n >= 1900 && n <= 2099 {
		return 0, false
	}
	// Decimal audio tokens and episode ranges are not single episodes.
	suffix := tail[len(m[1]):]
	if regexp.MustCompile(`^[.-][0-9]{1,3}(?:[^0-9]|$)`).MatchString(suffix) {
		return 0, false
	}
	return n, n > 0
}
func MatchesAbsoluteEpisode(name, title string, number int) bool {
	n, ok := AbsoluteEpisodeNumber(name, title)
	return ok && n == number
}
func MatchesSeriesEpisode(name, title string, season, episode, absolute int) bool {
	tail, ok := TitleTail(name, title)
	if !ok {
		return false
	}
	if MatchesEpisode(tail, season, episode) {
		// Require the identifier immediately after the series title/year, rather
		// than inside the title of a different film or spin-off.
		return regexp.MustCompile(`(?i)^(?:s\d{1,3}e\d{1,4}|\d{1,3}x\d{1,4})`).MatchString(tail)
	}
	return absolute > 0 && MatchesAbsoluteEpisode(name, title, absolute)
}
func IsEpisodicRelease(name string) bool { _, _, ok := EpisodeNumbers(name); return ok }
func MatchesMovieText(name, title string, year int) bool {
	if IsEpisodicRelease(name) {
		return false
	}
	normalize := func(s string) string {
		return strings.TrimSpace(titleSeparators.ReplaceAllString(strings.ToLower(s), " "))
	}
	wanted := normalize(title)
	if wanted == "" || !strings.Contains(" "+normalize(name)+" ", " "+wanted+" ") {
		return false
	}
	if year > 0 && !regexp.MustCompile(fmt.Sprintf(`(?:^|[^0-9])%d(?:[^0-9]|$)`, year)).MatchString(name) {
		return false
	}
	return true
}

// LooksLikeMovie protects the obfuscated-file fallback during episode import.
// A recognizable series title followed by a movie subtitle/year is not an
// unnamed episode file merely because the download contains one video.
func LooksLikeMovie(name, title string) bool {
	_, ok := TitleTail(name, title)
	return ok && releaseYear.MatchString(name) && !IsEpisodicRelease(name)
}

func FilterEpisodeReleases(releases []Release, title string, season, episode, absolute int) []Release {
	filtered := make([]Release, 0, len(releases))
	for _, release := range releases {
		if MatchesSeriesEpisode(release.Title, title, season, episode, absolute) {
			filtered = append(filtered, release)
		}
	}
	return filtered
}
func FilterMovieReleases(releases []Release, title string, year int) []Release {
	filtered := make([]Release, 0, len(releases))
	for _, release := range releases {
		if MatchesMovieText(release.Title, title, year) {
			filtered = append(filtered, release)
		}
	}
	return filtered
}

// IsMultiEpisodeRelease identifies an explicit combined season/episode file.
// Importers must not silently assign only its first episode.
func IsMultiEpisodeRelease(name string) bool { return combinedEpisodeToken.MatchString(name) }
func HasAbsoluteEpisodeRange(name, title string) bool {
	tail, ok := TitleTail(name, title)
	if !ok {
		tail = name
	}
	tail = namedEpisode.ReplaceAllString(tail, "")
	return regexp.MustCompile(`(?i)^\d{1,4}(?:v\d+)?[ _]*[-~][ _]*\d{1,4}(?:v\d+)?(?:$|[^0-9])`).MatchString(tail)
}
