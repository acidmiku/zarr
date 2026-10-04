package metadata

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"mediaforge/internal/database"
)

type metadataFlight struct {
	done chan struct{}
	data []byte
	err  error
}
type metadataSnapshot struct {
	data    []byte
	expires time.Time
}
type metadataHTTPError struct {
	host   string
	status int
}

func (e *metadataHTTPError) Error() string {
	return fmt.Sprintf("%s returned HTTP %d", e.host, e.status)
}

type musicMetadataCache struct {
	client   *http.Client
	db       *database.DB
	limiter  *rate.Limiter
	mu       sync.Mutex
	inflight map[string]*metadataFlight
	memory   map[string]metadataSnapshot
	backoff  map[string]time.Time
}

func newMusicMetadataCache(client *http.Client, db *database.DB, limiter *rate.Limiter) *musicMetadataCache {
	return &musicMetadataCache{client: client, db: db, limiter: limiter, inflight: map[string]*metadataFlight{}, memory: map[string]metadataSnapshot{}, backoff: map[string]time.Time{}}
}
func musicCacheKey(rawURL string) string {
	return fmt.Sprintf("music-metadata:v1:%x", sha256.Sum256([]byte(rawURL)))
}

func (c *musicMetadataCache) read(key string) metadataSnapshot {
	if c.db != nil {
		var data, expiry string
		if c.db.QueryRow(`SELECT data,CAST(expires_at AS TEXT) FROM cache WHERE cache_key=?`, key).Scan(&data, &expiry) == nil && json.Valid([]byte(data)) {
			parsed, err := time.Parse(time.RFC3339, expiry)
			if err != nil {
				parsed, _ = time.Parse("2006-01-02 15:04:05", expiry)
			}
			return metadataSnapshot{[]byte(data), parsed}
		}
		return metadataSnapshot{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.memory[key]
}
func (c *musicMetadataCache) save(key string, data []byte, ttl time.Duration) {
	expires := time.Now().UTC().Add(ttl)
	if c.db != nil {
		c.db.Exec(`INSERT INTO cache(cache_key,data,expires_at) VALUES(?,?,?) ON CONFLICT(cache_key) DO UPDATE SET data=excluded.data,expires_at=excluded.expires_at`, key, string(data), expires.Format(time.RFC3339))
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.memory) > 500 {
		c.memory = map[string]metadataSnapshot{}
	}
	c.memory[key] = metadataSnapshot{append([]byte(nil), data...), expires}
}

// Metadata snapshots are never deleted merely because they expired. A refresh
// failure can serve the last valid JSON, and identical requests share one fetch.
func (c *musicMetadataCache) get(ctx context.Context, rawURL string, ttl time.Duration) ([]byte, error) {
	key := musicCacheKey(rawURL)
	snapshot := c.read(key)
	if len(snapshot.data) > 0 && time.Now().Before(snapshot.expires) {
		return snapshot.data, nil
	}
	c.mu.Lock()
	if existing := c.inflight[key]; existing != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-existing.done:
			return existing.data, existing.err
		}
	}
	flight := &metadataFlight{done: make(chan struct{})}
	c.inflight[key] = flight
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.inflight, key); close(flight.done); c.mu.Unlock() }()
	// Another fetch may have published between the first read and registration.
	snapshot = c.read(key)
	if len(snapshot.data) > 0 && time.Now().Before(snapshot.expires) {
		flight.data = snapshot.data
		return flight.data, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	data, err := c.fetch(requestCtx, rawURL)
	if err == nil {
		c.save(key, data, ttl)
		flight.data = data
		return data, nil
	}
	var statusError *metadataHTTPError
	permanentFailure := errors.As(err, &statusError) && statusError.status >= 400 && statusError.status < 500 && statusError.status != 429
	if ctx.Err() == nil && !permanentFailure && len(snapshot.data) > 0 {
		flight.data = snapshot.data
		return flight.data, nil
	}
	flight.err = err
	return nil, err
}
func retryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		if seconds > 3600 {
			seconds = 3600
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		d := time.Until(date)
		if d > time.Hour {
			return time.Hour
		}
		if d > 0 {
			return d
		}
	}
	return time.Minute
}
func (c *musicMetadataCache) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata endpoint")
	}
	backoffKey := "music-backoff:" + u.Hostname()
	c.mu.Lock()
	until := c.backoff[u.Hostname()]
	c.mu.Unlock()
	if saved := c.read(backoffKey); len(saved.data) > 0 {
		var stored time.Time
		if json.Unmarshal(saved.data, &stored) == nil && stored.After(until) {
			until = stored
		}
	}
	if time.Now().Before(until) {
		return nil, fmt.Errorf("%s is cooling down after an upstream failure", u.Hostname())
	}
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		// Another request may have received Retry-After while this one queued.
		c.mu.Lock()
		until = c.backoff[u.Hostname()]
		c.mu.Unlock()
		if time.Now().Before(until) {
			return nil, fmt.Errorf("%s is cooling down after an upstream failure", u.Hostname())
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata request")
	}
	req.Header.Set("User-Agent", mbUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c.cooldown(u.Hostname(), backoffKey, 10*time.Second)
		return nil, fmt.Errorf("%s is unavailable", u.Hostname())
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 || resp.StatusCode == 503 {
		c.cooldown(u.Hostname(), backoffKey, retryAfter(resp.Header.Get("Retry-After")))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &metadataHTTPError{u.Hostname(), resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		return nil, fmt.Errorf("metadata response incomplete or too large")
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("upstream returned invalid metadata JSON")
	}
	var apiError struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &apiError) == nil && len(apiError.Error) > 0 && string(apiError.Error) != "null" && string(apiError.Error) != "0" {
		return nil, fmt.Errorf("upstream metadata service reported an error")
	}
	return data, nil
}
func (c *musicMetadataCache) cooldown(host, key string, duration time.Duration) {
	until := time.Now().UTC().Add(duration)
	c.mu.Lock()
	c.backoff[host] = until
	c.mu.Unlock()
	data, _ := json.Marshal(until)
	c.save(key, data, duration)
}
