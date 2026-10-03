package metadata

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

var musicBrainzIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const coverArtBase = "https://coverartarchive.org"

// CoverArtClient fetches album cover art from the Cover Art Archive.
type CoverArtClient struct {
	client   *http.Client
	cacheDir string
}

func NewCoverArtClient(client *http.Client, configDir string) *CoverArtClient {
	cacheDir := filepath.Join(configDir, "cache", "covers")
	os.MkdirAll(cacheDir, 0755)
	return &CoverArtClient{client: client, cacheDir: cacheDir}
}

// GetCover fetches the front cover for a release group, caching indefinitely.
// Returns the local file path.
func (c *CoverArtClient) GetCover(releaseGroupID string) (string, error) {
	if !musicBrainzIDPattern.MatchString(releaseGroupID) {
		return "", fmt.Errorf("invalid MusicBrainz release group ID")
	}
	cached := c.CoverPath(releaseGroupID)

	// Check cache
	if _, err := os.Stat(cached); err == nil {
		return cached, nil
	}

	// Fetch from archive
	u := fmt.Sprintf("%s/release-group/%s/front", coverArtBase, releaseGroupID)
	resp, err := c.client.Get(u)
	if err != nil {
		return "", fmt.Errorf("coverart request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("coverart %d for %s", resp.StatusCode, releaseGroupID)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024+1))
	if err != nil {
		return "", fmt.Errorf("coverart read: %w", err)
	}

	if len(data) > 20*1024*1024 {
		return "", fmt.Errorf("cover art exceeds 20 MB")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("cover art is empty")
	}
	// Publish only complete images, so simultaneous requests cannot serve partial caches.
	file, err := os.CreateTemp(c.cacheDir, "cover-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(file.Name(), cached); err != nil {
		if _, statErr := os.Stat(cached); statErr != nil {
			return "", err
		}
	}

	slog.Debug("cached cover art", "rgid", releaseGroupID)
	return cached, nil
}

// CoverPath returns the cached cover path without fetching.
func (c *CoverArtClient) CoverPath(releaseGroupID string) string {
	if !musicBrainzIDPattern.MatchString(releaseGroupID) {
		return ""
	}
	return filepath.Join(c.cacheDir, "mb_"+releaseGroupID+".jpg")
}

// HasCover checks if a cover is already cached.
func (c *CoverArtClient) HasCover(releaseGroupID string) bool {
	_, err := os.Stat(c.CoverPath(releaseGroupID))
	return err == nil
}
