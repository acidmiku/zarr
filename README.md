# Zarr

**Self-hosted media library manager for movies, TV series, and anime.**

Zarr is an all-in-one solution that replaces the traditional "arr-stack" (Sonarr, Radarr, Prowlarr, etc.) with a single, unified application. It handles everything from discovery and searching to downloading and organizing your media library.

## Features

### 🎬 Media Management
- **Unified Library** - Manage movies, TV series, and anime in one place
- **Smart Discovery** - Browse trending content and search across TMDB and AniList
- **Automatic Organization** - Files are automatically organized with clean, standardized naming
- **Quality Profiles** - Customize quality preferences, upgrade policies, and filtering rules
- **Season Management** - Add specific seasons, track episodes, and manage episode types (standard, special, OVA, etc.)

### 🔍 Intelligent Search & Download
- **Multi-Indexer Support** - Connect multiple Newznab indexers with priority management
- **Release Scoring** - Automatic quality scoring and best release selection
- **SABnzbd Integration** - Seamless Usenet downloading with progress tracking
- **Automatic Upgrades** - Optionally upgrade to better quality releases when available
- **Episode Tracking** - Air date monitoring and automatic searching for new episodes

### 🤖 AI Assistant
- **Personalized Recommendations** - AI-powered recommendations based on your ratings and viewing history
- **Natural Conversations** - Chat naturally about what to watch next
- **Context-Aware** - Understands your preferences and avoids suggesting titles you've already rated low
- **Tool Integration** - Can search anime databases, web search, and create ratings directly
- **Multiple AI Models** - Supports various OpenRouter models including Claude, GPT-4, and more

### 📊 Ratings & Reviews
- **Personal Ratings** - Rate titles 1-5 stars with optional comments
- **Persistent Metadata** - Ratings retain title/poster even if removed from library
- **Flexible Filtering** - Sort and filter by type, rating, or date
- **Inline Editing** - Quick edit ratings and comments without leaving the page

### 🎨 Modern Interface
- **Obsidian Glass Design** - Beautiful frosted glass aesthetic with smooth animations
- **Dark & Light Themes** - Toggle between themes with system preferences
- **Bright Cyan Accents** - Distinctive color scheme with excellent contrast
- **Responsive Design** - Works beautifully on desktop and mobile
- **Real-time Updates** - Live download progress and activity feeds

### 📈 Activity Tracking
- **Complete History** - Every action logged (added, grabbed, imported, failed)
- **Episode-Level Details** - Track exactly which episodes were grabbed and when
- **Download Queue** - Monitor active downloads with progress, speed, and ETA
- **Error Reporting** - Clear failure messages when downloads don't complete

## Architecture

### Stack
- **Backend**: Go 1.22+ with embedded HTTP server
- **Frontend**: SvelteKit with TypeScript
- **Database**: SQLite (embedded, zero configuration)
- **Container**: Multi-stage Docker build (Node → Go → Alpine runtime)
- **External Services**:
  - SABnzbd (Usenet downloader)
  - TMDB (metadata for movies/TV)
  - AniList (anime metadata)
  - OpenRouter (AI assistant)
  - Brave Search (optional, for AI web search)

### Design Principles
- **Single Binary**: Everything embedded - frontend, migrations, static assets
- **Zero Dependencies**: No external database servers or message queues
- **Docker First**: Designed to run in containers alongside SABnzbd
- **Minimal Configuration**: Sensible defaults with optional customization

## Prerequisites

- **Docker** and **Docker Compose** installed
- **Usenet Provider** with SABnzbd configured (or willing to set up)
- **TMDB API Key** (free from https://www.themoviedb.org/settings/api)
- **Indexer Access** (Newznab-compatible, e.g., NZBGeek, DrunkenSlug, etc.)
- **OpenRouter API Key** (optional, for AI assistant - https://openrouter.ai)

## Installation

### Quick Start

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

### Docker Compose Configuration

The included `docker-compose.yml` runs both Zarr and SABnzbd:

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

### Directory Structure

```
data/
├── config/           # Zarr configuration and database
├── media/           # Organized media files
│   ├── movies/
│   ├── series/
│   └── anime/
├── sabnzbd-config/  # SABnzbd configuration
└── usenet/          # Shared download directory
    ├── complete/
    └── incomplete/
```

## Configuration

### Initial Setup

1. **TMDB API Key**
   - Visit https://www.themoviedb.org/settings/api
   - Copy your API key
   - Paste into Settings → TMDB API Key

2. **SABnzbd Configuration**

   Access SABnzbd at http://localhost:8080 and configure:

   **a. Usenet Server**
   - Settings → Servers → Add your Usenet provider credentials

   **b. Folders (Settings → Folders)**
   - Temporary Download Folder: `/data/usenet/incomplete`
   - Completed Download Folder: `/data/usenet/complete`

   **c. Categories (Settings → Categories)**
   - Add a new category named: `mediaforge`
   - Check the `+Delete` option
   - This tells SABnzbd to delete downloads after processing, saving disk space

   **d. Security (Settings → General → Security)**
   - Host Whitelist: Add `sabnzbd, localhost, 127.0.0.1`
     - This allows Zarr to connect via Docker's internal network
     - **Critical:** Without this, Zarr's requests will be rejected

   **e. API Key**
   - Copy the API key from Settings → General → API Key
   - In Zarr Settings → SABnzbd:
     - URL: `http://sabnzbd:8080` (use Docker service name)
     - API Key: (paste from SABnzbd)

3. **Add Indexers**
   - Go to Settings → Indexers
   - Add your Newznab indexers (URL, API key, priority)
   - Test each indexer to verify connectivity

4. **Create Quality Profile**
   - Settings → Quality Profiles → Create
   - Select desired qualities (e.g., remux-1080p, bluray-1080p, web-1080p)
   - Set language preference
   - Configure upgrade policy
   - Add reject patterns (e.g., "cam", "ts", "hdcam")

### AI Assistant Setup (Optional)

1. **Get OpenRouter API Key**
   - Sign up at https://openrouter.ai
   - Generate an API key
   - Add credits to your account

2. **Configure in Zarr**
   - Settings → AI Assistant → OpenRouter API Key
   - Select a model (default: Claude Sonnet 4)
   - Test the connection

3. **Optional: Brave Search**
   - Get API key from https://brave.com/search/api/
   - Add to Settings → AI Assistant → Brave Search API Key
   - Enables web search capability for the AI

### Advanced Settings

- **Media Root**: Default `/data/media` - where organized files are stored
- **Proxy**: Optional HTTP proxy for external API calls
- **AI Personality**: Customize the assistant's tone and behavior
- **Custom AI System Prompt**: Fine-tune recommendations logic

## Usage

### Adding Content

1. **Discover** tab - Browse trending or search for titles
2. Click a poster to view details
3. Select quality profile
4. Click "Add to Library"
5. For series: Choose which seasons to monitor
6. Zarr automatically searches for releases and begins downloading

### Managing Your Library

- **Library** tab - View all content with filters (type, status)
- Click any title to see:
  - Metadata and overview
  - Episode list (for series)
  - Download history
  - Manual search options
  - Rating/review

### Episode Management

- **Expand seasons** to see episode lists
- Episode statuses:
  - **Wanted** - Monitored, searching
  - **Searching** - Active search in progress
  - **Downloading** - In SABnzbd queue
  - **Available** - File downloaded and organized
- Manual actions:
  - Search specific episodes
  - Cancel downloads
  - Delete files
  - Reset to wanted

### Using the AI Assistant

1. **Assistant** tab
2. Start a new chat or continue previous conversation
3. Ask for recommendations: "What should I watch next?"
4. Give feedback: "I've already seen Breaking Bad, loved it" (auto-creates rating)
5. Browse recommendations with personalized explanations
6. Click "Add to Library" directly from recommendation cards

### Rating Content

- **From library detail**: Click "Rate this" button
- **From ratings page**: Edit ratings inline
- **Via AI assistant**: Mention titles you've watched
- Ratings persist even if you remove the title from your library

### Monitoring Downloads

- **Activity** tab - See recent actions (grabbed, imported, failed)
- **Download Queue** - Real-time progress with speed and ETA
- Failed downloads show error messages
- Completed downloads automatically import and organize

## Development

### Building from Source

```bash
# Install dependencies
cd ui && npm install && cd ..

# Build frontend
cd ui && npm run build && cd ..

# Build Go binary
go build -o mediaforge ./cmd/mediaforge

# Run locally
./mediaforge
```

### Development Mode

```bash
# Terminal 1: Frontend dev server
cd ui && npm run dev

# Terminal 2: Backend with API proxy
go run ./cmd/mediaforge
```

### Project Structure

```
zarr/
├── cmd/mediaforge/          # Main entry point
├── internal/
│   ├── ai/                  # AI assistant (OpenRouter, tools, personality)
│   ├── config/              # Configuration management
│   ├── database/            # SQLite database and migrations
│   ├── grabber/             # SABnzbd integration
│   ├── indexer/             # Newznab client and release scoring
│   ├── metadata/            # TMDB, AniList, anime-offline-database
│   ├── postprocess/         # File organization and naming
│   ├── scanner/             # Library scanning
│   ├── scheduler/           # Background tasks (polling, RSS, metadata refresh)
│   └── server/              # HTTP handlers and routes
├── ui/                      # SvelteKit frontend
│   ├── src/
│   │   ├── lib/
│   │   │   ├── components/  # Reusable UI components
│   │   │   └── stores/      # Svelte stores (app state, theme)
│   │   └── routes/          # Pages (discover, library, assistant, etc.)
│   └── static/              # Static assets
├── docker-compose.yml       # Docker orchestration
├── Dockerfile               # Multi-stage build
└── README.md
```

### Database Migrations

Migrations are embedded in `internal/database/migrations/*.sql` and run automatically on startup. Migration files:
- `001_initial.sql` - Core schema
- `002_ratings.sql` - User ratings table
- `003_ai_sessions.sql` - AI chat history
- `004_ratings_metadata.sql` - Cached metadata on ratings

### API Routes

All routes under `/api`:

**Discovery**
- `GET /search` - Search TMDB/AniList
- `GET /trending` - Trending content
- `GET /metadata/{tmdbID}` - Full metadata

**Library**
- `GET /library` - List all items
- `POST /library` - Add to library
- `GET /library/{id}` - Item details
- `DELETE /library/{id}` - Remove item

**Episodes**
- `GET /library/{id}/episodes` - List seasons/episodes
- `POST /library/{id}/episodes/{epID}/search` - Manual search
- `POST /library/{id}/search` - Search all wanted
- `POST /library/{id}/seasons` - Add seasons

**Downloads**
- `GET /downloads` - Queue status
- `DELETE /downloads/{id}` - Cancel download

**Ratings**
- `GET /ratings` - List user ratings
- `POST /ratings` - Create/update rating

**AI Assistant**
- `GET /ai/sessions` - Chat history
- `POST /ai/sessions` - New session
- `POST /ai/sessions/{id}/chat` - Send message
- `POST /ai/sessions/{id}/recommend` - Get recommendations

## Troubleshooting

### Downloads Marked as Failed

**Symptom**: SABnzbd completes successfully but Zarr shows "failed"

**Cause**: Path mismatch between containers

**Solution**: Ensure both containers share the same volume mount for downloads:
```yaml
volumes:
  - ./data/usenet:/data/usenet  # Both containers
```

### "No video files found" Error

**Check**:
1. SABnzbd completed extraction successfully
2. Files have supported extensions (.mkv, .mp4, .avi, .m4v, .ts)
3. Download path is accessible to Zarr container

### SABnzbd Connection Issues

**Symptom**: Zarr can't connect to SABnzbd, or SABnzbd logs show "Refused connection"

**Solution**:
1. Check SABnzbd Settings → General → Security → Host Whitelist
2. Add: `sabnzbd, localhost, 127.0.0.1`
3. Restart SABnzbd
4. Verify URL in Zarr uses `http://sabnzbd:8080` (Docker service name, not localhost)

### Downloads Not Appearing in SABnzbd

**Symptom**: Zarr says "grabbed" but nothing appears in SABnzbd queue

**Solution**:
1. Check SABnzbd Settings → Categories
2. Ensure `mediaforge` category exists with `+Delete` enabled
3. Without this category, SABnzbd may reject downloads

### Search Returns No Results

**Common causes**:
1. Indexer API key invalid - test in Settings → Indexers
2. No releases match quality profile - check reject patterns
3. TVDB ID missing for series - try adding manually via library detail

### AI Assistant Not Responding

**Check**:
1. OpenRouter API key valid and has credits
2. Model selected in settings
3. Network connectivity to OpenRouter API
4. Browser console for error messages

## Performance Tips

- **Indexer Priority**: Set faster indexers to higher priority
- **Quality Profiles**: Be specific - fewer qualities = faster decisions
- **Episode Polling**: Default 30min is fine, reduce for faster new episode detection
- **Database Location**: SSD recommended for better query performance
- **Docker Resources**: Allocate at least 1GB RAM for smooth operation

## Security Notes

- **API Keys**: Stored encrypted in SQLite database
- **Network Exposure**: Zarr has no authentication - run behind reverse proxy or VPN
- **File Permissions**: Container runs as user:group 1000:1000 by default
- **HTTPS**: Use nginx/Caddy reverse proxy for HTTPS in production

## Roadmap

- [ ] Multi-user support with authentication
- [ ] Plex/Jellyfin/Emby library sync
- [ ] Custom metadata editing
- [ ] Import existing library (scan existing files)
- [ ] Torrent support (via qBittorrent)
- [ ] Subtitle management
- [ ] Mobile app

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
