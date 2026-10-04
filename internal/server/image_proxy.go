package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// imageCache handles on-disk caching of proxied images.
//
// Policy:
//   - Fetch once, keep forever. No TTL.
//   - At most N concurrent upstream fetches (the proxy chokes under fan-out).
//   - Retry 5xx / transport errors; don't retry 4xx.
//   - Requests for the same URL coalesce: one fetcher, everyone else waits
//     on the same result — on fetcher failure, waiters do NOT re-fetch.
//   - Short-lived negative cache for 403/404 so UI scroll doesn't re-ask.
type imageCache struct {
	dir string

	// Bounded upstream concurrency.
	sem chan struct{}

	inflightMu sync.Mutex
	inflight   map[string]*inflightEntry

	negMu    sync.Mutex
	negCache map[string]time.Time
	negTTL   time.Duration
}

type inflightEntry struct {
	done chan struct{}
	// Populated before done is closed.
	data   []byte
	ct     string
	status int
	err    error
}

const (
	imgUpstreamConcurrency = 5
	imgNegativeTTL         = 10 * time.Minute
)

func newImageCache(configDir string) *imageCache {
	dir := filepath.Join(configDir, "image_cache")
	os.MkdirAll(dir, 0755)
	return &imageCache{
		dir:      dir,
		sem:      make(chan struct{}, imgUpstreamConcurrency),
		inflight: make(map[string]*inflightEntry),
		negCache: make(map[string]time.Time),
		negTTL:   imgNegativeTTL,
	}
}

func cacheKey(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}

type cachedImage struct {
	path        string
	contentType string
	etag        string
	modTime     time.Time
}

// get returns a cached image if it exists on disk. No TTL — fetch once, keep forever.
func (c *imageCache) get(url string) *cachedImage {
	key := cacheKey(url)
	dataPath := filepath.Join(c.dir, key+".dat")
	metaPath := filepath.Join(c.dir, key+".meta")

	info, err := os.Stat(dataPath)
	if err != nil {
		return nil
	}

	ct := "image/jpeg"
	if meta, err := os.ReadFile(metaPath); err == nil && len(meta) > 0 {
		ct = string(meta)
	}

	return &cachedImage{
		path:        dataPath,
		contentType: ct,
		etag:        `"` + key + `"`,
		modTime:     info.ModTime(),
	}
}

// put writes the image to disk atomically. Meta file is written first, then
// data is renamed into place — so if get() sees the data file, meta is ready.
func (c *imageCache) put(url string, data []byte, contentType string) error {
	key := cacheKey(url)
	dataPath := filepath.Join(c.dir, key+".dat")
	metaPath := filepath.Join(c.dir, key+".meta")
	dataTmp := dataPath + ".tmp"

	if err := os.WriteFile(metaPath, []byte(contentType), 0644); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	if err := os.WriteFile(dataTmp, data, 0644); err != nil {
		os.Remove(metaPath)
		return fmt.Errorf("write data tmp: %w", err)
	}
	if err := os.Rename(dataTmp, dataPath); err != nil {
		os.Remove(dataTmp)
		os.Remove(metaPath)
		return fmt.Errorf("rename data: %w", err)
	}
	return nil
}

func (c *imageCache) isNegative(url string) bool {
	c.negMu.Lock()
	defer c.negMu.Unlock()
	exp, ok := c.negCache[url]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(c.negCache, url)
		return false
	}
	return true
}

func (c *imageCache) markNegative(url string) {
	c.negMu.Lock()
	defer c.negMu.Unlock()
	c.negCache[url] = time.Now().Add(c.negTTL)
}

// fetchOrJoin runs a single upstream fetch for the URL, or joins an already
// in-flight fetch. The returned entry has its result populated by the time
// done is closed.
func (c *imageCache) fetchOrJoin(ctx context.Context, client *http.Client, url string) *inflightEntry {
	c.inflightMu.Lock()
	if existing, ok := c.inflight[url]; ok {
		c.inflightMu.Unlock()
		// Wait for the in-flight fetcher (or ctx cancel).
		select {
		case <-existing.done:
			return existing
		case <-ctx.Done():
			return &inflightEntry{done: closedChan(), status: 499, err: ctx.Err()}
		}
	}
	entry := &inflightEntry{done: make(chan struct{})}
	c.inflight[url] = entry
	c.inflightMu.Unlock()

	// We own the fetch. Do it synchronously so the caller can directly use the
	// result; other waiters block on entry.done.
	// The caller's earlier disk check may predate another completed fetch. Once
	// ownership is established, recheck before opening another upstream request.
	if cached := c.get(url); cached != nil {
		if data, err := os.ReadFile(cached.path); err == nil {
			entry.data, entry.ct, entry.status = data, cached.contentType, http.StatusOK
		}
	}
	if entry.status != http.StatusOK {
		if c.isNegative(url) {
			entry.status, entry.err = http.StatusNotFound, fmt.Errorf("image is temporarily unavailable")
		} else {
			c.doFetch(ctx, client, url, entry)
		}
	}

	c.inflightMu.Lock()
	delete(c.inflight, url)
	close(entry.done)
	c.inflightMu.Unlock()
	return entry
}

// doFetch acquires a semaphore slot and runs a single HTTP fetch — transient
// retries are handled by the underlying retry transport on the HTTP client.
// This layer adds status-aware handling (negative cache, success persisting).
func (c *imageCache) doFetch(ctx context.Context, client *http.Client, url string, entry *inflightEntry) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		entry.status = 499
		entry.err = ctx.Err()
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		entry.status = http.StatusBadGateway
		entry.err = err
		return
	}
	imageClient := *client
	imageClient.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowedImageURL(r.URL.String()) {
			return fmt.Errorf("image redirect is not allowed")
		}
		return nil
	}
	resp, err := imageClient.Do(req)
	if err != nil {
		entry.status = http.StatusBadGateway
		entry.err = err
		slog.Warn("image fetch failed", "url", url, "error", err)
		return
	}
	defer resp.Body.Close()

	const maxImageBytes = 15 << 20
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if len(body) > maxImageBytes {
		readErr = fmt.Errorf("image exceeds size limit")
	}
	if readErr != nil {
		entry.status = http.StatusBadGateway
		entry.err = readErr
		slog.Warn("image read failed", "url", url, "error", readErr)
		return
	}

	status := resp.StatusCode
	ct := resp.Header.Get("Content-Type")

	if status == http.StatusOK {
		ct = http.DetectContentType(body)
		if ct != "image/jpeg" && ct != "image/png" && ct != "image/gif" && ct != "image/webp" {
			entry.status = http.StatusBadGateway
			entry.err = fmt.Errorf("upstream response is not a supported image")
			return
		}
		if err := c.put(url, body, ct); err != nil {
			slog.Warn("image cache write failed", "url", url, "error", err)
		}
		entry.data = body
		entry.ct = ct
		entry.status = http.StatusOK
		return
	}

	if status == http.StatusForbidden || status == http.StatusNotFound {
		c.markNegative(url)
	}
	entry.status = status
	entry.err = fmt.Errorf("upstream %d", status)
	slog.Warn("image fetch non-OK", "url", url, "status", status)
}

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

var allowedImagePrefixes = []string{
	"https://image.tmdb.org/",
	"https://s4.anilist.co/",
	"https://img.anili.st/",
	"https://cdn.myanimelist.net/",
	"https://coverartarchive.org/",
	"https://archive.org/",
	"https://lastfm.freetls.fastly.net/",
	"https://lastfm-img.freetls.fastly.net/",
	"https://lastfm-img2.akamaized.net/",
}

func allowedImageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	for _, prefix := range allowedImagePrefixes {
		if strings.HasPrefix(raw, prefix) {
			return true
		}
	}
	// Cover Art Archive redirects image downloads to Internet Archive storage.
	return strings.HasSuffix(u.Hostname(), ".archive.org") || u.Hostname() == "dzcdn.net" || strings.HasSuffix(u.Hostname(), ".dzcdn.net") || u.Hostname() == "mzstatic.com" || strings.HasSuffix(u.Hostname(), ".mzstatic.com")
}

// handleImageProxy proxies external images through the backend to avoid CORS issues,
// routes through the configured proxy, and caches on disk to avoid hammering upstream APIs.
func (s *Server) handleImageProxy(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		writeError(w, 400, "url parameter required")
		return
	}

	if !allowedImageURL(imageURL) {
		writeError(w, 403, "URL not allowed")
		return
	}

	// 1. Disk cache hit — serve immediately.
	if s.imgCache != nil {
		if cached := s.imgCache.get(imageURL); cached != nil {
			serveCached(w, r, cached)
			return
		}
		// 2. Negative cache hit — don't bother upstream.
		if s.imgCache.isNegative(imageURL) {
			writeError(w, 404, "image not found")
			return
		}
	}

	if s.imgCache == nil {
		writeError(w, 500, "image cache unavailable")
		return
	}

	// 3. Fetch (or join an in-flight fetch). Single-flight + retry live here.
	entry := s.imgCache.fetchOrJoin(r.Context(), s.proxyClient, imageURL)
	if entry.status != http.StatusOK {
		// Pass through 403/404 faithfully; everything else collapses to 502.
		if entry.status == http.StatusForbidden || entry.status == http.StatusNotFound {
			writeError(w, entry.status, "upstream error")
			return
		}
		if entry.status == 499 { // client canceled
			return
		}
		writeError(w, http.StatusBadGateway, "failed to fetch image")
		return
	}

	// Success — serve from disk (authoritative) if available, else from memory.
	if cached := s.imgCache.get(imageURL); cached != nil {
		serveCached(w, r, cached)
		return
	}
	// Fallback: put() failed but we have bytes in-memory.
	w.Header().Set("Content-Type", entry.ct)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	w.Write(entry.data)
}

func serveCached(w http.ResponseWriter, r *http.Request, c *cachedImage) {
	w.Header().Set("Content-Type", c.contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", c.etag)
	w.Header().Set("X-Cache", "HIT")

	// Conditional request support — 304 Not Modified.
	if inm := r.Header.Get("If-None-Match"); inm != "" && strings.Contains(inm, c.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	f, err := os.Open(c.path)
	if err != nil {
		writeError(w, 500, "cache read failed")
		return
	}
	defer f.Close()
	http.ServeContent(w, r, filepath.Base(c.path), c.modTime, f)
}
