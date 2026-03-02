package indexer

import (
	"encoding/json"
)

// QualityProfile represents a quality profile from the database.
type QualityProfile struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Qualities      string `json:"qualities"`       // JSON array
	Tags           string `json:"tags"`             // JSON object
	Language       string `json:"language"`
	RejectPatterns string `json:"reject_patterns"`  // JSON array
	UpgradeAllowed bool   `json:"upgrade_allowed"`
	ProfileType    string `json:"profile_type"`
}

// Release represents a found release with computed score.
type Release struct {
	Title        string   `json:"title"`
	NZBURL       string   `json:"nzb_url"`
	Size         int64    `json:"size"`
	Quality      string   `json:"quality"`
	Tags         []string `json:"tags"`
	Score        int      `json:"score"`
	Indexer      string   `json:"indexer"`
	Category     string   `json:"category"`
	Acceptable   bool     `json:"acceptable"`
	RejectReason string   `json:"reject_reason,omitempty"`
	DownloadType string   `json:"download_type"`           // "nzb" or "torrent"
	Seeders      int      `json:"seeders,omitempty"`
	Leechers     int      `json:"leechers,omitempty"`
	TopicID      int      `json:"topic_id,omitempty"`      // Rutracker topic ID
}

// ScoreRelease computes a score for a release against a quality profile.
// Returns the release with score set, or acceptable=false if rejected.
func ScoreRelease(rel *Release, profile *QualityProfile) {
	// Parse quality profile
	var qualities []string
	if err := json.Unmarshal([]byte(profile.Qualities), &qualities); err != nil {
		rel.Acceptable = false
		rel.RejectReason = "invalid quality profile"
		return
	}

	var tagBonuses map[string]int
	if profile.Tags != "" {
		json.Unmarshal([]byte(profile.Tags), &tagBonuses)
	}

	var rejectPatterns []string
	if profile.RejectPatterns != "" {
		json.Unmarshal([]byte(profile.RejectPatterns), &rejectPatterns)
	}

	// Check reject patterns
	if MatchesRejectPatterns(rel.Title, rejectPatterns) {
		rel.Acceptable = false
		rel.RejectReason = "matches reject pattern"
		return
	}

	// Check language (skip for music profiles with language "any")
	if profile.Language != "any" && !LanguageAcceptable(rel.Title, profile.Language) {
		rel.Acceptable = false
		rel.RejectReason = "language mismatch"
		return
	}

	// Check if quality is in profile
	qualityIndex := -1
	for i, q := range qualities {
		if q == rel.Quality {
			qualityIndex = i
			break
		}
	}

	if qualityIndex == -1 {
		// Music releases often lack quality info in the title — accept with low score
		if profile.ProfileType == "music" && rel.Quality == "" {
			rel.Quality = "unknown"
			rel.Score = 1
			rel.Acceptable = true
			return
		}
		rel.Acceptable = false
		rel.RejectReason = "quality not in profile"
		return
	}

	// Base score: higher for qualities listed first
	baseScore := (len(qualities) - qualityIndex) * 100

	// Tag bonuses
	bonus := 0
	for _, tag := range rel.Tags {
		if b, ok := tagBonuses[tag]; ok {
			bonus += b
		}
	}

	rel.Score = baseScore + bonus
	rel.Acceptable = true
}

// BestRelease selects the highest-scoring acceptable release.
func BestRelease(releases []Release) *Release {
	var best *Release
	for i := range releases {
		if !releases[i].Acceptable {
			continue
		}
		if best == nil || releases[i].Score > best.Score {
			best = &releases[i]
		}
	}
	return best
}

// ShouldUpgrade checks if a new release should replace an existing one.
func ShouldUpgrade(newScore, existingScore int, upgradeAllowed bool) bool {
	return upgradeAllowed && newScore > existingScore
}
