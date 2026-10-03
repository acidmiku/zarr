# Development

## Build and verify

Use Go 1.24+ with CGO and a C compiler, and Node.js 22.12+.

```sh
go test -race ./...
go vet ./...
cd ui
npm ci
npx playwright install chromium
npm run test:ui
npm audit --audit-level=moderate
```

The browser suite uses a production build with API fixtures; it does not require real provider credentials. Go tests create isolated temporary databases/files and mock external services. CI runs both suites on Linux.

For a complete runnable build, use `docker compose up -d --build`; its build embeds the Svelte output in the Go binary. If building manually, copy `ui/build/*` into `internal/server/static/` before `go build ./cmd/mediaforge`.

## Development mode

Run the backend with a private development config directory and an absolute media root. Environment seeds are used only for settings not already present in that database. Run `npm run dev` in `ui` for the frontend; Vite proxies `/api` to localhost:9876. Use the same browser origin for the page and API: arbitrary cross-origin browser requests are rejected.

The Windows build includes native disk-space support. On Windows, install a compatible C compiler for SQLite/CGO or use the Docker build.

## Project Structure

```
zarr/
├── cmd/mediaforge/          # Main entry point
├── internal/
│   ├── ai/                  # AI assistant (OpenRouter, tools, personality)
│   ├── config/              # Configuration management
│   ├── database/            # SQLite database and migrations
│   ├── grabber/             # SABnzbd integration
│   ├── httpclient/          # Proxy/direct HTTP client factory
│   ├── indexer/             # Newznab client, release parsing, and scoring
│   ├── metadata/            # TMDB, AniList, MusicBrainz, CoverArt, Last.fm
│   ├── postprocess/         # File organization and naming
│   ├── qbt/                 # qBittorrent Web API client
│   ├── rutracker/           # Rutracker scraper client
│   ├── scanner/             # Library scanning
│   ├── scheduler/           # Background tasks (polling, RSS, metadata refresh)
│   └── server/              # HTTP handlers and routes
├── ui/                      # SvelteKit frontend
│   ├── src/
│   │   ├── lib/
│   │   │   ├── components/  # Reusable UI components
│   │   │   ├── stores/      # Svelte stores (app state, theme)
│   │   │   └── api.ts       # API client
│   │   └── routes/          # Pages (discover, library, music, assistant, etc.)
│   └── static/              # Static assets
├── docs/                    # Documentation
├── docker-compose.yml       # Docker orchestration
├── Dockerfile               # Multi-stage build
└── README.md
```

## Database Migrations

Migrations are embedded in `internal/database/migrations/*.sql` and run automatically on startup:

| File | Description |
|------|-------------|
| `001_initial.sql` | Core schema (media_items, episodes, indexers, quality_profiles, downloads, activity_log, settings) |
| `002_user_ratings.sql` | User ratings table |
| `003_ai_tables.sql` | AI chat history |
| `004_ratings_metadata.sql` | Cached metadata on ratings |
| `005_music.sql` | Artists, albums, tracks tables |
| `006_torrent_support.sql` | Indexer type/credentials, download_type, qbt_hash fields |
| `007_fix_downloads_nullable.sql` | Nullable media_item_id for music downloads |
| `008_blacklist.sql` | Failed-release blacklist and lookup indexes |
| `009_ai_reasoning.sql` | Visible reasoning and opaque provider reasoning details |
| `010_library_integrity.sql` | Protect new AniList and album identities without deleting historical duplicates |
| `011_anime_scoring.sql` | Preset scoring config, current-release baselines, upgrade cadence, and separate anime presets |

## Anime scoring maintenance

The pinned profiles and custom formats live in `internal/indexer/trash/`. Read [the implementation notes](../internal/indexer/TRASH.md) before updating the snapshot. The updater accepts a reviewed full upstream commit SHA; review the data diff and run `go test -race ./internal/indexer` after updating. Keep the upstream license and integrity manifest with the data.

Advanced release ordering uses `CompareReleases`/`BestRelease`: quality-group rank precedes format score. The compatibility `score` field alone cannot order these releases. API clients can obtain expanded profile configuration from `/api/profiles` and preset metadata from `/api/profiles/presets`.

## API Routes

Selected routes under `/api`; `internal/server/routes.go` is the complete route registry:

### Discovery
- `GET /search` — Search TMDB/AniList
- `GET /trending` — Trending content
- `GET /metadata/{tmdbID}` — Full metadata

### Library
- `GET /library` — List all items
- `POST /library` — Add to library
- `GET /library/{id}` — Item details
- `PUT /library/{id}` — Change quality profile or anime flag
- `DELETE /library/{id}` — Remove item

### Episodes
- `GET /library/{id}/episodes` — List seasons/episodes
- `POST /library/{id}/episodes/{epID}/search` — Manual search
- `POST /library/{id}/search` — Search all wanted
- `POST /library/{id}/seasons` — Add seasons

### Downloads
- `GET /downloads` — Queue status
- `POST /downloads/{id}/retry` — Retry a failed download/import
- `DELETE /downloads/{id}` — Cancel download

### Profiles and release selection

- `GET /profiles` — Profiles with expanded scoring configuration
- `GET /profiles/presets` — Pinned preset defaults, formats, source URL and version
- `POST /profiles`, `PUT /profiles/{id}`, `DELETE /profiles/{id}` — Manage profiles
- `GET /releases` — Search and score releases for a library item or episode
- `POST /releases/grab` — Manually grab a release

### Setup and connections

- `GET /setup/status` — First-run setup state
- `GET /settings`, `PUT /settings` — Read/update settings; saved secrets are not echoed
- `POST /settings/test-{tmdb|openrouter|sabnzbd|qbittorrent}` — Test supplied connection details without saving them
- `GET /indexers`, `POST /indexers`, `PUT /indexers/{id}`, `DELETE /indexers/{id}` — Manage indexers
- `GET /usenet/servers`, `POST /usenet/servers`, `PUT /usenet/servers/{id}`, `DELETE /usenet/servers/{id}` — Manage SABnzbd providers
- `POST /import/scan`, `POST /import/execute` — Discover and import existing files
- `POST /system/scan` — Reconcile organized library files and invalidate uncertain release provenance

### Ratings
- `GET /ratings` — List user ratings
- `PUT /ratings` — Create/update rating

### AI Assistant
- `GET /ai/sessions` — Chat history
- `POST /ai/sessions` — New session
- `POST /ai/sessions/{id}/messages` — Send message
- `POST /ai/sessions/{id}/recommend` — Get recommendations

### Music
- `GET /music/library/artists` — List library artists
- `GET /music/artist/{mbid}` — Artist metadata with releases
- `POST /music/library` — Add an album to the library
- `POST /music/library/{id}/search` — Search album releases
