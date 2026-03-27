package postprocess

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// NamingTemplates holds the naming templates for different content types.
type NamingTemplates struct {
	Movie        string
	Series       string
	Anime        string
	AnimeSpecial string
}

// DefaultTemplates returns the default naming templates.
func DefaultTemplates() NamingTemplates {
	return NamingTemplates{
		Movie:        "{title} ({year})/{title} ({year})",
		Series:       "{title} ({year})/Season {season:02d}/{title} - S{season:02d}E{episode:02d} - {episode_title}",
		Anime:        "{title}/Season {season:02d}/{title} - S{season:02d}E{episode:02d} - {absolute:03d} - {episode_title}",
		AnimeSpecial: "{title}/Specials/{title} - S00E{special_number:02d} - {episode_title}",
	}
}

// MoviePath generates the destination path for a movie file.
func MoviePath(mediaRoot, title string, year int, ext string) string {
	clean := SanitizeFilename(title)
	dir := fmt.Sprintf("%s (%d)", clean, year)
	file := fmt.Sprintf("%s (%d)%s", clean, year, ext)
	return filepath.Join(mediaRoot, "movies", dir, file)
}

// SeriesEpisodePath generates the destination path for a TV series episode.
func SeriesEpisodePath(mediaRoot, title string, year, season, episode int, episodeTitle, ext string) string {
	clean := SanitizeFilename(title)
	epClean := SanitizeFilename(episodeTitle)
	seriesDir := fmt.Sprintf("%s (%d)", clean, year)
	seasonDir := fmt.Sprintf("Season %02d", season)
	file := fmt.Sprintf("%s - S%02dE%02d - %s%s", clean, season, episode, epClean, ext)
	return filepath.Join(mediaRoot, "tv", seriesDir, seasonDir, file)
}

// AnimeEpisodePath generates the destination path for an anime episode.
func AnimeEpisodePath(mediaRoot, title string, season, episode, absolute int, episodeTitle, ext string) string {
	clean := SanitizeFilename(title)
	epClean := SanitizeFilename(episodeTitle)
	seasonDir := fmt.Sprintf("Season %02d", season)
	file := fmt.Sprintf("%s - S%02dE%02d - %03d - %s%s", clean, season, episode, absolute, epClean, ext)
	return filepath.Join(mediaRoot, "anime", clean, seasonDir, file)
}

// AnimeSpecialPath generates the destination path for an anime special/OVA.
func AnimeSpecialPath(mediaRoot, title string, specialNum int, episodeTitle, ext string) string {
	clean := SanitizeFilename(title)
	epClean := SanitizeFilename(episodeTitle)
	file := fmt.Sprintf("%s - S00E%02d - %s%s", clean, specialNum, epClean, ext)
	return filepath.Join(mediaRoot, "anime", clean, "Specials", file)
}

// Audio file extensions.
var audioExtensions = map[string]bool{
	".flac": true,
	".mp3":  true,
	".ogg":  true,
	".m4a":  true,
	".aac":  true,
	".wav":  true,
}

// IsAudioFile checks if a filename has an audio extension.
func IsAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return audioExtensions[ext]
}

// MusicTrackPath generates the destination path for a music track.
// Format: music/{artist}/{album} ({year})/{track:02d} - {title}.{ext}
func MusicTrackPath(mediaRoot, artist, album string, year, trackNum, discNum int, title, ext string) string {
	cleanArtist := SanitizeFilename(artist)
	cleanAlbum := SanitizeFilename(album)
	cleanTitle := SanitizeFilename(title)

	var albumDir string
	if year > 0 {
		albumDir = fmt.Sprintf("%s (%d)", cleanAlbum, year)
	} else {
		albumDir = cleanAlbum
	}

	var file string
	if discNum > 1 {
		file = fmt.Sprintf("%d-%02d - %s%s", discNum, trackNum, cleanTitle, ext)
	} else {
		file = fmt.Sprintf("%02d - %s%s", trackNum, cleanTitle, ext)
	}
	return filepath.Join(mediaRoot, "music", cleanArtist, albumDir, file)
}

// Video file extensions.
var videoExtensions = map[string]bool{
	".mkv": true,
	".mp4": true,
	".avi": true,
	".wmv": true,
	".m4v": true,
	".ts":  true,
}

// IsVideoFile checks if a filename has a video extension.
func IsVideoFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return videoExtensions[ext]
}

// SanitizeFilename removes characters that are invalid in file paths.
var invalidChars = regexp.MustCompile(`[<>:"/\\|?*]`)

func SanitizeFilename(name string) string {
	clean := invalidChars.ReplaceAllString(name, "")
	clean = strings.TrimSpace(clean)
	// Collapse multiple spaces
	clean = regexp.MustCompile(`\s+`).ReplaceAllString(clean, " ")
	return clean
}
