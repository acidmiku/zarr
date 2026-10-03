package indexer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

//go:embed trash/presets.json
var trashPresetData []byte

type FormatMatch struct {
	TrashID string `json:"id"`
	Name    string `json:"name"`
	Score   int    `json:"score"`
}

// QualityGroups are ordered best first. Members of one group have equal rank.
type ScoringConfig struct {
	Preset                  string         `json:"preset"`
	QualityGroups           [][]string     `json:"quality_groups"`
	MinimumFormatScore      int            `json:"minimum_format_score"`
	UpgradeUntilQuality     string         `json:"upgrade_until_quality"`
	UpgradeUntilFormatScore int            `json:"upgrade_until_format_score"`
	FormatScores            map[string]int `json:"format_scores"`
	DualAudio               string         `json:"dual_audio"`
}

type ScoringPreset struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	MediaType string        `json:"media_type"`
	Version   string        `json:"version"`
	SourceURL string        `json:"source_url"`
	Config    ScoringConfig `json:"config"`
	Qualities []string      `json:"qualities"`
	Formats   []FormatMatch `json:"formats"`
}

type trashSpec struct {
	Name           string `json:"name"`
	Implementation string `json:"implementation"`
	Negate         bool   `json:"negate"`
	Required       bool   `json:"required"`
	Fields         struct {
		Value json.RawMessage `json:"value"`
	} `json:"fields"`
	regex  *regexp2.Regexp
	number int
}
type trashFormat struct {
	ID             string         `json:"trash_id"`
	Name           string         `json:"name"`
	Scores         map[string]int `json:"trash_scores"`
	Specifications []trashSpec    `json:"specifications"`
}
type trashProfile struct {
	Profile struct {
		SourceURL          string `json:"trash_url"`
		ScoreSet           string `json:"trash_score_set"`
		MinimumFormatScore int    `json:"minFormatScore"`
		CutoffFormatScore  int    `json:"cutoffFormatScore"`
		Items              []struct {
			Name    string   `json:"name"`
			Allowed bool     `json:"allowed"`
			Items   []string `json:"items"`
		} `json:"items"`
	} `json:"profile"`
	Formats []trashFormat `json:"formats"`
}
type trashBundle struct {
	Revision string                  `json:"revision"`
	Profiles map[string]trashProfile `json:"profiles"`
}

var bundle trashBundle
var bundleErr error
var bundleOnce sync.Once

func loadTRaSH() error {
	bundleOnce.Do(func() {
		if bundleErr = json.Unmarshal(trashPresetData, &bundle); bundleErr != nil {
			return
		}
		for key, profile := range bundle.Profiles {
			for fi := range profile.Formats {
				for si := range profile.Formats[fi].Specifications {
					spec := &profile.Formats[fi].Specifications[si]
					if bundleErr = compileTRaSHSpec(spec); bundleErr != nil {
						return
					}
				}
			}
			bundle.Profiles[key] = profile
		}
	})
	return bundleErr
}

var qualityAliases = map[string]string{
	"Bluray-1080p Remux": "remux-1080p", "Remux-1080p": "remux-1080p", "Bluray-1080p": "bluray-1080p",
	"HDTV-1080p": "hdtv-1080p", "WEBRip-1080p": "web-1080p", "WEBDL-1080p": "web-1080p",
	"Bluray-720p": "bluray-720p", "HDTV-720p": "hdtv-720p", "WEBRip-720p": "web-720p", "WEBDL-720p": "web-720p",
	"Bluray-576p": "bluray-576p", "Bluray-480p": "bluray-480p", "WEBRip-480p": "web-480p", "WEBDL-480p": "web-480p",
	"DVD": "dvd", "SDTV": "sdtv",
	"Bluray-2160p Remux": "remux-2160p", "Remux-2160p": "remux-2160p", "Bluray-2160p": "bluray-2160p",
	"HDTV-2160p": "hdtv-2160p", "WEBRip-2160p": "web-2160p", "WEBDL-2160p": "web-2160p",
}

func DefaultScoringConfig(preset string) (ScoringConfig, error) {
	if err := loadTRaSH(); err != nil {
		return ScoringConfig{}, err
	}
	profile, ok := bundle.Profiles[preset]
	if !ok {
		return ScoringConfig{}, fmt.Errorf("unknown preset %q", preset)
	}
	config := ScoringConfig{Preset: preset, MinimumFormatScore: profile.Profile.MinimumFormatScore,
		UpgradeUntilQuality: "bluray-1080p", UpgradeUntilFormatScore: profile.Profile.CutoffFormatScore,
		FormatScores: make(map[string]int), DualAudio: "optional"}
	for _, item := range profile.Profile.Items {
		if !item.Allowed {
			continue
		}
		names := item.Items
		if len(names) == 0 {
			names = []string{item.Name}
		}
		var group []string
		seen := map[string]bool{}
		for _, name := range names {
			quality, ok := qualityAliases[name]
			if !ok {
				return ScoringConfig{}, fmt.Errorf("unsupported upstream quality %q", name)
			}
			if !seen[quality] {
				group = append(group, quality)
				seen[quality] = true
			}
		}
		config.QualityGroups = append(config.QualityGroups, group)
	}
	for _, format := range profile.Formats {
		score, exists := format.Scores[profile.Profile.ScoreSet]
		if !exists {
			score = format.Scores["default"]
		}
		config.FormatScores[format.ID] = score
	}
	return config, nil
}

// ResolveScoringConfig expands defaults only for omitted fields. Explicit zero
// values and individual custom-format overrides are never replaced by defaults.
func ResolveScoringConfig(raw string) (ScoringConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return ScoringConfig{}, nil
	}
	if len(raw) > 65536 {
		return ScoringConfig{}, fmt.Errorf("configuration is too large")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || fields == nil {
		return ScoringConfig{}, fmt.Errorf("configuration must be an object")
	}
	if len(fields) == 0 {
		return ScoringConfig{}, nil
	}
	var overrides struct {
		Preset                  string         `json:"preset"`
		QualityGroups           *[][]string    `json:"quality_groups"`
		MinimumFormatScore      *int           `json:"minimum_format_score"`
		UpgradeUntilQuality     *string        `json:"upgrade_until_quality"`
		UpgradeUntilFormatScore *int           `json:"upgrade_until_format_score"`
		FormatScores            map[string]int `json:"format_scores"`
		DualAudio               *string        `json:"dual_audio"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&overrides); err != nil {
		return ScoringConfig{}, fmt.Errorf("invalid scoring configuration: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ScoringConfig{}, fmt.Errorf("unexpected trailing JSON")
	}
	for field, value := range fields {
		if string(value) == "null" {
			return ScoringConfig{}, fmt.Errorf("%s cannot be null", field)
		}
	}
	if rawScores, ok := fields["format_scores"]; ok {
		var scores map[string]json.RawMessage
		if err := json.Unmarshal(rawScores, &scores); err != nil {
			return ScoringConfig{}, fmt.Errorf("format_scores must be an object")
		}
		for id, value := range scores {
			if strings.TrimSpace(string(value)) == "null" {
				return ScoringConfig{}, fmt.Errorf("format score %q cannot be null", id)
			}
		}
	}
	config, err := DefaultScoringConfig(overrides.Preset)
	if err != nil {
		return ScoringConfig{}, err
	}
	if overrides.QualityGroups != nil {
		config.QualityGroups = *overrides.QualityGroups
	}
	if overrides.MinimumFormatScore != nil {
		config.MinimumFormatScore = *overrides.MinimumFormatScore
	}
	if overrides.UpgradeUntilQuality != nil {
		config.UpgradeUntilQuality = *overrides.UpgradeUntilQuality
	}
	if overrides.UpgradeUntilFormatScore != nil {
		config.UpgradeUntilFormatScore = *overrides.UpgradeUntilFormatScore
	}
	if overrides.DualAudio != nil {
		config.DualAudio = *overrides.DualAudio
	}
	for id, score := range overrides.FormatScores {
		if _, ok := config.FormatScores[id]; !ok {
			return ScoringConfig{}, fmt.Errorf("unknown custom format %q", id)
		}
		config.FormatScores[id] = score
	}
	if err := validateResolvedConfig(config); err != nil {
		return ScoringConfig{}, err
	}
	return config, nil
}

func ParseScoringConfig(raw string) (ScoringConfig, error) { return ResolveScoringConfig(raw) }
func ValidateScoringConfig(raw string) error               { _, err := ResolveScoringConfig(raw); return err }

func validateResolvedConfig(config ScoringConfig) error {
	if len(config.QualityGroups) == 0 || len(config.QualityGroups) > 20 {
		return fmt.Errorf("quality_groups must contain 1 to 20 groups")
	}
	valid := map[string]bool{}
	for _, quality := range qualityAliases {
		valid[quality] = true
	}
	seen := map[string]bool{}
	for _, group := range config.QualityGroups {
		if len(group) == 0 {
			return fmt.Errorf("quality groups cannot be empty")
		}
		for _, quality := range group {
			if !valid[quality] || seen[quality] {
				return fmt.Errorf("unknown or repeated quality %q", quality)
			}
			seen[quality] = true
		}
	}
	if !seen[config.UpgradeUntilQuality] {
		return fmt.Errorf("upgrade cutoff must belong to an allowed quality group")
	}
	if config.MinimumFormatScore < -100000 || config.MinimumFormatScore > 100000 || config.UpgradeUntilFormatScore < config.MinimumFormatScore || config.UpgradeUntilFormatScore > 100000 {
		return fmt.Errorf("invalid minimum or upgrade format score")
	}
	for _, score := range config.FormatScores {
		if score < -100000 || score > 100000 {
			return fmt.Errorf("format scores must be between -100000 and 100000")
		}
	}
	switch config.DualAudio {
	case "optional", "within-tier", "above-tier", "required":
	default:
		return fmt.Errorf("invalid dual_audio preference")
	}
	return nil
}

func ScoringPresets() []ScoringPreset {
	if loadTRaSH() != nil {
		return []ScoringPreset{}
	}
	var presets []ScoringPreset
	for _, id := range []string{"sonarr-anime", "radarr-anime"} {
		config, err := DefaultScoringConfig(id)
		if err != nil {
			continue
		}
		name, mediaType := "Anime Series \u00b7 TRaSH", "series"
		if id == "radarr-anime" {
			name, mediaType = "Anime Movies \u00b7 TRaSH", "movie"
		}
		preset := ScoringPreset{ID: id, Name: name, MediaType: mediaType, Version: bundle.Revision, SourceURL: bundle.Profiles[id].Profile.SourceURL, Config: config}
		for _, group := range config.QualityGroups {
			preset.Qualities = append(preset.Qualities, group...)
		}
		for _, format := range bundle.Profiles[id].Formats {
			preset.Formats = append(preset.Formats, FormatMatch{format.ID, format.Name, config.FormatScores[format.ID]})
		}
		presets = append(presets, preset)
	}
	return presets
}
