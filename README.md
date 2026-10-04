# Zarr

**Self-hosted media library manager for movies, TV series, anime, and music.**

Zarr brings discovery, release selection, download tracking, and library organization into one application. It uses SABnzbd for Usenet transfers and qBittorrent for torrents, with connections managed from the web UI.

## Changelog

### Music discovery and download improvements (2026-10-04)

- Cached music catalog, complete artist discographies, personal discovery shelves and verified artwork fallbacks.
- Separate saving, downloading and monitoring, with persistent search status and retries.
- Artist/album/edition checks before grabs, explicit edition selection, and shared tag-aware validation for downloads and manual imports.
- [Music guide](docs/music.md) covers profiles, caching, import behavior and current limits.

### v0.3.0 (2026-10-04) — Neon Archive and core overhaul

- New Neon Archive interface with a unified collection dashboard, responsive navigation, and live download panels.
- Quick start without mandatory keys, UI-managed connections and Usenet providers, and automatic setup of the bundled download clients.
- Recoverable imports and upgrades, safer retries/cancellation, preserved torrent seed files, and fixes to library, scanner, and assistant workflows.
- Anime films and series remain distinct across discovery, recommendations, imports, and release searches.
- Separate [TRaSH-based anime movie and series profiles](docs/anime-quality-profiles.md), with grouped qualities, custom formats, score explanations, and safe automatic upgrades.
- [Full changelog](CHANGELOG.md) · [Verification and known limits](docs/overhaul-verification.md) · [UI screenshots](docs/neon-archive.md)


### v0.2.0 (2026-03-02)

- **6 Color Themes** — Added Ember (warm dark), Violet (cool dark), Rose (warm light), and Mint (cool light) alongside the existing Obsidian and Light themes
- **Expandable Theme Picker** — Replaced the simple toggle with a popover showing all themes grouped by Dark/Light
- **Music Library** — Full music support with MusicBrainz, CoverArt Archive, and Last.fm integration. Track-level monitoring, album search, and quality profiles for FLAC/MP3/AAC
- **Torrent Support** — qBittorrent integration for torrent downloads alongside Usenet. Automatic progress tracking, seed ratio management, and cleanup
- **Rutracker Indexer** — Scraper-based torrent indexer with session auth, rate limiting, and forum category mapping

### v0.1.0

- Initial release with movies, TV series, and anime management
- Usenet downloading via SABnzbd with automatic post-processing
- AI assistant with personalized recommendations via OpenRouter
- Quality profiles with release scoring
- Personal ratings and review system
- Obsidian Glass design with dark and light themes

## Features

- **Unified Library** — Movies, TV series, anime, and music in one place
- **Smart Discovery** — Trending content, search across TMDB, AniList, and MusicBrainz
- **Dual Download** — Usenet (SABnzbd) and torrent (qBittorrent) support
- **Multi-Indexer** — Newznab and Rutracker with priority management
- **UI-managed Connections** — Metadata and AI keys, download clients, indexers, Usenet providers, and proxies in Settings
- **Release Scoring** — Quality profiles, best release selection, and automatic upgrades for the TRaSH anime presets
- **AI Assistant** — OpenRouter recommendations with configurable models, streamed reasoning, and tool history; Kimi K3/high reasoning by default
- **Music Support** — Personal discovery, cached metadata/covers, edition selection, explicit downloads and tag-verified imports
- **Existing Libraries** — Import files with metadata matching and reconcile organized media through library scans
- **6 Themes** — Obsidian, Ember, Violet (dark) + Light, Rose, Mint (light)
- **Ratings** — Personal 1-5 star ratings with persistent metadata
- **Activity Tracking** — Complete history, download queue, real-time progress

## Architecture

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24+ with embedded HTTP server |
| Frontend | SvelteKit + TypeScript |
| Database | SQLite (embedded, zero config) |
| Container | Multi-stage Docker build (Node > Go > Alpine) |
| Downloaders | SABnzbd (Usenet), qBittorrent (torrent) |
| Metadata | TMDB, AniList, MusicBrainz, CoverArt Archive, Last.fm |
| AI | OpenRouter + Brave Search |

## Quick Start

```bash
git clone https://github.com/acidmiku/zarr.git
cd zarr
docker compose up -d --build
```

Open [Zarr](http://localhost:9876). Add connections in Quick start or open the library immediately. The bundled downloaders receive generated credentials automatically; provider keys, indexers, Usenet servers, and AI settings are managed in the UI. No `.env` editing is required.

Existing installation? Follow the [upgrade notes](docs/installation.md#storage-and-upgrades). Saved connections, custom profiles, and library assignments are preserved. New anime additions use the matching TRaSH preset; existing titles opt in by changing their quality profile. This is a single-user local service; see [installation](docs/installation.md) before exposing it beyond localhost.

## Documentation

| Section | Description |
|---------|-------------|
| [Installation](docs/installation.md) | Prerequisites, Docker Compose setup, directory structure |
| [Configuration](docs/configuration.md) | TMDB, SABnzbd, qBittorrent, indexers, quality profiles, AI setup |
| [Anime quality profiles](docs/anime-quality-profiles.md) | TRaSH presets, scoring, dual audio, upgrades, and compatibility limits |
| [Music](docs/music.md) | Discovery, saving, monitoring, edition selection, artwork caching and verified imports |
| [Usage](docs/usage.md) | Adding content, library management, music, AI assistant, ratings |
| [Development](docs/development.md) | Building from source, project structure, migrations, API routes |
| [Verification](docs/overhaul-verification.md) | Automated tests, live checks, and untested network paths |
| [Troubleshooting](docs/troubleshooting.md) | Common issues, performance tips, security notes |
| [Contributing Indexers](docs/contributing-indexers.md) | How to add new torrent indexer integrations |

## Roadmap

- [ ] Multi-user support with authentication
- [ ] Plex/Jellyfin/Emby library sync
- [ ] Custom metadata editing
- [ ] Subtitle management
- [ ] Mobile app

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See [Contributing Indexers](docs/contributing-indexers.md) for a guide on adding new torrent indexer support.

## License

This project is licensed under the MIT License — see the LICENSE file for details.
