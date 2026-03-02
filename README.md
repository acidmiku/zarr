# Zarr

**Self-hosted media library manager for movies, TV series, anime, and music.**

Zarr is an all-in-one solution that replaces the traditional "arr-stack" (Sonarr, Radarr, Prowlarr, etc.) with a single, unified application. It handles everything from discovery and searching to downloading and organizing your media library.

## Changelog

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
- Quality profiles with release scoring and automatic upgrades
- Personal ratings and review system
- Obsidian Glass design with dark and light themes

## Features

- **Unified Library** — Movies, TV series, anime, and music in one place
- **Smart Discovery** — Trending content, search across TMDB, AniList, and MusicBrainz
- **Dual Download** — Usenet (SABnzbd) and torrent (qBittorrent) support
- **Multi-Indexer** — Newznab and Rutracker with priority management
- **Release Scoring** — Automatic quality scoring, best release selection, and upgrades
- **AI Assistant** — Personalized recommendations via OpenRouter (Claude, GPT-4, etc.)
- **Music Support** — Artist/album management with MusicBrainz and Last.fm metadata
- **6 Themes** — Obsidian, Ember, Violet (dark) + Light, Rose, Mint (light)
- **Ratings** — Personal 1-5 star ratings with persistent metadata
- **Activity Tracking** — Complete history, download queue, real-time progress

## Architecture

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.22+ with embedded HTTP server |
| Frontend | SvelteKit + TypeScript |
| Database | SQLite (embedded, zero config) |
| Container | Multi-stage Docker build (Node > Go > Alpine) |
| Downloaders | SABnzbd (Usenet), qBittorrent (torrent) |
| Metadata | TMDB, AniList, MusicBrainz, CoverArt Archive, Last.fm |
| AI | OpenRouter + Brave Search |

## Quick Start

```bash
git clone https://github.com/YOUR_USERNAME/zarr.git
cd zarr
mkdir -p data/{config,media,sabnzbd-config,usenet}
docker compose up -d
```

Open http://localhost:9876 and complete the setup wizard.

## Documentation

| Section | Description |
|---------|-------------|
| [Installation](docs/installation.md) | Prerequisites, Docker Compose setup, directory structure |
| [Configuration](docs/configuration.md) | TMDB, SABnzbd, qBittorrent, indexers, quality profiles, AI setup |
| [Usage](docs/usage.md) | Adding content, library management, music, AI assistant, ratings |
| [Development](docs/development.md) | Building from source, project structure, migrations, API routes |
| [Troubleshooting](docs/troubleshooting.md) | Common issues, performance tips, security notes |
| [Contributing Indexers](docs/contributing-indexers.md) | How to add new torrent indexer integrations |

## Roadmap

- [ ] Multi-user support with authentication
- [ ] Plex/Jellyfin/Emby library sync
- [ ] Custom metadata editing
- [ ] Import existing library (scan existing files)
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
