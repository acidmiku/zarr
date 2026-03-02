package metadata

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

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
	cached := filepath.Join(c.cacheDir, "mb_"+releaseGroupID+".jpg")

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

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("coverart read: %w", err)
	}

	if err := os.WriteFile(cached, data, 0644); err != nil {
		slog.Warn("coverart cache write failed", "error", err)
		return "", err
	}

	slog.Debug("cached cover art", "rgid", releaseGroupID)
	return cached, nil
}

// CoverPath returns the cached cover path without fetching.
func (c *CoverArtClient) CoverPath(releaseGroupID string) string {
	return filepath.Join(c.cacheDir, "mb_"+releaseGroupID+".jpg")
}

// HasCover checks if a cover is already cached.
func (c *CoverArtClient) HasCover(releaseGroupID string) bool {
	_, err := os.Stat(c.CoverPath(releaseGroupID))
	return err == nil
}
