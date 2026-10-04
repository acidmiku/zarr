package indexer

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type MusicIdentity struct {
	Artist        string   `json:"artist"`
	Album         string   `json:"album"`
	ArtistAliases []string `json:"artist_aliases,omitempty"`
	AlbumAliases  []string `json:"album_aliases,omitempty"`
	AlbumType     string   `json:"album_type,omitempty"`
	Edition       string   `json:"edition,omitempty"`
}

func normalizeMusicName(value string) string {
	value = cases.Fold().String(norm.NFKC.String(value))
	var text strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			text.WriteRune(r)
		case r == '\'' || r == '’':
		case r == '&':
			text.WriteString(" and ")
		default:
			text.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(text.String()), " ")
}

// Equality deliberately retains accents, words and edition qualifiers. Aliases
// must be explicitly supplied by metadata rather than guessed by substring.
func MatchesMusicName(a, b string) bool {
	left, right := normalizeMusicName(a), normalizeMusicName(b)
	return left != "" && left == right
}

var musicYearPrefix = regexp.MustCompile(`^(?:19|20)\d{2} `)
var musicSceneGroup = regexp.MustCompile(`-[A-Za-z][A-Za-z0-9]{1,20}$`)
var musicMetadataNumber = regexp.MustCompile(`^(?:(?:19|20)\d{2}|\d+(?:cd|lp|disc|discs|kbps|kbit|khz|bit)|16|24|32|44|48|88|96|176|192|256|320|1|2|v0|v2)$`)
var musicMetadataWords = map[string]bool{
	"flac": true, "alac": true, "wav": true, "ape": true, "lossless": true,
	"mp3": true, "aac": true, "m4a": true, "ogg": true, "opus": true,
	"web": true, "cd": true, "cds": true, "cdda": true, "lp": true, "vinyl": true, "digital": true,
	"rip": true, "webrip": true, "webflac": true, "eac": true, "log": true, "cue": true,
	"stereo": true, "joint": true, "vbr": true, "cbr": true, "kbps": true, "khz": true, "bit": true,
	"hi": true, "res": true, "hires": true, "proper": true, "repack": true,
}

var musicEditionWords = map[string]string{
	"live": "live", "remix": "remix", "remixes": "remix", "deluxe": "deluxe", "expanded": "expanded",
	"anniversary": "anniversary", "remastered": "remaster", "remaster": "remaster", "instrumental": "instrumental", "karaoke": "karaoke",
}

func musicEditionFlags(value string) map[string]bool {
	result := map[string]bool{}
	for _, word := range strings.Fields(normalizeMusicName(value)) {
		if flag := musicEditionWords[word]; flag != "" {
			result[flag] = true
		}
	}
	return result
}

func musicIdentityTail(release string, identity MusicIdentity) (string, string, bool) {
	name := normalizeMusicName(release)
	for _, artist := range append([]string{identity.Artist}, identity.ArtistAliases...) {
		artist = normalizeMusicName(artist)
		if artist == "" || !strings.HasPrefix(name, artist+" ") {
			continue
		}
		withoutArtist := strings.TrimPrefix(name, artist+" ")
		variants := []string{withoutArtist, musicYearPrefix.ReplaceAllString(withoutArtist, "")}
		for _, variant := range variants {
			for _, album := range append([]string{identity.Album}, identity.AlbumAliases...) {
				album = normalizeMusicName(album)
				if album != "" && (variant == album || strings.HasPrefix(variant, album+" ")) {
					return strings.TrimSpace(strings.TrimPrefix(variant, album)), variant, true
				}
			}
		}
	}
	return "", "", false
}

// MusicReleaseRejection checks album identity before any quality preference.
// Unexplained text after the album is not treated as a codec/release group: it
// can be a track title, another album, or a different edition.
func MusicReleaseRejection(title string, identity MusicIdentity) string {
	if normalizeMusicName(identity.Artist) == "" || normalizeMusicName(identity.Album) == "" {
		return "artist and album identity are required"
	}
	searchTitle := title
	if ParseMusicReleaseName(title).Quality != "" {
		group := strings.TrimPrefix(musicSceneGroup.FindString(title), "-")
		if musicEditionWords[strings.ToLower(group)] == "" && !musicMetadataWords[strings.ToLower(group)] {
			searchTitle = musicSceneGroup.ReplaceAllString(title, "")
		}
	}
	tail, albumBody, matched := musicIdentityTail(searchTitle, identity)
	if !matched {
		return "artist or album does not match"
	}
	expectedEdition, foundEdition := musicEditionFlags(identity.Album+" "+identity.Edition), musicEditionFlags(albumBody)
	if len(expectedEdition) != len(foundEdition) {
		return "different album edition or version"
	}
	for flag := range expectedEdition {
		if !foundEdition[flag] {
			return "different album edition or version"
		}
	}
	for _, token := range strings.Fields(tail) {
		if expectedEdition[musicEditionWords[token]] || len(expectedEdition) > 0 && token == "edition" {
			continue
		}
		if token == "single" || token == "ep" {
			if strings.EqualFold(identity.AlbumType, token) {
				continue
			}
			return "single/EP does not match requested album"
		}
		switch token {
		case "live", "remix", "remixes", "deluxe", "expanded", "anniversary", "remastered", "remaster", "instrumental", "karaoke":
			return "different album edition or version"
		case "discography", "collection", "boxset", "box", "anthology":
			return "album collections are not an individual album"
		}
		if !musicMetadataWords[token] && !musicMetadataNumber.MatchString(token) {
			return "unrecognized album suffix (possible track or different edition)"
		}
	}
	return ""
}

func ScoreMusicRelease(release *Release, profile *QualityProfile, identity MusicIdentity) {
	if reason := MusicReleaseRejection(release.Title, identity); reason != "" {
		release.Score, release.Acceptable, release.RejectReason = 0, false, reason
		return
	}
	parsed := ParseMusicReleaseName(release.Title)
	release.Quality, release.Tags = parsed.Quality, parsed.Tags
	ScoreRelease(release, profile)
}
