package indexer

import (
	"regexp"
	"strings"
)

// ParsedRelease holds extracted info from a release name.
type ParsedRelease struct {
	Quality string
	Tags    []string
}

// Quality detection patterns, ordered by typical preference.
var qualityPatterns = []struct {
	Name    string
	Pattern *regexp.Regexp
	Reject  *regexp.Regexp // must NOT match
}{
	{"remux-2160p", regexp.MustCompile(`(?i)(remux.*2160p|2160p.*remux)`), nil},
	{"remux-1080p", regexp.MustCompile(`(?i)(remux.*1080p|1080p.*remux)`), nil},
	{"bluray-2160p", regexp.MustCompile(`(?i)(blu.?ray.*2160p|2160p.*blu.?ray)`), regexp.MustCompile(`(?i)remux`)},
	{"bluray-1080p", regexp.MustCompile(`(?i)(blu.?ray.*1080p|1080p.*blu.?ray)`), regexp.MustCompile(`(?i)remux`)},
	{"web-2160p", regexp.MustCompile(`(?i)(web.?dl.*2160p|2160p.*web.?dl|web.?rip.*2160p|webrip.*2160p|2160p.*web.?rip)`), nil},
	{"web-1080p", regexp.MustCompile(`(?i)(web.?dl.*1080p|1080p.*web.?dl|web.?rip.*1080p|webrip.*1080p|1080p.*web.?rip|1080p.*web)`), nil},
	{"web-720p", regexp.MustCompile(`(?i)(web.?dl.*720p|720p.*web.?dl|web.?rip.*720p|720p.*web)`), nil},
	{"hdtv-1080p", regexp.MustCompile(`(?i)(hdtv.*1080p|1080p.*hdtv)`), nil},
	{"hdtv-720p", regexp.MustCompile(`(?i)(hdtv.*720p|720p.*hdtv)`), nil},
}

// For anime releases that just say 1080p without a source, assume web.
var fallbackResolution = map[string]*regexp.Regexp{
	"web-1080p": regexp.MustCompile(`(?i)\b1080p\b`),
	"web-720p":  regexp.MustCompile(`(?i)\b720p\b`),
	"web-2160p": regexp.MustCompile(`(?i)\b2160p\b`),
}

// Tag detection patterns.
var tagPatterns = []struct {
	Name    string
	Pattern *regexp.Regexp
}{
	{"10bit", regexp.MustCompile(`(?i)(10.?bit|hi10p?|hi10)`)},
	{"dual-audio", regexp.MustCompile(`(?i)(dual.?audio|multi.?audio)`)},
	{"uncensored", regexp.MustCompile(`(?i)uncensored`)},
	{"imax-enhanced", regexp.MustCompile(`(?i)imax.?enhanced`)},
	{"hdr", regexp.MustCompile(`(?i)(\bHDR\b|HDR10|HDR10\+|Dolby\.?Vision|DoVi|\bDV\b)`)},
	{"remux", regexp.MustCompile(`(?i)\bremux\b`)},
}

// Language detection patterns.
var langPatterns = struct {
	DualAudio  *regexp.Regexp
	Multi      *regexp.Regexp
	Japanese   *regexp.Regexp
	English    *regexp.Regexp
	EngSub     *regexp.Regexp
	Dubbed     *regexp.Regexp
	Raw        *regexp.Regexp
}{
	DualAudio: regexp.MustCompile(`(?i)\bDual.?Audio\b`),
	Multi:     regexp.MustCompile(`(?i)\bMulti\b`),
	Japanese:  regexp.MustCompile(`(?i)(\bJapanese\b|\bJPN\b|\bJA\b)`),
	English:   regexp.MustCompile(`(?i)(\bEnglish\b|\bENG\b)`),
	EngSub:    regexp.MustCompile(`(?i)(\bENGSUB\b|English.?Sub)`),
	Dubbed:    regexp.MustCompile(`(?i)(\bDUBBED\b|\bDUB\b)`),
	Raw:       regexp.MustCompile(`(?i)\bRaw\b`),
}

// ParseReleaseName extracts quality and tags from a release name.
func ParseReleaseName(name string) ParsedRelease {
	result := ParsedRelease{}

	// Detect quality
	for _, qp := range qualityPatterns {
		if qp.Pattern.MatchString(name) {
			if qp.Reject != nil && qp.Reject.MatchString(name) {
				continue
			}
			result.Quality = qp.Name
			break
		}
	}

	// Fallback: if no specific source+resolution matched, check resolution alone
	if result.Quality == "" {
		for quality, pattern := range fallbackResolution {
			if pattern.MatchString(name) {
				result.Quality = quality
				break
			}
		}
	}

	// Detect tags
	for _, tp := range tagPatterns {
		if tp.Pattern.MatchString(name) {
			result.Tags = append(result.Tags, tp.Name)
		}
	}

	return result
}

// LanguageAcceptable checks if a release is acceptable for the given language preset.
func LanguageAcceptable(releaseName, langPreset string) bool {
	switch langPreset {
	case "ja-en":
		// Reject dubbed/raw
		if langPatterns.Dubbed.MatchString(releaseName) {
			return false
		}
		if langPatterns.Raw.MatchString(releaseName) {
			return false
		}
		return true
	case "en":
		// Accept everything (most releases on English indexers are English)
		return true
	default:
		return true
	}
}

// MatchesRejectPatterns checks if a release name matches any reject patterns.
func MatchesRejectPatterns(releaseName string, patterns []string) bool {
	lower := strings.ToLower(releaseName)
	for _, p := range patterns {
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(p) + `\b`).MatchString(lower) {
			return true
		}
	}
	return false
}
