package postprocess

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Build a minimal FLAC metadata stream, so the production parser handles real
// Vorbis-comment framing rather than mocked tag objects.
func writeTaggedFLAC(t *testing.T, path string, tags map[string]string) {
	t.Helper()
	var block bytes.Buffer
	binary.Write(&block, binary.LittleEndian, uint32(4))
	block.WriteString("test")
	binary.Write(&block, binary.LittleEndian, uint32(len(tags)))
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := key + "=" + tags[key]
		binary.Write(&block, binary.LittleEndian, uint32(len(value)))
		block.WriteString(value)
	}
	n := block.Len()
	data := append([]byte{'f', 'L', 'a', 'C', 0x84, byte(n >> 16), byte(n >> 8), byte(n)}, block.Bytes()...)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func taggedAlbum(t *testing.T) (MusicImport, string) {
	t.Helper()
	input := MusicImport{ArtistMBID: "artist-id", ArtistName: "Artist", ReleaseGroupID: "group-id", ReleaseID: "release-id", Title: "Album", AlbumType: "album", Tracks: []MusicImportTrack{{RecordingID: "rec-1", Title: "One", Number: 1, DiscNumber: 1}, {RecordingID: "rec-2", Title: "Two", Number: 1, DiscNumber: 2}}}
	dir := t.TempDir()
	for i, track := range input.Tracks {
		writeTaggedFLAC(t, filepath.Join(dir, fmt.Sprintf("obfuscated-%d.flac", i)), map[string]string{"TITLE": track.Title, "ARTIST": "Artist", "ALBUM": "Album", "TRACKNUMBER": "1", "DISCNUMBER": fmt.Sprint(track.DiscNumber), "MUSICBRAINZ_ALBUMARTISTID": "artist-id", "MUSICBRAINZ_ALBUMID": "release-id", "MUSICBRAINZ_RELEASEGROUPID": "group-id", "MUSICBRAINZ_TRACKID": track.RecordingID})
	}
	return input, dir
}

func TestTaggedMultiDiscManualImportUsesMetadataAndTransaction(t *testing.T) {
	db := testDB(t)
	input, source := taggedAlbum(t)
	root := t.TempDir()
	id, err := New(db, root, "").ImportMusic(source, input)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM tracks WHERE album_id=? AND status='available'`, id).Scan(&count)
	if count != 2 {
		t.Fatal(count)
	}
	for _, track := range input.Tracks {
		path := MusicTrackPath(root, "Artist", "Album", 0, track.Number, track.DiscNumber, track.Title, ".flac")
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := findAudioFiles(source)
	if len(files) != 0 {
		t.Fatal("successful manual import did not move sources")
	}
}

func TestTaggedManualImportRejectsConflictsWithoutPartialWrites(t *testing.T) {
	for _, test := range []struct{ field, value string }{{"ARTIST", "Wrong Artist"}, {"ALBUM", "Wrong Album"}, {"TITLE", "Wrong Song"}, {"DISCNUMBER", "1"}, {"MUSICBRAINZ_RELEASEGROUPID", "wrong-group"}, {"MUSICBRAINZ_TRACKID", "wrong-recording"}, {"ALBUM", "Album Deluxe Edition"}} {
		t.Run(test.field+test.value, func(t *testing.T) {
			db := testDB(t)
			input, source := taggedAlbum(t)
			root := t.TempDir()
			tags := map[string]string{"TITLE": "Two", "ARTIST": "Artist", "ALBUM": "Album", "TRACKNUMBER": "1", "DISCNUMBER": "2", "MUSICBRAINZ_RELEASEGROUPID": "group-id", "MUSICBRAINZ_TRACKID": "rec-2"}
			tags[test.field] = test.value
			writeTaggedFLAC(t, filepath.Join(source, "obfuscated-1.flac"), tags)
			if _, err := New(db, root, "").ImportMusic(source, input); err == nil {
				t.Fatal("conflicting tags accepted")
			}
			for _, table := range []string{"artists", "albums", "tracks"} {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
				if count != 0 {
					t.Fatalf("partial %s left", table)
				}
			}
			files, _ := findAudioFiles(source)
			if len(files) != 2 {
				t.Fatal("wrong source was partially moved")
			}
			dest, _ := findAudioFiles(root)
			if len(dest) != 0 {
				t.Fatal("failed import wrote library files")
			}
		})
	}
}

func TestMusicManualImportDatabaseFailureRestoresSourcesAndRows(t *testing.T) {
	db := testDB(t)
	input, source := taggedAlbum(t)
	root := t.TempDir()
	execute(t, db, `CREATE TRIGGER fail_track BEFORE INSERT ON tracks WHEN NEW.title='Two' BEGIN SELECT RAISE(ABORT,'test failure');END`)
	if _, err := New(db, root, "").ImportMusic(source, input); err == nil {
		t.Fatal("DB failure ignored")
	}
	for _, table := range []string{"artists", "albums", "tracks"} {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if count != 0 {
			t.Fatal("database transaction leaked " + table)
		}
	}
	files, _ := findAudioFiles(source)
	if len(files) != 2 {
		t.Fatal("sources lost on DB failure")
	}
	files, _ = findAudioFiles(root)
	if len(files) != 0 {
		t.Fatal("installed files not rolled back")
	}
}

func TestUntaggedMusicRequiresTrackTitleAndUnambiguousDisc(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{{"01 - Song.flac", true}, {"01 - Artist - Song.flac", true}, {"Song.flac", true}, {"01 - Wrong Song.flac", false}, {"01.flac", false}} {
		t.Run(test.name, func(t *testing.T) {
			db := testDB(t)
			execute(t, db, `INSERT INTO artists(id,name) VALUES(1,'Artist')`)
			execute(t, db, `INSERT INTO albums(id,artist_id,title) VALUES(1,1,'Album')`)
			execute(t, db, `INSERT INTO tracks(id,album_id,number,title) VALUES(1,1,1,'Song')`)
			execute(t, db, `INSERT INTO downloads(id,album_id,nzb_title) VALUES(1,1,'Album')`)
			src := filepath.Join(t.TempDir(), test.name)
			writeMedia(t, src, "untagged test data")
			err := New(db, t.TempDir(), "").ProcessTorrent(1, src)
			if (err == nil) != test.want {
				t.Fatalf("err=%v", err)
			}
			if _, err := os.Stat(src); err != nil {
				t.Fatal("torrent source removed")
			}
		})
	}
}

func TestIncompleteTaggedAlbumRetainsAllSources(t *testing.T) {
	db := testDB(t)
	input, source := taggedAlbum(t)
	os.Remove(filepath.Join(source, "obfuscated-1.flac"))
	if _, err := New(db, t.TempDir(), "").ImportMusic(source, input); err == nil {
		t.Fatal("partial tagged album accepted")
	}
	if _, err := os.Stat(filepath.Join(source, "obfuscated-0.flac")); err != nil {
		t.Fatal("partial album source moved")
	}
}

func TestEquivalentCountryEditionAcceptedWithRecordingEvidence(t *testing.T) {
	db := testDB(t)
	input, source := taggedAlbum(t)
	for i, track := range input.Tracks {
		writeTaggedFLAC(t, filepath.Join(source, fmt.Sprintf("obfuscated-%d.flac", i)), map[string]string{"TITLE": track.Title, "ARTIST": "Artist", "ALBUM": "Album", "TRACKNUMBER": "1", "DISCNUMBER": fmt.Sprint(track.DiscNumber), "MUSICBRAINZ_ALBUMID": "alternate-country-release", "MUSICBRAINZ_TRACKID": track.RecordingID})
	}
	if _, err := New(db, t.TempDir(), "").ImportMusic(source, input); err != nil {
		t.Fatal(err)
	}
}

func TestStrictLosslessChecksActualContainerNotFileExtension(t *testing.T) {
	db := testDB(t)
	input, source := taggedAlbum(t)
	res, err := db.Exec(`INSERT INTO quality_profiles(name,qualities,tags,language,reject_patterns,profile_type) VALUES('Strict lossless','["flac"]','{}','any','[]','music')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	input.QualityProfileID = int(id)
	var frames bytes.Buffer
	for _, frame := range []struct{ id, value string }{{"TIT2", "One"}, {"TALB", "Album"}, {"TPE1", "Artist"}, {"TRCK", "1"}, {"TPOS", "1"}} {
		frames.WriteString(frame.id)
		binary.Write(&frames, binary.BigEndian, uint32(len(frame.value)+1))
		frames.Write([]byte{0, 0, 0})
		frames.WriteString(frame.value)
	}
	n := frames.Len()
	data := append([]byte{'I', 'D', '3', 3, 0, 0, byte(n>>21) & 127, byte(n>>14) & 127, byte(n>>7) & 127, byte(n) & 127}, frames.Bytes()...)
	if err := os.WriteFile(filepath.Join(source, "obfuscated-0.flac"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(db, t.TempDir(), "").ImportMusic(source, input); err == nil {
		t.Fatal("MP3 in a file named .flac bypassed lossless policy")
	}
	files, _ := findAudioFiles(source)
	if len(files) != 2 {
		t.Fatal("codec rejection moved source")
	}
}

func TestMalformedRecognizedTagsDoNotUseFilenameFallback(t *testing.T) {
	db := testDB(t)
	input, _ := taggedAlbum(t)
	input.Tracks = input.Tracks[:1]
	src := filepath.Join(t.TempDir(), "01 - One.flac")
	writeMedia(t, src, "fLaC corrupted metadata")
	if _, err := New(db, t.TempDir(), "").ImportMusic(src, input); err == nil {
		t.Fatal("malformed FLAC accepted by filename fallback")
	}
}
