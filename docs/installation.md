# Installation

## Prerequisites

- **Docker** and **Docker Compose** installed
- **Usenet Provider** with SABnzbd configured (or willing to set up)
- **TMDB API Key** (free from https://www.themoviedb.org/settings/api)
- **Indexer Access** (Newznab-compatible, e.g., NZBGeek, DrunkenSlug, etc.)
- **OpenRouter API Key** (optional, for AI assistant - https://openrouter.ai)

## Quick Start

1. **Clone the repository**
   ```bash
   git clone https://github.com/YOUR_USERNAME/zarr.git
   cd zarr
   ```

   > Replace `YOUR_USERNAME` with your GitHub username or organization

2. **Create data directories**
   ```bash
   mkdir -p data/{config,media,sabnzbd-config,usenet}
   ```

3. **Start the stack**
   ```bash
   docker compose up -d
   ```

4. **Access Zarr**
   - Open http://localhost:9876 in your browser
   - Complete the setup wizard

## Docker Compose Configuration

The included `docker-compose.yml` runs Zarr, SABnzbd, and optionally qBittorrent:

```yaml
version: "3.8"

services:
  mediaforge:
    build: .
    ports:
      - "9876:9876"
    volumes:
      - ./data/config:/config
      - ./data/media:/data/media
      - ./data/usenet:/data/usenet
      - ./data/sabnzbd-config/Downloads:/sabnzbd-downloads
    depends_on:
      - sabnzbd

  sabnzbd:
    image: lscr.io/linuxserver/sabnzbd:latest
    ports:
      - "8080:8080"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=America/New_York
    volumes:
      - ./data/sabnzbd-config:/config
      - ./data/usenet:/data/usenet
```

## Directory Structure

```
data/
├── config/           # Zarr configuration and database
├── media/           # Organized media files
│   ├── movies/
│   ├── series/
│   ├── anime/
│   └── music/
├── sabnzbd-config/  # SABnzbd configuration
└── usenet/          # Shared download directory
    ├── complete/
    └── incomplete/
```
