package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaforge/internal/metadata"
)

func TestManualSeriesImportRejectsCombinedEpisodesBeforeAnyWrites(t *testing.T) {
	for _, filename := range []string{"Chainsaw Man S01E01E02.mkv", "Chainsaw Man S01E01-02.mkv", "Chainsaw Man 1x01-02.mkv", "Chainsaw Man 01v2-03.mkv", "01-02.mkv"} {
		t.Run(filename, func(t *testing.T) {
			s := newConnectionTestServer(t)
			s.cfg.MediaRoot = t.TempDir()
			s.tmdb = metadata.NewTMDBClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/3/tv/100" {
					return nil, fmt.Errorf("unexpected endpoint %s", r.URL.Path)
				}
				return animeTestResponse(`{"id":100,"name":"Chainsaw Man","genres":[{"id":16}],"origin_country":["JP"]}`), nil
			})}, "test")
			source := t.TempDir()
			for _, name := range []string{filename, "Chainsaw Man S01E03.mkv"} {
				if err := os.WriteFile(filepath.Join(source, name), []byte("video"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result := s.importSeries(importItem{SourcePath: source, TMDBID: 100, Type: "series", QualityProfileID: 1}, true)
			if result.Status != "error" || !strings.Contains(result.Message, "combined episode files") {
				t.Fatalf("unsafe import: %+v", result)
			}
			var mediaCount int
			if err := s.db.QueryRow(`SELECT count(*) FROM media_items`).Scan(&mediaCount); err != nil {
				t.Fatal(err)
			}
			if mediaCount != 0 {
				t.Fatal("created library entry before preflight completed")
			}
			for _, name := range []string{filename, "Chainsaw Man S01E03.mkv"} {
				if data, err := os.ReadFile(filepath.Join(source, name)); err != nil || string(data) != "video" {
					t.Fatalf("source changed: %s %v", name, err)
				}
			}
		})
	}
}
