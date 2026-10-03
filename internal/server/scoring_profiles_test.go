package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mediaforge/internal/indexer"
)

func TestScoringProfileDefaultsAndCompatibility(t *testing.T) {
	s := newConnectionTestServer(t)
	w := callDomain(s.handleListProfiles, "GET", "/", "", "", "")
	var profiles []struct {
		ID     int            `json:"id"`
		Name   string         `json:"name"`
		Config map[string]any `json:"scoring_config"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &profiles) != nil {
		t.Fatalf("profiles: %s", w.Body.String())
	}
	var newID int
	for _, p := range profiles {
		if p.Name == "Anime" && len(p.Config) != 0 {
			t.Fatal("legacy Anime profile changed")
		}
		if p.Config["preset"] == "sonarr-anime" {
			newID = p.ID
			if p.Config["minimum_format_score"] != float64(100) {
				t.Fatalf("wrong upstream minimum: %+v", p.Config)
			}
		}
	}
	if newID == 0 {
		t.Fatal("new preset missing")
	}
	// An old client editing the profile cannot erase advanced scoring.
	body := `{"name":"Edited anime profile","qualities":["web-1080p"],"upgrade_allowed":true}`
	w = callDomain(s.handleUpdateProfile, "PUT", "/", body, "id", fmt.Sprint(newID))
	if w.Code != 200 {
		t.Fatalf("legacy edit: %s", w.Body.String())
	}
	var config, qualities string
	if err := s.db.QueryRow(`SELECT scoring_config,qualities FROM quality_profiles WHERE id=?`, newID).Scan(&config, &qualities); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, `"preset":"sonarr-anime"`) || !strings.Contains(qualities, "remux-1080p") {
		t.Fatal("old client silently changed preset/quality groups")
	}
	// An explicit empty config really does select the legacy engine.
	body = `{"name":"Edited anime profile","qualities":["web-1080p"],"scoring_config":{}}`
	w = callDomain(s.handleUpdateProfile, "PUT", "/", body, "id", fmt.Sprint(newID))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s.db.QueryRow(`SELECT scoring_config FROM quality_profiles WHERE id=?`, newID).Scan(&config)
	if config != "{}" {
		t.Fatal("explicit legacy selection ignored")
	}
}

func TestReleaseAPIOrdersQualityBeforeFormatScore(t *testing.T) {
	s := newConnectionTestServer(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>
<item><title>[Arg0] Chainsaw Man The Movie Reze Arc 2025 1080p WEB-DL</title><link>https://example.invalid/web</link></item>
<item><title>[Unknown] Chainsaw Man The Movie Reze Arc 2025 1080p BluRay</title><link>https://example.invalid/bluray</link></item>
<item><title>[Arg0] Chainsaw Man The Movie Reze Arc 2025 1080p WEB-DL AV1</title><link>https://example.invalid/rejected</link></item>
</channel></rss>`)
	}))
	defer remote.Close()
	s.newznab = indexer.NewNewznabClient(remote.Client())
	if _, err := s.db.Exec(`UPDATE quality_profiles SET scoring_config='{"preset":"radarr-anime","minimum_format_score":0}' WHERE name='Anime Movies · TRaSH'`); err != nil {
		t.Fatal(err)
	}
	id, err := s.resolveMediaProfile(0, "movie", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO media_items(id,type,title,year,anime,imdb_id,quality_profile_id) VALUES(1,'movie','Chainsaw Man The Movie Reze Arc',2025,TRUE,'tt1234567',?)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO indexers(name,url,api_key,type,content_types) VALUES('Fixture',?,'test','newznab','["anime"]')`, remote.URL); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handleSearchReleases(w, httptest.NewRequest("GET", "/api/releases?media_item_id=1", nil))
	var releases []indexer.Release
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &releases) != nil || len(releases) != 3 {
		t.Fatalf("release response: %d %s", w.Code, w.Body.String())
	}
	if releases[0].Quality != "bluray-1080p" || !releases[0].Acceptable || releases[1].FormatScore <= releases[0].FormatScore {
		t.Fatalf("quality grouping did not survive API ordering: %+v", releases)
	}
	if releases[2].Acceptable || !strings.Contains(releases[2].RejectReason, "minimum") {
		t.Fatalf("negative-score rejection missing: %+v", releases[2])
	}
	if len(releases[1].MatchedFormats) == 0 || releases[1].ScoringVersion == "" {
		t.Fatal("release explanation metadata missing")
	}
}

func TestScoringProfileValidationAndPresetAPI(t *testing.T) {
	s := newConnectionTestServer(t)
	for _, config := range []string{`null`, `[]`, `{"preset":"unknown"}`, `{"preset":"sonarr-anime","minimum_format_score":"zero"}`, `{"preset":"sonarr-anime","format_scores":{"unknown-id":1}}`} {
		body := fmt.Sprintf(`{"name":"Invalid","qualities":["web-1080p"],"scoring_config":%s}`, config)
		w := callDomain(s.handleCreateProfile, "POST", "/", body, "", "")
		if w.Code != 400 {
			t.Fatalf("accepted config %s: %d %s", config, w.Code, w.Body.String())
		}
	}
	w := callDomain(s.handleCreateProfile, "POST", "/", `{"name":"New film preset","scoring_config":{"preset":"radarr-anime","minimum_format_score":0},"upgrade_allowed":true}`, "", "")
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var config string
	s.db.QueryRow(`SELECT scoring_config FROM quality_profiles WHERE name='New film preset'`).Scan(&config)
	var fields map[string]any
	json.Unmarshal([]byte(config), &fields)
	if fields["minimum_format_score"] != float64(0) {
		t.Fatal("explicit zero overridden by default")
	}
	w = callDomain(s.handleCreateProfile, "POST", "/", `{"name":"Bad music preset","profile_type":"music","scoring_config":{"preset":"radarr-anime"}}`, "", "")
	if w.Code != 400 {
		t.Fatal("anime scoring accepted for music")
	}
	w = callDomain(s.handleProfilePresets, "GET", "/", "", "", "")
	var presets []map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &presets) != nil || len(presets) != 2 {
		t.Fatalf("preset catalog: %s", w.Body.String())
	}
	for _, preset := range presets {
		if preset["version"] == "" || preset["source_url"] == "" || preset["config"] == nil || preset["formats"] == nil {
			t.Fatalf("incomplete preset metadata: %+v", preset)
		}
	}
}

func TestNewAnimeDefaultsDoNotReassignExistingLibrary(t *testing.T) {
	s := newConnectionTestServer(t)
	if _, err := s.db.Exec(`INSERT INTO media_items(type,title,anime,quality_profile_id) VALUES('series','Existing',TRUE,3)`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"movie", "series"} {
		id, err := s.resolveMediaProfile(0, kind, true)
		if err != nil {
			t.Fatal(err)
		}
		var config string
		s.db.QueryRow(`SELECT scoring_config FROM quality_profiles WHERE id=?`, id).Scan(&config)
		wanted := "sonarr-anime"
		if kind == "movie" {
			wanted = "radarr-anime"
		}
		if !strings.Contains(config, wanted) {
			t.Fatalf("%s wrong preset: %s", kind, config)
		}
		id, err = s.resolveMediaProfile(3, kind, true)
		if err != nil || id != 3 {
			t.Fatal("explicit legacy choice ignored")
		}
	}
	var id int
	s.db.QueryRow(`SELECT quality_profile_id FROM media_items WHERE title='Existing'`).Scan(&id)
	if id != 3 {
		t.Fatal("existing assignment changed")
	}
}
