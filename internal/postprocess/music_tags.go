package postprocess

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
	"github.com/dhowden/tag/mbz"
	"mediaforge/internal/indexer"
)

type audioIdentity struct {
	title, artist, album, albumArtist         string
	track, disc                               int
	releaseID, groupID, artistID, recordingID string
	codec                                     string
	bitDepth                                  int
}

type albumTrack struct {
	id, number, disc   int
	title, recordingID string
	path               sql.NullString
}

// Bound total metadata reads while retaining seeks to ID3v1/MP4 tags near EOF.
type metadataReader struct {
	*os.File
	remaining int64
}

func (r *metadataReader) Read(dst []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, fmt.Errorf("audio metadata exceeds 32 MiB")
	}
	if int64(len(dst)) > r.remaining {
		dst = dst[:r.remaining]
	}
	n, err := r.File.Read(dst)
	r.remaining -= int64(n)
	return n, err
}

func readAudioIdentity(path string) (identity audioIdentity, err error) {
	f, err := os.Open(path)
	if err != nil {
		return identity, err
	}
	defer f.Close()
	header := make([]byte, 42)
	n, _ := f.Read(header)
	header = header[:n]
	f.Seek(0, io.SeekStart)
	switch {
	case strings.HasPrefix(string(header), "fLaC"):
		identity.codec = "flac"
		if len(header) >= 42 && header[4]&0x7f == 0 && header[7] == 34 {
			identity.bitDepth = int((binary.BigEndian.Uint64(header[18:26])>>36)&31) + 1
		}
	case strings.HasPrefix(string(header), "ID3") || len(header) >= 2 && header[0] == 0xff && header[1]&0xe0 == 0xe0:
		identity.codec = "mp3"
	case len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WAVE":
		identity.codec = "wav"
	}
	recognized := strings.HasPrefix(string(header), "fLaC") || strings.HasPrefix(string(header), "ID3") || strings.HasPrefix(string(header), "OggS") || len(header) >= 8 && string(header[4:8]) == "ftyp"
	// The library parses untrusted container metadata. A malformed tag must
	// become an import error, never a process panic or a filename fallback.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("malformed audio metadata")
		}
	}()
	m, err := tag.ReadFrom(&metadataReader{File: f, remaining: 32 << 20})
	if err != nil {
		if !recognized || errors.Is(err, tag.ErrNoTagsFound) {
			return identity, nil
		}
		return identity, fmt.Errorf("cannot read embedded audio tags: %w", err)
	}
	identity.title, identity.artist, identity.album, identity.albumArtist = m.Title(), m.Artist(), m.Album(), m.AlbumArtist()
	switch m.FileType() {
	case tag.FLAC:
		identity.codec = "flac"
	case tag.MP3:
		identity.codec = "mp3"
	case tag.M4A, tag.M4B, tag.M4P:
		identity.codec = "aac"
	case tag.OGG:
		identity.codec = "ogg"
	case tag.ALAC:
		identity.codec = "alac"
	}
	identity.track, _ = m.Track()
	identity.disc, _ = m.Disc()
	ids := mbz.Extract(m)
	identity.releaseID, identity.groupID = ids.Get(mbz.Album), ids.Get(mbz.ReleaseGroup)
	identity.artistID = ids.Get(mbz.AlbumArtist)
	if identity.artistID == "" && identity.albumArtist == "" {
		identity.artistID = ids.Get(mbz.Artist)
	}
	identity.recordingID = ids.Get(mbz.Recording)
	// Picard's Vorbis/MP4 MUSICBRAINZ_TRACKID stores the recording ID;
	// ID3's separate release-track ID is deliberately not used here.
	if identity.recordingID == "" && (m.Format() == tag.VORBIS || m.Format() == tag.MP4) {
		identity.recordingID = ids.Get(mbz.Track)
	}
	return identity, nil
}

func validateMusicCodec(tags audioIdentity, rawQualities string) error {
	if rawQualities == "" {
		return nil
	}
	var qualities []string
	if err := json.Unmarshal([]byte(rawQualities), &qualities); err != nil {
		return fmt.Errorf("invalid music quality profile")
	}
	if len(qualities) == 0 {
		return fmt.Errorf("music quality profile has no allowed formats")
	}
	allowed := map[string]bool{}
	losslessOnly := true
	allowsFLAC16 := false
	for _, quality := range qualities {
		family := strings.SplitN(quality, "-", 2)[0]
		allowed[family] = true
		if family != "flac" && family != "alac" && family != "wav" && family != "ape" {
			losslessOnly = false
		}
		if quality == "flac" {
			allowsFLAC16 = true
		}
	}
	if tags.codec == "" {
		if losslessOnly {
			return fmt.Errorf("cannot verify the lossless audio container")
		}
		return nil
	}
	if !allowed[tags.codec] {
		return fmt.Errorf("actual %s audio is not allowed by the selected music profile", strings.ToUpper(tags.codec))
	}
	if tags.codec == "flac" && !allowsFLAC16 && tags.bitDepth != 24 {
		return fmt.Errorf("24-bit FLAC could not be verified")
	}
	return nil
}

func musicIDEqual(actual, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected))
}
func musicIDsContain(actual, expected string) bool {
	for _, id := range strings.FieldsFunc(actual, func(r rune) bool { return r == ';' || r == '/' || r == ',' || r == '\x00' }) {
		if musicIDEqual(id, expected) {
			return true
		}
	}
	return false
}

type albumIdentity struct{ artist, title, editionTitle, edition, artistID, releaseID, groupID string }

func (album albumIdentity) validate(tags audioIdentity) error {
	if tags.groupID != "" && album.groupID != "" && !musicIDEqual(tags.groupID, album.groupID) {
		return fmt.Errorf("embedded release-group ID belongs to another album")
	}
	if tags.artistID != "" && album.artistID != "" && !musicIDsContain(tags.artistID, album.artistID) {
		return fmt.Errorf("embedded album-artist ID does not match")
	}
	artist := tags.albumArtist
	if artist == "" {
		artist = tags.artist
	}
	matchedArtistID := tags.artistID != "" && album.artistID != "" && musicIDsContain(tags.artistID, album.artistID)
	if artist != "" && !matchedArtistID && !indexer.MatchesMusicName(artist, album.artist) && !(tags.albumArtist == "" && indexer.MatchesMusicName(album.artist, "Various Artists")) {
		return fmt.Errorf("embedded artist does not match album artist")
	}
	if tags.album != "" && !indexer.MatchesMusicName(tags.album, album.title) && !indexer.MatchesMusicName(tags.album, album.editionTitle) {
		return fmt.Errorf("embedded album title or edition does not match")
	}
	return nil
}

var discTrackPrefix = regexp.MustCompile(`^(\d{1,2})[-.](\d{1,3})(?:[ ._-]+|$)`)
var trackPrefix = regexp.MustCompile(`^(\d{1,3})(?:[ ._-]+|$)`)

func audioFilenameIdentity(path string) (title string, number, disc int) {
	title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if match := discTrackPrefix.FindStringSubmatch(title); match != nil {
		disc, _ = strconv.Atoi(match[1])
		number, _ = strconv.Atoi(match[2])
		title = title[len(match[0]):]
	} else if match := trackPrefix.FindStringSubmatch(title); match != nil {
		number, _ = strconv.Atoi(match[1])
		title = title[len(match[0]):]
	}
	if match := discDirectory.FindStringSubmatch(filepath.Base(filepath.Dir(path))); match != nil {
		disc, _ = strconv.Atoi(match[1])
	}
	return strings.TrimSpace(title), number, disc
}
