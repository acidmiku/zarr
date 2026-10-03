package scanner

import (
	"fmt"
	"mediaforge/internal/database"
	"mediaforge/internal/indexer"
	"os"
	"path/filepath"
	"testing"
)

func TestScanInvalidatesStaleReleaseProvenanceIncludingSamePathReplacements(t *testing.T) {
	for _, kind := range []string{"movie", "tv", "anime"} {
		for _, samePath := range []bool{false, true} {
			for _, hadTitle := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/same-path=%t/stored-title=%t", kind, samePath, hadTitle), func(t *testing.T) {
					db, err := database.Open(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					root := t.TempDir()
					path := filepath.Join(root, "movies", "Film (2025)", "Film (2025).mkv")
					mediaType, title := "movie", "Film"
					if kind != "movie" {
						mediaType, title = "series", "Show"
						folder := "Show (2025)"
						if kind == "anime" {
							folder = "Show"
						}
						path = filepath.Join(root, kind, folder, "Season 01", "Show - S01E01 - Episode.mkv")
					}
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("old WEB 720p file"), 0600); err != nil {
						t.Fatal(err)
					}
					oldPath := path
					if kind == "movie" {
						oldPath = filepath.Dir(path)
					}
					if !samePath {
						oldPath = filepath.Join(root, "previous-import-location")
					}
					oldTitle := "[FLE] " + title + " 720p WEB-DL"
					var storedTitle any
					if hadTitle {
						storedTitle = oldTitle
					}
					if _, err := db.Exec(`INSERT INTO media_items(id,type,title,year,anime,status,root_path,current_release_title) VALUES(1,?,?,2025,?,'available',?,?)`, mediaType, title, kind == "anime", oldPath, storedTitle); err != nil {
						t.Fatal(err)
					}
					var episodeID any
					if kind != "movie" {
						if _, err := db.Exec(`INSERT INTO seasons(id,media_item_id,number) VALUES(1,1,1)`); err != nil {
							t.Fatal(err)
						}
						if _, err := db.Exec(`INSERT INTO episodes(id,season_id,media_item_id,number,status,file_path,current_release_title) VALUES(1,1,1,1,'available',?,?)`, oldPath, storedTitle); err != nil {
							t.Fatal(err)
						}
						episodeID = 1
					}
					if _, err := db.Exec(`INSERT INTO downloads(media_item_id,episode_id,nzb_title,status) VALUES(1,?,?,'imported')`, episodeID, oldTitle); err != nil {
						t.Fatal(err)
					}
					// Simulate an external replacement; unchanged paths provide no
					// identity proof either. The scanner must not trust old history.
					newContent := []byte("externally replaced with higher quality BD 1080p")
					if err := os.WriteFile(path, newContent, 0600); err != nil {
						t.Fatal(err)
					}
					s := New(db, root)
					for scan := 0; scan < 2; scan++ {
						result, err := s.Scan()
						if err != nil || result.FilesMatched != 1 {
							t.Fatalf("scan=%+v err=%v", result, err)
						}
					}
					var baseline, status, matchedPath string
					query := `SELECT COALESCE(NULLIF(m.current_release_title,''),(SELECT nzb_title FROM downloads WHERE media_item_id=m.id ORDER BY id DESC LIMIT 1),''),status,root_path FROM media_items m WHERE id=1`
					wantPath := filepath.Dir(path)
					if kind != "movie" {
						query = `SELECT COALESCE(NULLIF(e.current_release_title,''),(SELECT nzb_title FROM downloads WHERE episode_id=e.id ORDER BY id DESC LIMIT 1),''),status,file_path FROM episodes e WHERE id=1`
						wantPath = path
					}
					if err := db.QueryRow(query).Scan(&baseline, &status, &matchedPath); err != nil {
						t.Fatal(err)
					}
					if baseline != "manual-grab" || indexer.ParseReleaseName(baseline).Quality != "" {
						t.Fatalf("stale upgrade baseline survived: %q", baseline)
					}
					if status != "available" || matchedPath != wantPath {
						t.Fatalf("scan lost available file: %s %s", status, matchedPath)
					}
					if content, err := os.ReadFile(path); err != nil || string(content) != string(newContent) {
						t.Fatal("scan changed external replacement")
					}
				})
			}
		}
	}
}

func TestScannerMatchesExactSanitizedTitlesAndYear(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`INSERT INTO media_items(id,type,title,year) VALUES(1,'movie','Aliens',1979),(2,'movie','Alien',1979),(3,'series','Show: New',2000),(4,'series','Show: New',2020)`)
	s := New(db, t.TempDir())
	id, err := s.findMedia("Alien", "movie", false, 1979)
	if err != nil || id != 2 {
		t.Fatalf("matched wrong movie: %d %v", id, err)
	}
	id, err = s.findMedia("Show New", "series", false, 2020)
	if err != nil || id != 4 {
		t.Fatalf("matched wrong show: %d %v", id, err)
	}
	if _, err = s.findMedia("Show New", "series", false, 0); err == nil {
		t.Fatal("ambiguous remake matched arbitrarily")
	}
}

func TestScannerMovieTypeIncludesAnimeMovies(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`INSERT INTO media_items(id,type,title,year,anime) VALUES(1,'series','Chainsaw Man',2022,1),(2,'movie','Chainsaw Man The Movie Reze Arc',2025,1)`)
	s := New(db, t.TempDir())
	id, err := s.findMedia("Chainsaw Man The Movie Reze Arc", "movie", false, 2025)
	if err != nil || id != 2 {
		t.Fatalf("anime movie wasn't scanned as movie: %d %v", id, err)
	}
}
