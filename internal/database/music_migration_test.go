package database

import (
	"database/sql"
	"strings"
	"testing"
)

func TestMusicMigrationPreservesExistingIntentButNewSavesAreUnmonitored(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.Compare(file.Name(), "012_") >= 0 {
			continue
		}
		content, _ := migrationsFS.ReadFile("migrations/" + file.Name())
		if _, err = db.Exec(string(content)); err != nil {
			t.Fatal(file.Name(), err)
		}
	}
	if _, err = db.Exec(`INSERT INTO artists(id,name) VALUES(1,'Artist');INSERT INTO albums(id,artist_id,title,status) VALUES(1,1,'Wanted','wanted'),(2,1,'Owned','available')`); err != nil {
		t.Fatal(err)
	}
	migration, _ := migrationsFS.ReadFile("migrations/012_music_catalog_requests.sql")
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO albums(id,artist_id,title) VALUES(3,1,'New save')`); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]int{1: 1, 2: 1, 3: 0} {
		var got int
		if err = db.QueryRow(`SELECT monitored FROM albums WHERE id=?`, id).Scan(&got); err != nil || got != want {
			t.Fatalf("album%d monitored=%d want%d error%v", id, got, want, err)
		}
	}
}
