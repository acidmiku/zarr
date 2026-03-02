package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// imageCache handles on-disk caching of proxied images.
type imageCache struct {
	dir string
	mu  sync.RWMutex
	// in-flight dedup: prevents multiple concurrent fetches for the same URL
	inflight map[string]*sync.WaitGroup
	inflightMu sync.Mutex
}

func newImageCache(configDir string) *imageCache {
	dir := filepath.Join(configDir, "image_cache")
	os.MkdirAll(dir, 0755)
	return &imageCache{
		dir:      dir,
		inflight: make(map[string]*sync.WaitGroup),
	}
}

// cacheKey returns a stable filesystem-safe key for a URL.
func cacheKey(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}

type cachedImage struct {
	path        string
	contentType string
}

// get returns a cached image if it exists and isn't stale (7 days).
func (c *imageCache) get(url string) *cachedImage {
	key := cacheKey(url)
	dataPath := filepath.Join(c.dir, key+".dat")
	metaPath := filepath.Join(c.dir, key+".meta")

	info, err := os.Stat(dataPath)
	if err != nil {
		return nil
	}

	// Stale after 7 days
	if time.Since(info.ModTime()) > 7*24*time.Hour {
		return nil
	}

	ct := "image/jpeg" // default
	if meta, err := os.ReadFile(metaPath); err == nil {
		ct = string(meta)
	}

	return &cachedImage{path: dataPath, contentType: ct}
}

// put stores an image in the cache.
func (c *imageCache) put(url string, data []byte, contentType string) {
	key := cacheKey(url)
	dataPath := filepath.Join(c.dir, key+".dat")
	metaPath := filepath.Join(c.dir, key+".meta")

	if err := os.WriteFile(dataPath, data, 0644); err != nil {
		slog.Warn("image cache write failed", "error", err)
		return
	}
	os.WriteFile(metaPath, []byte(contentType), 0644)
}

// acquireInflight returns true if this goroutine should do the fetch.
// If false, another goroutine is already fetching — wait for it then read cache.
func (c *imageCache) acquireInflight(url string) (doFetch bool, wait func()) {
	c.inflightMu.Lock()
	if wg, ok := c.inflight[url]; ok {
		c.inflightMu.Unlock()
		return false, wg.Wait
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.inflight[url] = wg
	c.inflightMu.Unlock()
	return true, nil
}

func (c *imageCache) releaseInflight(url string) {
	c.inflightMu.Lock()
	if wg, ok := c.inflight[url]; ok {
		wg.Done()
		delete(c.inflight, url)
	}
	c.inflightMu.Unlock()
}

// handleImageProxy proxies external images through the backend to avoid CORS issues,
// routes through the configured proxy, and caches on disk to avoid hammering upstream APIs.
func (s *Server) handleImageProxy(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		writeError(w, 400, "url parameter required")
		return
	}

	// Only allow image URLs from known sources
	allowed := false
	allowedPrefixes := []string{
		"https://image.tmdb.org/",
		"https://s4.anilist.co/",
		"https://img.anili.st/",
		"https://cdn.myanimelist.net/",
		"https://coverartarchive.org/",
		"https://archive.org/",
		"https://lastfm.freetls.fastly.net/",
		"https://lastfm-img2.akamaized.net/",
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(imageURL, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, 403, "URL not allowed")
		return
	}

	// Check disk cache first
	if s.imgCache != nil {
		if cached := s.imgCache.get(imageURL); cached != nil {
			w.Header().Set("Content-Type", cached.contentType)
			w.Header().Set("Cache-Control", "public, max-age=604800") // 7 days browser cache
			w.Header().Set("X-Cache", "HIT")
			http.ServeFile(w, r, cached.path)
			return
		}
	}

	// Dedup concurrent fetches for the same URL
	if s.imgCache != nil {
		doFetch, wait := s.imgCache.acquireInflight(imageURL)
		if !doFetch {
			// Another goroutine is fetching this — wait then serve from cache
			wait()
			if cached := s.imgCache.get(imageURL); cached != nil {
				w.Header().Set("Content-Type", cached.contentType)
				w.Header().Set("Cache-Control", "public, max-age=604800")
				w.Header().Set("X-Cache", "HIT")
				http.ServeFile(w, r, cached.path)
				return
			}
			// Fallthrough to fetch if cache write somehow failed
		} else {
			defer s.imgCache.releaseInflight(imageURL)
		}
	}

	// Fetch from upstream via proxy client
	resp, err := s.proxyClient.Get(imageURL)
	if err != nil {
		writeError(w, 502, "failed to fetch image")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, 502, "failed to read image")
		return
	}

	// Determine content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		// Guess from URL extension
		ext := filepath.Ext(imageURL)
		contentType = mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "image/jpeg"
		}
	}

	// Store in disk cache
	if s.imgCache != nil {
		s.imgCache.put(imageURL, body, contentType)
	}

	// Serve to client
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
