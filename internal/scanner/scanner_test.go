package scanner

import (
	"mediaforge/internal/database"
	"testing"
)

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
