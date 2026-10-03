package indexer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTRaSHSnapshotIntegrityAndUnmodifiedDefinitions(t *testing.T) {
	var manifest struct {
		Revision string            `json:"revision"`
		Files    map[string]string `json:"files"`
	}
	manifestData, err := os.ReadFile("trash/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if err = loadTRaSH(); err != nil {
		t.Fatal(err)
	}
	if manifest.Revision != bundle.Revision {
		t.Fatal("manifest and bundle revisions differ")
	}
	for name, expected := range manifest.Files {
		data, err := os.ReadFile(filepath.Join("trash", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != expected {
			t.Fatalf("vendored snapshot modified: %s", name)
		}
	}
	var raw struct {
		Profiles map[string]struct {
			Formats []json.RawMessage `json:"formats"`
		} `json:"profiles"`
	}
	if err = json.Unmarshal(trashPresetData, &raw); err != nil {
		t.Fatal(err)
	}
	for preset, profile := range raw.Profiles {
		app := strings.TrimSuffix(preset, "-anime")
		originals := map[string]any{}
		paths, err := filepath.Glob("trash/" + app + "/*.json")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if filepath.Base(path) == "quality-profile.json" {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err = json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			originals[value["trash_id"].(string)] = value
		}
		for _, definition := range profile.Formats {
			var value map[string]any
			if err = json.Unmarshal(definition, &value); err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(value)
			expected, _ := json.Marshal(originals[value["trash_id"].(string)])
			if string(actual) != string(expected) {
				t.Fatal("bundled format differs from unmodified upstream JSON")
			}
		}
	}
}

func presetProfile(t *testing.T, preset string) *QualityProfile {
	t.Helper()
	config, err := DefaultScoringConfig(preset)
	if err != nil {
		t.Fatal(err)
	}
	var qualities []string
	for _, group := range config.QualityGroups {
		qualities = append(qualities, group...)
	}
	encoded, _ := json.Marshal(qualities)
	return &QualityProfile{Qualities: string(encoded), Tags: `{"dual-audio":99999}`, Language: "ja-en", UpgradeAllowed: true, ScoringConfig: fmt.Sprintf(`{"preset":%q}`, preset)}
}
func scored(t *testing.T, profile *QualityProfile, title string) Release {
	t.Helper()
	rel := Release{Title: title, Quality: ParseReleaseName(title).Quality}
	ScoreRelease(&rel, profile)
	return rel
}
func hasFormat(rel Release, name string) bool {
	for _, f := range rel.MatchedFormats {
		if f.Name == name {
			return true
		}
	}
	return false
}

func TestPinnedTRaSHPresetsAndRegexesLoadCompletely(t *testing.T) {
	if err := loadTRaSH(); err != nil {
		t.Fatal(err)
	}
	presets := ScoringPresets()
	if len(presets) != 2 || bundle.Revision != "e7c97a676743d7430fc1c5808701c48252a2ac63" {
		t.Fatal("unexpected preset manifest")
	}
	for i, p := range presets {
		want := 41
		if i == 1 {
			want = 31
		}
		if len(p.Formats) != want || p.Config.MinimumFormatScore != 100 || p.Config.UpgradeUntilFormatScore != 10000 {
			t.Fatalf("incomplete preset: %+v", p)
		}
		if p.Config.QualityGroups[0][0] != "remux-1080p" || p.Config.QualityGroups[0][1] != "bluray-1080p" {
			t.Fatal("upstream grouped quality lost")
		}
		for _, format := range bundle.Profiles[p.ID].Formats {
			for _, spec := range format.Specifications {
				if strings.HasSuffix(spec.Implementation, "TitleSpecification") || spec.Implementation == "ReleaseGroupSpecification" {
					if spec.regex == nil || spec.regex.MatchTimeout <= 0 {
						t.Fatalf("uncompiled/unbounded pattern %s", spec.Name)
					}
				}
			}
		}
		for _, f := range p.Formats {
			if f.Name == "Remux Tier 01" && f.Score != 975 {
				t.Fatal("used non-anime remux score")
			}
		}
	}
}

func TestTRaSHRequiredOptionalAndNegatedConditions(t *testing.T) {
	for _, test := range []struct {
		name, title string
		source      int
		want        bool
	}{
		{"required alone satisfies group", "required", 6, true},
		{"optional cannot rescue required", "optional", 6, false},
		{"negated required denies", "required banned", 6, false},
		{"different specification group must pass", "required optional", 3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var format trashFormat
			json.Unmarshal([]byte(`{"specifications":[{"name":"required","implementation":"ReleaseTitleSpecification","required":true,"fields":{"value":"required"}},{"name":"optional","implementation":"ReleaseTitleSpecification","fields":{"value":"optional"}},{"name":"not banned","implementation":"ReleaseTitleSpecification","required":true,"negate":true,"fields":{"value":"banned"}},{"name":"BD","implementation":"SourceSpecification","fields":{"value":6}},{"name":"DVD","implementation":"SourceSpecification","fields":{"value":5}}]}`), &format)
			for i := range format.Specifications {
				if err := compileTRaSHSpec(&format.Specifications[i]); err != nil {
					t.Fatal(err)
				}
			}
			got, err := matchesTRaSHFormat(format, formatInput{title: test.title, source: test.source})
			if err != nil || got != test.want {
				t.Fatalf("matched=%t want=%t err=%v", got, test.want, err)
			}
		})
	}
	if err := compileTRaSHSpec(&trashSpec{Implementation: "FutureUnsupportedSpecification"}); err == nil {
		t.Fatal("unsupported rules silently ignored")
	}
}

func TestTRaSHSourceGatesAndAnimeScores(t *testing.T) {
	for _, preset := range []string{"sonarr-anime", "radarr-anime"} {
		t.Run(preset, func(t *testing.T) {
			profile := presetProfile(t, preset)
			for _, test := range []struct {
				title, format string
				score         int
			}{
				{"[FLE] Chainsaw Man - 01 [BD 1080p]", "Anime BD Tier 01", 1400},
				{"[FLE] Chainsaw Man - 01 [WEB-DL 1080p]", "Anime Web Tier 01", 600},
				{"[FLE] Chainsaw Man - 01v2 [BD 1080p]", "v2", 1402},
				{"[FLE] Chainsaw Man - 01v3 [BD 1080p]", "v3", 1403},
				{"Chainsaw.Man.1080p.BluRay.REMUX-FraMeSToR", "Remux Tier 01", 975},
			} {
				rel := scored(t, profile, test.title)
				if !rel.Acceptable || !hasFormat(rel, test.format) || rel.FormatScore != test.score {
					t.Fatalf("%s: %+v", test.title, rel)
				}
				if strings.Contains(test.title, "WEB-DL") && hasFormat(rel, "Anime BD Tier 01") {
					t.Fatal("BD tier matched web source")
				}
				if strings.Contains(test.title, "01v3") && hasFormat(rel, "v2") {
					t.Fatal("negative lookahead/required v2 guard lost")
				}
			}
			unknown := scored(t, profile, "[UnknownGroup] Chainsaw Man 1080p WEB-DL")
			if unknown.Acceptable || !strings.Contains(unknown.RejectReason, "below minimum") {
				t.Fatalf("unknown group should fail default minimum: %+v", unknown)
			}
			for _, title := range []string{"[FLE] Chainsaw Man 1080p BD AV1", "[Golumpa] Chainsaw Man 1080p WEB-DL"} {
				rel := scored(t, profile, title)
				if rel.Acceptable || rel.FormatScore >= 0 {
					t.Fatalf("penalty lost: %+v", rel)
				}
			}
		})
	}
}

func TestTRaSHQualityTupleBeatsHugeFormatBonusAndRespectsCutoffs(t *testing.T) {
	profile := presetProfile(t, "sonarr-anime")
	profile.ScoringConfig = `{"preset":"sonarr-anime","minimum_format_score":0,"format_scores":{"e0014372773c8f0e1bef8824f00c7dc4":90000}}`
	bd := scored(t, profile, "[FLE] Chainsaw Man 1080p BD")
	web := scored(t, profile, "[FLE] Chainsaw Man 1080p WEB-DL")
	if CompareReleases(&bd, &web) <= 0 || BestRelease([]Release{web, bd}).Quality != "bluray-1080p" {
		t.Fatal("large format bonus jumped quality group")
	}
	if !IsUpgrade(&bd, &web, profile) || IsUpgrade(&web, &bd, profile) {
		t.Fatal("upgrade comparison allowed downgrade or blocked better quality")
	}
	profile.ScoringConfig = `{"preset":"sonarr-anime","upgrade_until_format_score":1400}`
	bd = scored(t, profile, bd.Title)
	v2 := scored(t, profile, "[FLE] Chainsaw Man 01v2 1080p BD")
	if IsUpgrade(&v2, &bd, profile) {
		t.Fatal("ignored format cutoff")
	}
	profile.ScoringConfig = `{"preset":"sonarr-anime","upgrade_until_quality":"web-1080p"}`
	bd = scored(t, profile, bd.Title)
	web = scored(t, profile, web.Title)
	if IsUpgrade(&bd, &web, profile) {
		t.Fatal("ignored quality cutoff")
	}
	profile.ScoringConfig = `{"preset":"sonarr-anime"}`
	web = scored(t, profile, web.Title)
	webv2 := scored(t, profile, "[FLE] Chainsaw Man 01v2 1080p HDTV")
	if !IsUpgrade(&webv2, &web, profile) {
		t.Fatal("same-group HDTV format improvement rejected")
	}
	profile.UpgradeAllowed = false
	if IsUpgrade(&webv2, &web, profile) {
		t.Fatal("disabled upgrades accepted")
	}
}

func TestTRaSHDualAudioModesRequireActualMatch(t *testing.T) {
	profile := presetProfile(t, "sonarr-anime")
	for mode, bonus := range map[string]int{"optional": 0, "within-tier": 10, "above-tier": 101, "required": 2000} {
		profile.ScoringConfig = fmt.Sprintf(`{"preset":"sonarr-anime","dual_audio":%q}`, mode)
		rel := scored(t, profile, "[FLE] Chainsaw Man 1080p BD Dual Audio")
		if !rel.Acceptable || !hasFormat(rel, "Anime Dual Audio") || rel.FormatScore != 1400+bonus {
			t.Fatalf("%s: %+v", mode, rel)
		}
	}
	profile.ScoringConfig = `{"preset":"sonarr-anime","dual_audio":"required","format_scores":{"949c16fe0a8147f50ba82cc2df9411c9":9000}}`
	for _, title := range []string{"[FLE] Chainsaw Man 1080p BD", "[FLE] Chainsaw Man 1080p BD Dual Audio [JA]"} {
		rel := scored(t, profile, title)
		if rel.Acceptable || rel.RejectReason != "dual audio is required" {
			t.Fatalf("unrelated bonus or conflicting JA marker satisfied required: %+v", rel)
		}
	}
	profile.ScoringConfig = `{"preset":"sonarr-anime","dual_audio":"required"}`
	for _, label := range []string{"DUAL", "Dual Audio", "[Dual]"} {
		rel := scored(t, profile, "[FLE] Chainsaw Man 1080p WEB-DL "+label)
		if !rel.Acceptable || !hasFormat(rel, "Anime Dual Audio") {
			t.Fatalf("upstream-supported dual label rejected: %s %+v", label, rel)
		}
	}
	rel := Release{Title: "[FLE] Chainsaw Man 1080p BD Dual Audio", Languages: []string{"en"}}
	ScoreRelease(&rel, profile)
	if rel.Acceptable {
		t.Fatal("explicit English-only audio overwritten with inferred Japanese")
	}
}

func TestTRaSHValidationAndExplicitZeroOverrides(t *testing.T) {
	for _, raw := range []string{"", "{}"} {
		config, err := ResolveScoringConfig(raw)
		if err != nil || config.Preset != "" {
			t.Fatal("legacy config rejected")
		}
	}
	config, err := ResolveScoringConfig(`{"preset":"sonarr-anime","minimum_format_score":0,"upgrade_until_format_score":0,"format_scores":{"949c16fe0a8147f50ba82cc2df9411c9":0}}`)
	if err != nil || config.MinimumFormatScore != 0 || config.UpgradeUntilFormatScore != 0 || config.FormatScores["949c16fe0a8147f50ba82cc2df9411c9"] != 0 {
		t.Fatalf("zero overridden: %+v %v", config, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"preset":"wrong"}`, `{"preset":"sonarr-anime","typo":true}`, `{"preset":"sonarr-anime","minimum_format_score":null}`, `{"preset":"sonarr-anime","format_scores":{"unknown":1}}`, `{"preset":"sonarr-anime","quality_groups":[["web-1080p","web-1080p"]]}`, `{"preset":"sonarr-anime","quality_groups":[]}`, `{"preset":"sonarr-anime","dual_audio":"maybe"}`} {
		if ValidateScoringConfig(raw) == nil {
			t.Fatalf("invalid config accepted: %s", raw)
		}
	}
}

func TestTRaSHNamedAudioPreferenceRoundTripDoesNotPersistBonus(t *testing.T) {
	config, err := ResolveScoringConfig(`{"preset":"sonarr-anime","dual_audio":"required"}`)
	if err != nil {
		t.Fatal(err)
	}
	if config.FormatScores["418f50b10f1907201b6cfdf881f467b7"] != 0 {
		t.Fatal("named preference mutated stored score")
	}
	config.DualAudio = "optional"
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	profile := presetProfile(t, "sonarr-anime")
	profile.ScoringConfig = string(encoded)
	rel := scored(t, profile, "[FLE] Chainsaw Man 1080p BD Dual Audio")
	if rel.FormatScore != 1400 {
		t.Fatalf("required bonus persisted after optional roundtrip: %+v", rel)
	}
}

func TestTRaSHBoundedRegexFailsClosed(t *testing.T) {
	var format trashFormat
	json.Unmarshal([]byte(`{"specifications":[{"name":"slow","implementation":"ReleaseTitleSpecification","negate":true,"fields":{"value":"^(a+)+$"}}]}`), &format)
	if err := compileTRaSHSpec(&format.Specifications[0]); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	matched, err := matchesTRaSHFormat(format, formatInput{title: strings.Repeat("a", 100) + "!"})
	if err == nil || matched || time.Since(started) > 2*time.Second {
		t.Fatalf("regex timeout not enforced: matched=%t err=%v", matched, err)
	}
	profile := presetProfile(t, "sonarr-anime")
	rel := scored(t, profile, strings.Repeat("a", maxScoringTitleBytes+1))
	if rel.Acceptable || !strings.Contains(rel.RejectReason, "too long") {
		t.Fatal("oversized title accepted")
	}
}

func TestLegacyScoringRemainsUnchangedWithoutPreset(t *testing.T) {
	profile := &QualityProfile{Qualities: `["bluray-1080p","web-1080p"]`, Tags: `{"dual-audio":250}`, Language: "any", ScoringConfig: `{}`, UpgradeAllowed: true}
	bd := Release{Title: "Film BD 1080p", Quality: "bluray-1080p"}
	web := Release{Title: "Film WEB 1080p Dual Audio", Quality: "web-1080p", Tags: []string{"dual-audio"}}
	ScoreRelease(&bd, profile)
	ScoreRelease(&web, profile)
	if bd.Score != 200 || web.Score != 350 || CompareReleases(&web, &bd) <= 0 || !IsUpgrade(&web, &bd, profile) {
		t.Fatalf("legacy formula changed: %+v %+v", bd, web)
	}
}
