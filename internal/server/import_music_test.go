package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaforge/internal/metadata"
	"mediaforge/internal/postprocess"
)

func musicImportFixture(t *testing.T, path, title, album string, number int) {
	t.Helper()
	tags := []string{"TITLE=" + title, "ARTIST=Canonical Artist", "ALBUM=" + album, fmt.Sprintf("TRACKNUMBER=%d", number), "DISCNUMBER=1"}
	var block bytes.Buffer
	binary.Write(&block, binary.LittleEndian, uint32(4))
	block.WriteString("test")
	binary.Write(&block, binary.LittleEndian, uint32(len(tags)))
	for _, v := range tags {
		binary.Write(&block, binary.LittleEndian, uint32(len(v)))
		block.WriteString(v)
	}
	n := block.Len()
	data := append([]byte{'f', 'L', 'a', 'C', 0x84, byte(n >> 16), byte(n >> 8), byte(n)}, block.Bytes()...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestManualMusicImportUsesCanonicalIdentityAndRejectsWrongAlbumBeforeMoving(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(fmt.Sprint(wrong), func(t *testing.T) {
			db := downloadsTestDB(t)
			root, source := t.TempDir(), t.TempDir()
			client := &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
				var body string
				if strings.Contains(r.URL.Path, "release-group/") {
					body = `{"id":"group","title":"Canonical Album","primary-type":"Album","artist-credit":[{"artist":{"id":"artist","name":"Canonical Artist"}}]}`
				} else {
					body = `{"id":"release","title":"Canonical Album","release-group":{"id":"group"},"media":[{"position":1,"track-count":2,"tracks":[{"position":1,"title":"One","recording":{"id":"rec1"}},{"position":2,"title":"Two","recording":{"id":"rec2"}}]}]}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			s := &Server{db: db, musicbrainz: metadata.NewMusicBrainzClient(client), processor: postprocess.New(db, root, "")}
			musicImportFixture(t, filepath.Join(source, "one.flac"), "One", "Canonical Album", 1)
			album := "Canonical Album"
			if wrong {
				album = "Wrong Album"
			}
			musicImportFixture(t, filepath.Join(source, "two.flac"), "Two", album, 2)
			result := s.importMusic(importItem{SourcePath: source, ReleaseGroupID: "group", ReleaseID: "release", ArtistName: "Untrusted Artist Label", ArtistMBID: "untrusted-id", AlbumTitle: "Untrusted Album Label"})
			if wrong {
				if result.Status != "error" {
					t.Fatal(result)
				}
				for _, table := range []string{"artists", "albums", "tracks"} {
					var count int
					db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
					if count != 0 {
						t.Fatal("partial metadata left: " + table)
					}
				}
				for _, name := range []string{"one.flac", "two.flac"} {
					if _, err := os.Stat(filepath.Join(source, name)); err != nil {
						t.Fatal("partial source move")
					}
				}
			} else {
				if result.Status != "imported" {
					t.Fatal(result)
				}
				var title, artist string
				db.QueryRow(`SELECT al.title,a.name FROM albums al JOIN artists a ON a.id=al.artist_id WHERE al.id=?`, result.LibraryID).Scan(&title, &artist)
				if title != "Canonical Album" || artist != "Canonical Artist" {
					t.Fatal("client labels replaced canonical metadata")
				}
			}
		})
	}
}
