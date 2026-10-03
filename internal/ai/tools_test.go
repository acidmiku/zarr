package ai

import (
	"io"
	"mediaforge/internal/database"
	"mediaforge/internal/metadata"
	"net/http"
	"strings"
	"testing"
)

func TestMusicRatingDoesNotEnqueueOrCreateAlbum(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mb := metadata.NewMusicBrainzClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"release-groups":[{"id":"rg","title":"Album","artist-credit":[{"artist":{"id":"artist","name":"Artist"}}]}]}`))}, nil
	})})
	e := ToolExecutor{DB: db.DB, MusicBrainz: mb}
	result, _, err := e.ExecuteTool("save_ratings", `{"ratings":[{"title":"Album","media_type":"music","score":4}]}`)
	if err != nil || !strings.Contains(result, "add this album") {
		t.Fatalf("result=%s err=%v", result, err)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM albums`).Scan(&count)
	if count != 0 {
		t.Fatal("rating created a download target")
	}
	db.Exec(`INSERT INTO artists(id,name) VALUES (1,'Artist')`)
	db.Exec(`INSERT INTO albums(artist_id,title,rating) VALUES (1,'Legacy',999)`)
	if _, _, err := e.ExecuteTool("get_user_ratings", `{"type":"music"}`); err != nil {
		t.Fatal(err)
	}
	result, _, err = e.ExecuteTool("save_ratings", `{"ratings":[{"title":"Album","media_type":"music","score":9}]}`)
	if err != nil || !strings.Contains(result, "rating must be 1-5") {
		t.Fatalf("invalid score was silently coerced: %s", result)
	}
}
