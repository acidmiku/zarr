package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func TestScoringMigrationPreservesCustomizedProfilesAndAssignments(t *testing.T) {
	dir := t.TempDir()
	old, err := sql.Open("sqlite3", filepath.Join(dir, "mediaforge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if _, err := old.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil || version > 10 {
			continue
		}
		contents, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := old.Exec(string(contents)); err != nil {
			t.Fatalf("migration %d: %v", version, err)
		}
		if _, err := old.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := old.Exec(`UPDATE quality_profiles SET name='My Anime', qualities='["web-1080p"]', tags='{"dual-audio":999}', upgrade_allowed=0 WHERE id=3;
		INSERT INTO media_items(type,title,anime,quality_profile_id,status) VALUES('series','Existing collection',1,3,'available')`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name, qualities, tags, config string
	var upgrade bool
	if err := db.QueryRow(`SELECT name,qualities,tags,upgrade_allowed,scoring_config FROM quality_profiles WHERE id=3`).Scan(&name, &qualities, &tags, &upgrade, &config); err != nil {
		t.Fatal(err)
	}
	if name != "My Anime" || qualities != `["web-1080p"]` || tags != `{"dual-audio":999}` || upgrade || config != "{}" {
		t.Fatal("migration changed a user profile")
	}
	var assigned int
	if err := db.QueryRow(`SELECT quality_profile_id FROM media_items WHERE title='Existing collection'`).Scan(&assigned); err != nil || assigned != 3 {
		t.Fatal("migration reassigned library")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM quality_profiles WHERE scoring_config!='{}'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("preset count=%d err=%v", count, err)
	}
	// Reopening cannot create duplicate presets or reset custom scores.
	db.Close()
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT count(*) FROM quality_profiles WHERE scoring_config!='{}'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("reopen preset count=%d err=%v", count, err)
	}
}
