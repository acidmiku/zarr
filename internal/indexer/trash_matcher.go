package indexer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
)

const maxScoringTitleBytes = 4096

func compileTRaSHSpec(spec *trashSpec) error {
	switch spec.Implementation {
	case "ReleaseTitleSpecification", "ReleaseGroupSpecification":
		var pattern string
		if err := json.Unmarshal(spec.Fields.Value, &pattern); err != nil || pattern == "" {
			return fmt.Errorf("invalid regex in %s", spec.Name)
		}
		compiled, err := regexp2.Compile(pattern, regexp2.IgnoreCase)
		if err != nil {
			return fmt.Errorf("unsupported regex in %s: %w", spec.Name, err)
		}
		compiled.MatchTimeout = 50 * time.Millisecond
		spec.regex = compiled
	case "SourceSpecification", "LanguageSpecification", "QualityModifierSpecification":
		if err := json.Unmarshal(spec.Fields.Value, &spec.number); err != nil {
			return fmt.Errorf("invalid numeric specification %s", spec.Name)
		}
		if spec.Implementation == "LanguageSpecification" && spec.number != 8 && spec.number != 10 && spec.number != 21 {
			return fmt.Errorf("unsupported language specification %d", spec.number)
		}
		if spec.Implementation == "QualityModifierSpecification" && spec.number != 5 {
			return fmt.Errorf("unsupported quality modifier %d", spec.number)
		}
		if spec.Implementation == "SourceSpecification" && (spec.number < 1 || spec.number > 9) {
			return fmt.Errorf("unsupported quality source %d", spec.number)
		}
	default:
		return fmt.Errorf("unsupported custom format specification %q", spec.Implementation)
	}
	return nil
}

type formatInput struct {
	title, group     string
	source, modifier int
	languages        map[int]bool
}

// Sonarr ANDs implementation groups. Within a group all required conditions
// must pass and at least one condition (required or optional) must pass.
// A required true condition already satisfies the latter; an optional match
// cannot rescue a failed required condition. Negation is applied first.
func matchesTRaSHFormat(format trashFormat, input formatInput) (bool, error) {
	if len(format.Specifications) == 0 {
		return false, fmt.Errorf("custom format has no specifications")
	}
	groups := map[string]bool{}
	for _, spec := range format.Specifications {
		matched := false
		var err error
		switch spec.Implementation {
		case "ReleaseTitleSpecification":
			matched, err = spec.regex.MatchString(input.title)
		case "ReleaseGroupSpecification":
			matched, err = spec.regex.MatchString(input.group)
		case "SourceSpecification":
			matched = input.source == spec.number
		case "QualityModifierSpecification":
			matched = input.modifier == spec.number
		case "LanguageSpecification":
			matched = input.languages[spec.number]
		default:
			return false, fmt.Errorf("unsupported specification %q", spec.Implementation)
		}
		if err != nil {
			return false, fmt.Errorf("custom format matching exceeded its time limit")
		}
		if spec.Negate {
			matched = !matched
		}
		if spec.Required && !matched {
			return false, nil
		}
		groups[spec.Implementation] = groups[spec.Implementation] || matched
	}
	for _, matched := range groups {
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

var leadingReleaseGroup = regexp.MustCompile(`^\[([^\[\]]+)\]`)
var trailingReleaseGroup = regexp.MustCompile(`-([A-Za-z0-9][A-Za-z0-9_-]*)$`)
var webRipSource = regexp.MustCompile(`(?i)\bweb[ ._-]?rip\b`)
var chineseAudio = regexp.MustCompile(`(?i)\b(chinese|mandarin|cantonese|zh|chi|zho)\b`)
var koreanAudio = regexp.MustCompile(`(?i)\b(korean|ko|kor)\b`)

func buildFormatInput(rel *Release, preset string) formatInput {
	input := formatInput{title: rel.Title, group: rel.ReleaseGroup, languages: map[int]bool{}}
	if input.group == "" {
		name := rel.Title
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".mkv" || ext == ".mp4" || ext == ".avi" || ext == ".m4v" {
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		if match := leadingReleaseGroup.FindStringSubmatch(name); match != nil {
			input.group = match[1]
		} else if match := trailingReleaseGroup.FindStringSubmatch(name); match != nil {
			input.group = match[1]
		}
	}
	// Source enum values differ between Sonarr and Radarr. WEB-DL/WEBRip share
	// an app quality group but remain distinct custom-format inputs.
	source := ""
	switch {
	case strings.HasPrefix(rel.Quality, "remux-"):
		source = "remux"
		input.modifier = 5
	case strings.HasPrefix(rel.Quality, "bluray-"):
		source = "bluray"
	case strings.HasPrefix(rel.Quality, "web-"):
		source = "web"
		if webRipSource.MatchString(rel.Title) {
			source = "webrip"
		}
	case strings.HasPrefix(rel.Quality, "hdtv-") || rel.Quality == "sdtv":
		source = "tv"
	case rel.Quality == "dvd":
		source = "dvd"
	}
	if preset == "sonarr-anime" {
		input.source = map[string]int{"tv": 1, "web": 3, "webrip": 4, "dvd": 5, "bluray": 6, "remux": 7}[source]
	} else {
		input.source = map[string]int{"tv": 6, "web": 7, "webrip": 8, "dvd": 5, "bluray": 9, "remux": 9}[source]
	}
	if len(rel.Languages) > 0 {
		for _, language := range rel.Languages {
			switch strings.ToLower(language) {
			case "ja", "jpn", "japanese":
				input.languages[8] = true
			case "zh", "zho", "chi", "chinese":
				input.languages[10] = true
			case "ko", "kor", "korean":
				input.languages[21] = true
			}
		}
	} else {
		if langPatterns.Japanese.MatchString(rel.Title) {
			input.languages[8] = true
		}
		if chineseAudio.MatchString(rel.Title) {
			input.languages[10] = true
		}
		if koreanAudio.MatchString(rel.Title) {
			input.languages[21] = true
		}
	}
	return input
}

func qualityRank(quality string, config ScoringConfig) int {
	for i, group := range config.QualityGroups {
		for _, q := range group {
			if q == quality {
				return len(config.QualityGroups) - i
			}
		}
	}
	return 0
}

func scoreTRaSHRelease(rel *Release, profile *QualityProfile, config ScoringConfig) {
	rel.ScoringPreset, rel.ScoringVersion = config.Preset, bundle.Revision
	if len(rel.Title) > maxScoringTitleBytes || len(rel.ReleaseGroup) > maxScoringTitleBytes {
		rel.RejectReason = "release title too long for custom format evaluation"
		return
	}
	var rejectPatterns []string
	if profile.RejectPatterns != "" && json.Unmarshal([]byte(profile.RejectPatterns), &rejectPatterns) != nil {
		rel.RejectReason = "invalid reject patterns"
		return
	}
	if MatchesRejectPatterns(rel.Title, rejectPatterns) {
		rel.RejectReason = "matches reject pattern"
		return
	}
	var allowed []string
	if json.Unmarshal([]byte(profile.Qualities), &allowed) != nil {
		rel.RejectReason = "invalid quality profile"
		return
	}
	if rel.Quality == "" {
		rel.Quality = ParseReleaseName(rel.Title).Quality
	}
	rel.QualityRank = qualityRank(rel.Quality, config)
	qualityAllowed := false
	for _, quality := range allowed {
		if rel.Quality == quality {
			qualityAllowed = true
		}
	}
	if !qualityAllowed || rel.QualityRank == 0 {
		rel.RejectReason = "quality not in profile"
		return
	}
	input := buildFormatInput(rel, config.Preset)
	// Infer unspecified anime language only after the actual upstream dual
	// title conditions pass. A narrower auxiliary regex missed supported names
	// such as "1080p WEB-DL DUAL"; copying/flattening those patterns can also
	// lose their negative lookarounds. Explicit language metadata always wins.
	if len(rel.Languages) == 0 && len(input.languages) == 0 {
		for _, format := range bundle.Profiles[config.Preset].Formats {
			if format.Name != "Anime Dual Audio" {
				continue
			}
			var titleConditions trashFormat
			for _, spec := range format.Specifications {
				if spec.Implementation == "ReleaseTitleSpecification" {
					titleConditions.Specifications = append(titleConditions.Specifications, spec)
				}
			}
			matched, err := matchesTRaSHFormat(titleConditions, input)
			if err != nil {
				rel.RejectReason = err.Error()
				return
			}
			if matched {
				input.languages[8] = true
			}
		}
	}
	dualAudio := false
	for _, format := range bundle.Profiles[config.Preset].Formats {
		matched, err := matchesTRaSHFormat(format, input)
		if err != nil {
			rel.RejectReason = err.Error()
			rel.FormatScore = 0
			rel.Score = 0
			rel.MatchedFormats = nil
			return
		}
		if matched {
			score := config.FormatScores[format.ID]
			// Apply named preferences at evaluation time, preserving the user's
			// declared override when a config is serialized and changed later.
			if format.Name == "Anime Dual Audio" {
				switch config.DualAudio {
				case "within-tier":
					score = 10
				case "above-tier":
					score = 101
				case "required":
					score = 2000
				}
			}
			rel.MatchedFormats = append(rel.MatchedFormats, FormatMatch{format.ID, format.Name, score})
			rel.FormatScore += score
			if format.Name == "Anime Dual Audio" {
				dualAudio = true
			}
		}
	}
	rel.Score = rel.FormatScore // Display compatibility only; ranking uses the tuple.
	if config.DualAudio == "required" && !dualAudio {
		rel.RejectReason = "dual audio is required"
		return
	}
	if rel.FormatScore < config.MinimumFormatScore {
		rel.RejectReason = fmt.Sprintf("custom format score %d is below minimum %d", rel.FormatScore, config.MinimumFormatScore)
		return
	}
	rel.Acceptable = true
}

// CompareReleases returns positive when a is preferred. Callers must compare
// releases scored against the same profile, and filter Acceptable separately.
func CompareReleases(a, b *Release) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return -1
	}
	if b == nil {
		return 1
	}
	compare := func(a, b int) int {
		if a > b {
			return 1
		}
		if a < b {
			return -1
		}
		return 0
	}
	if a.ScoringPreset != "" || b.ScoringPreset != "" {
		if rank := compare(a.QualityRank, b.QualityRank); rank != 0 {
			return rank
		}
		return compare(a.FormatScore, b.FormatScore)
	}
	return compare(a.Score, b.Score)
}

// IsUpgrade expects candidate and current scored with this profile. Quality
// upgrades stop at the quality cutoff; same-group format upgrades stop at the
// format cutoff. Neither kind permits a downgrade to a lower quality group.
func IsUpgrade(candidate, current *Release, profile *QualityProfile) bool {
	if candidate == nil || current == nil || profile == nil || !profile.UpgradeAllowed || !candidate.Acceptable {
		return false
	}
	config, err := ResolveScoringConfig(profile.ScoringConfig)
	if err != nil {
		return false
	}
	if config.Preset == "" {
		return candidate.Score > current.Score
	}
	if candidate.ScoringPreset != config.Preset || current.ScoringPreset != config.Preset || current.QualityRank == 0 || candidate.QualityRank < current.QualityRank {
		return false
	}
	if candidate.QualityRank > current.QualityRank {
		return current.QualityRank < qualityRank(config.UpgradeUntilQuality, config)
	}
	return candidate.FormatScore > current.FormatScore && current.FormatScore < config.UpgradeUntilFormatScore
}
