# Configuration

## Initial Setup

### 1. TMDB API Key

- Visit https://www.themoviedb.org/settings/api
- Copy your API key
- Paste into Settings > TMDB API Key

### 2. SABnzbd Configuration

Access SABnzbd at http://localhost:8080 and configure:

**a. Usenet Server**
- Settings > Servers > Add your Usenet provider credentials

**b. Folders (Settings > Folders)**
- Temporary Download Folder: `/data/usenet/incomplete`
- Completed Download Folder: `/data/usenet/complete`

**c. Categories (Settings > Categories)**
- Add a new category named: `mediaforge`
- Check the `+Delete` option
- This tells SABnzbd to delete downloads after processing, saving disk space

**d. Security (Settings > General > Security)**
- Host Whitelist: Add `sabnzbd, localhost, 127.0.0.1`
  - This allows Zarr to connect via Docker's internal network
  - **Critical:** Without this, Zarr's requests will be rejected

**e. API Key**
- Copy the API key from Settings > General > API Key
- In Zarr Settings > SABnzbd:
  - URL: `http://sabnzbd:8080` (use Docker service name)
  - API Key: (paste from SABnzbd)

### 3. qBittorrent Configuration (Optional)

If you want torrent support alongside Usenet:

- Add the qBittorrent service to your `docker-compose.yml`
- In Zarr Settings > qBittorrent:
  - URL: `http://qbittorrent:8080` (Docker service name)
  - Username/Password: your qBittorrent Web UI credentials
- Zarr uses the `zarr` category and tags downloads with `dl_{id}` for tracking

### 4. Add Indexers

- Go to Settings > Indexers
- Add your Newznab indexers (URL, API key, priority)
- For torrent indexers (e.g., Rutracker), select the appropriate type and provide credentials
- Test each indexer to verify connectivity

### 5. Create Quality Profile

- Settings > Quality Profiles > Create
- Select desired qualities (e.g., remux-1080p, bluray-1080p, web-1080p)
- Set language preference
- Configure upgrade policy
- Add reject patterns (e.g., "cam", "ts", "hdcam")

## AI Assistant Setup (Optional)

### 1. Get OpenRouter API Key

- Sign up at https://openrouter.ai
- Generate an API key
- Add credits to your account

### 2. Configure in Zarr

- Settings > AI Assistant > OpenRouter API Key
- Select a model (default: Claude Sonnet 4)
- Test the connection

### 3. Optional: Brave Search

- Get API key from https://brave.com/search/api/
- Add to Settings > AI Assistant > Brave Search API Key
- Enables web search capability for the AI

## Advanced Settings

- **Media Root**: Default `/data/media` — where organized files are stored
- **Proxy**: Optional HTTP proxy for external API calls
- **AI Personality**: Customize the assistant's tone and behavior
- **Custom AI System Prompt**: Fine-tune recommendations logic
