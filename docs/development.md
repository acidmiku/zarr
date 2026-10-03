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
| `002_ratings.sql` | User ratings table |
| `003_ai_sessions.sql` | AI chat history |
| `004_ratings_metadata.sql` | Cached metadata on ratings |
| `005_music.sql` | Artists, albums, tracks tables |
| `006_torrent_support.sql` | Indexer type/credentials, download_type, qbt_hash fields |
| `007_fix_downloads_nullable.sql` | Nullable media_item_id for music downloads |

## API Routes

All routes under `/api`:

### Discovery
- `GET /search` — Search TMDB/AniList
- `GET /trending` — Trending content
- `GET /metadata/{tmdbID}` — Full metadata

### Library
- `GET /library` — List all items
- `POST /library` — Add to library
- `GET /library/{id}` — Item details
- `DELETE /library/{id}` — Remove item

### Episodes
- `GET /library/{id}/episodes` — List seasons/episodes
- `POST /library/{id}/episodes/{epID}/search` — Manual search
- `POST /library/{id}/search` — Search all wanted
- `POST /library/{id}/seasons` — Add seasons

### Downloads
- `GET /downloads` — Queue status
- `DELETE /downloads/{id}` — Cancel download

### Ratings
- `GET /ratings` — List user ratings
- `POST /ratings` — Create/update rating

### AI Assistant
- `GET /ai/sessions` — Chat history
- `POST /ai/sessions` — New session
- `POST /ai/sessions/{id}/chat` — Send message
- `POST /ai/sessions/{id}/recommend` — Get recommendations

### Music
- `GET /music/artists` — List artists
- `GET /music/artists/{id}` — Artist details with albums
- `POST /music/artists` — Add artist
- `POST /music/albums/{id}/search` — Search album releases
