package database

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentFreshConnectionsAndSettingsWrites(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var workers sync.WaitGroup
	errors := make(chan error, 32)
	for i := 0; i < 24; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			for j := 0; j < 15; j++ {
				if index%3 == 0 {
					if err := db.SetSetting(fmt.Sprint("stress_", index), fmt.Sprint(j)); err != nil {
						errors <- err
						return
					}
				} else {
					rows, err := db.Query(`SELECT id,name FROM quality_profiles`)
					if err != nil {
						errors <- err
						return
					}
					for rows.Next() {
						var id int
						var name string
						if err := rows.Scan(&id, &name); err != nil {
							rows.Close()
							errors <- err
							return
						}
					}
					err = rows.Err()
					rows.Close()
					if err != nil {
						errors <- err
						return
					}
				}
			}
		}(i)
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestMigrationsEnforceIdentityAndForeignKeys(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO media_items(type,title,anilist_id) VALUES ('series','Anime',10)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO media_items(type,title,anilist_id) VALUES ('series','Duplicate',10)`); err == nil {
		t.Fatal("duplicate AniList identity accepted")
	}
	db.Exec(`INSERT INTO artists(id,name,mbid) VALUES (1,'Artist','artist')`)
	if _, err = db.Exec(`INSERT INTO albums(artist_id,title,release_group_id) VALUES (1,'Album','release')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO albums(artist_id,title,release_group_id) VALUES (1,'Duplicate','release')`); err == nil {
		t.Fatal("duplicate release group accepted")
	}
	if _, err = db.Exec(`INSERT INTO ai_messages(session_id,role,content) VALUES (999,'user','orphan')`); err == nil {
		t.Fatal("foreign key disabled")
	}
	db.Close()
	db, err = Open(dir)
	if err != nil {
		t.Fatal("migration reopen failed", err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM albums`).Scan(&count); err != nil || count != 1 {
		t.Fatal("reopen lost data")
	}
}
