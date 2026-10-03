# Configuration

All provider keys and download connections can be managed in **Settings**. Quick start uses the same forms; you can complete setup without an indexer or assistant account.

## Connections

| Connection | What to enter | Test behavior |
| --- | --- | --- |
| TMDB | API key or read access token | Authenticated configuration request |
| SABnzbd | Base URL and API key | Authenticated queue request |
| qBittorrent | Enabled, base URL, username, password | Login and version request |
| OpenRouter | API key, model ID, reasoning effort | Authenticated key check; no paid completion |
| Last.fm | Optional API key | Enables music discovery |
| Brave Search | Optional API key | Enables assistant web search |

Tests use the current form values and fall back to saved credentials for blank secret fields. Testing never changes the active connection. Save applies changes immediately, including proxy routing and background metadata refresh.

Saved secret fields are blank with a configured indicator. Leaving them blank preserves the existing value; the API supports explicit `clear_<setting_name>: true` when removing a key. Settings and indexer list APIs never return secret values. Regular settings are saved together in one transaction, so invalid input cannot partially replace your configuration.

The default media root is `/data/media`. Use an absolute path visible inside the Zarr container. Configure paths shared by the downloader and Zarr identically; a host path like `C:\Media` is not a Linux container path.

## Indexers

Settings supports creating, editing, enabling, testing, and deleting Newznab and Rutracker connections. Set the supported content types and priority for each source. NZBGeek's preset uses `https://api.nzbgeek.info`; supply your own API key. Editing a source does not require re-entering its secret.

Indexer access and Usenet provider access are separate: the indexer finds NZBs, while the provider delivers their articles.

## Usenet servers / backbones

Configure SABnzbd first, then use **Usenet servers** in Zarr to add or edit providers. These forms manage SABnzbd's actual server configuration through its [configuration API](https://sabnzbd.org/wiki/advanced/api), so you do not have to visit a second application.

Each provider has a display name, hostname, port, TLS setting, username, password, connection limit, enabled state, and priority. Saved passwords stay in SABnzbd. Blank password edits preserve them. The Test action verifies the saved NNTP connection and authentication. Deleting a provider removes it from SABnzbd.

The Newsgroup Ninja preset uses `news.newsgroup.ninja`, TLS port 563, and 20 connections. See the provider's [connection settings](https://support.newsgroup.ninja/kb/article/515-other-newsreaders/) for regional alternatives and account limits.

## Download clients

Fresh Compose installations are wired automatically. For external clients:

- SABnzbd: enter its reachable URL and API key. Use the `mediaforge` category and shared completed/incomplete folders. The app validates authentication rather than relying on the public version endpoint.
- qBittorrent: enable the connection and enter its reachable Web UI URL and login. Zarr tracks jobs in the `zarr` category with `dl_<id>` tags. Configure the desired save path in qBittorrent. Seed hours control when an imported torrent may be removed; failed imports retain their source data.

Downloads are marked available only after importing files successfully. Cancel stops the downloader and restores wanted state. A failed import is visible as failed and retains source data for diagnosis/retry.

## Assistant

The default is `moonshotai/kimi-k3` with `high` reasoning. Choose another OpenRouter model ID or reasoning level in Settings. The assistant streams reasoning separately from answer text and preserves opaque provider reasoning details and tool IDs in conversation history. It handles interrupted/error streams explicitly rather than saving an apparently successful empty answer.

Connection testing validates your key. Actual model availability and credits are checked when you chat. Model inference uses your OpenRouter account. See [OpenRouter reasoning documentation](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens) for supported model behavior.

## Proxy

Enter an HTTP(S) or SOCKS5 proxy URL for external APIs. Downloader traffic stays direct on the local network. From Docker Desktop, a proxy on the host's port 7897 is usually `http://host.docker.internal:7897`; `localhost` inside a container refers to that container. Save the proxy before testing external services.

## Quality profiles

Create or edit profiles for video and music with qualities, language, tag bonuses, reject patterns, and upgrade policy. Zarr supplies initial profiles and selects a suitable default when none is specified. Profiles still assigned to library items cannot be deleted.

## Environment variables

Environment variables remain optional first-run seeds for existing deployment tooling. Once a setting exists in the database, the UI is authoritative, including an explicitly cleared key. Process settings such as `MEDIAFORGE_CONFIG_DIR`, `MEDIAFORGE_PORT`, and `MEDIAFORGE_LOG_LEVEL` remain environment variables.
