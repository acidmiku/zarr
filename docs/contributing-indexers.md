# Contributing New Torrent Indexers

This guide walks through how to add a new torrent indexer to Zarr. The existing Rutracker integration (`internal/rutracker/`) serves as the reference implementation.

## Architecture Overview

Zarr's indexer system has three layers:

1. **Indexer clients** — Each indexer type has its own Go package under `internal/` that handles authentication, searching, and downloading `.torrent` files.
2. **Release normalization** — Search results from any indexer are converted to a common `Release` struct, then scored against quality profiles.
3. **Download dispatch** — Based on `DownloadType` (`"nzb"` or `"torrent"`), the release is sent to SABnzbd or qBittorrent.

```
Indexer Client  ──>  []Release  ──>  ScoreRelease()  ──>  BestRelease()  ──>  Download Client
(your package)       (common)        (scorer.go)          (scorer.go)         (qbt / grabber)
```

## Key Types

### IndexerConfig (`internal/indexer/newznab.go`)

Every indexer is stored in the `indexers` database table and loaded as an `IndexerConfig`:

```go
type IndexerConfig struct {
    ID           int
    Name         string
    URL          string
    APIKey       string
    Priority     int
    Enabled      bool
    Type         string   // "newznab", "rutracker", or your new type
    Username     string
    Password     string
    ContentTypes []string // ["movie", "series", "anime", "music"]
}
```

The `Type` field determines which client handles searches for this indexer.

### Release (`internal/indexer/scorer.go`)

All search results are normalized into `Release` structs:

```go
type Release struct {
    Title        string   // Raw release title
    NZBURL       string   // Download URL (NZB link or empty for torrents)
    Size         int64    // File size in bytes
    Quality      string   // Detected quality: "web-1080p", "bluray-2160p", etc.
    Tags         []string // Detected tags: "10bit", "dual-audio", "hdr", etc.
    Score        int      // Computed by ScoreRelease()
    Indexer      string   // Indexer name for display
    Acceptable   bool     // Set by ScoreRelease()
    DownloadType string   // "nzb" or "torrent"
    Seeders      int      // Torrent-specific
    Leechers     int      // Torrent-specific
    TopicID      int      // Indexer-specific ID for downloading
}
```

### Release Parsing (`internal/indexer/parser.go`)

`ParseReleaseName(title)` extracts quality and tags from any release title. You don't need to duplicate this logic — just call it with the title from your indexer's results.

### Release Scoring (`internal/indexer/scorer.go`)

`ScoreRelease(release, profile)` checks reject patterns, validates language, looks up quality in the user's profile, and computes a numeric score. `BestRelease(releases)` picks the highest-scoring acceptable release.

## Step-by-Step: Adding a New Indexer

### 1. Create the client package

Create `internal/yourindexer/yourindexer.go`:

```go
package yourindexer

import (
    "net/http"
    "sync"
)

type Result struct {
    ID       int    // Site-specific ID for downloading
    Title    string
    Size     int64
    Seeders  int
    Leechers int
}

type Client struct {
    client  *http.Client
    baseURL string
    mu      sync.Mutex
    // Add auth state (session cookies, tokens, etc.)
}

func New(httpClient *http.Client, baseURL string) *Client {
    return &Client{
        client:  httpClient,
        baseURL: baseURL,
    }
}

// Login authenticates with the indexer.
func (c *Client) Login(username, password string) error {
    // Implement authentication
}

// Search queries the indexer for releases.
func (c *Client) Search(query string, contentType string, username, password string) ([]Result, error) {
    // Implement search, parse results
}

// DownloadTorrent downloads a .torrent file by ID.
func (c *Client) DownloadTorrent(id int, username, password string) ([]byte, error) {
    // Return raw .torrent bytes
}

// TestConnection verifies connectivity.
func (c *Client) TestConnection(username, password string) error {
    return c.Login(username, password)
}
```

Key considerations:
- Use `sync.Mutex` to protect shared auth state
- Implement rate limiting if the site requires it (`golang.org/x/time/rate`)
- Handle charset conversion if needed (see `internal/rutracker/convert.go`)
- Always auto-relogin on session expiry

### 2. Wire into the Server

Add your client to the `Server` struct in `internal/server/server.go`:

```go
type Server struct {
    // ... existing fields ...
    yourindexer *yourindexer.Client
}
```

Update the `New()` constructor to accept and store the client.

### 3. Wire into the Scheduler

Add your client to the `Scheduler` struct in `internal/scheduler/scheduler.go`:

```go
type Scheduler struct {
    // ... existing fields ...
    yourindexer *yourindexer.Client
}
```

Update `scheduler.New()` similarly.

### 4. Initialize in main.go

In `cmd/mediaforge/main.go`, create the client and pass it to both `server.New()` and `scheduler.New()`:

```go
yourindexerClient := yourindexer.New(clients.Proxy, "https://yourindexer.example.com")
```

### 5. Add search integration

In the search handler (e.g., `internal/server/handlers_episodes.go`), add a branch for your indexer type:

```go
// Filter indexers by type
yourIndexers := filterIndexersByType(allIndexers, "yourindexer", contentType)

for _, idx := range yourIndexers {
    results, err := s.yourindexer.Search(query, contentType, idx.Username, idx.Password)
    if err != nil {
        slog.Warn("yourindexer search failed", "error", err)
        continue
    }

    for _, r := range results {
        parsed := indexer.ParseReleaseName(r.Title)
        releases = append(releases, indexer.Release{
            Title:        r.Title,
            Size:         r.Size,
            Quality:      parsed.Quality,
            Tags:         parsed.Tags,
            Indexer:      idx.Name,
            DownloadType: "torrent",
            Seeders:      r.Seeders,
            Leechers:     r.Leechers,
            TopicID:      r.ID,  // Or use a different field
        })
    }
}
```

### 6. Add download integration

When the best release has `DownloadType == "torrent"`, download the `.torrent` and send it to qBittorrent:

```go
if best.DownloadType == "torrent" {
    torrentData, err := s.yourindexer.DownloadTorrent(best.TopicID, idx.Username, idx.Password)
    if err != nil {
        return err
    }
    err = s.qbt.AddTorrent(torrentData, fmt.Sprintf("dl_%d.torrent", downloadID), int(downloadID))
}
```

### 7. Database migration

If your indexer needs new fields beyond what `Type`, `Username`, and `Password` provide, create a new migration in `internal/database/migrations/`:

```sql
-- 00N_yourindexer_support.sql
ALTER TABLE indexers ADD COLUMN your_field TEXT DEFAULT '';
```

### 8. UI: Add indexer type option

In `ui/src/routes/settings/+page.svelte`, add your indexer type to the type dropdown so users can select it when adding an indexer.

## Testing

1. Add your indexer in Settings > Indexers with the new type
2. Use "Test Connection" to verify authentication
3. Search for a known title and verify results appear
4. Grab a release and verify the `.torrent` uploads to qBittorrent
5. Monitor the download through Activity and the download queue

## Reference: Rutracker Implementation

- **Client**: `internal/rutracker/rutracker.go` (327 lines)
  - Session-based auth via form POST (`Login`)
  - HTML scraping for search results (`parseSearchResults`)
  - Rate limiting at 1 request per 2 seconds
  - Windows-1251 charset handling (`internal/rutracker/convert.go`)
- **Forum ID mapping**: Hardcoded map of content type to forum IDs
- **Integration**: Wired into `server.go`, `scheduler.go`, `main.go`, and `handlers_episodes.go`
