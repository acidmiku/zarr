# Changelog

## Music discovery and download improvements (2026-10-04)

- Add a persistent local music catalog with artist aliases, genres, full paginated discographies including EPs/singles, cached metadata, shared request handling, provider cooldowns and outage fallback.
- Add personal discovery shelves, favorites and optional Last.fm listening-history/similar-artist suggestions. Resolve external suggestions to canonical MusicBrainz albums before saving.
- Split Save to library, Download now and album monitoring. Preserve existing monitoring intent, persist deduplicated search attempts across restarts, and distinguish source failures, no acceptable releases, transfers and verified imports.
- Match artist, album and edition before ranking music releases. Honor the profile's codec fallbacks, relax brittle year searches, and reject unidentified quality in music profiles.
- Prefer a standard official edition instead of maximizing track count; expose edition selection and verify selected-release membership and complete tracklists.
- Share tag/track/identity validation across downloaded and manually imported music. Validate actual codec families, retain mismatched/incomplete source files, and stage imports transactionally before recording availability.
- Resolve missing covers through verified alternative providers and reuse persistent disk/browser caches, saved provider choices and short-lived missing-art caches. Accept Last.fm's current artwork CDN through the image proxy.
- See [Music discovery and downloads](docs/music.md) for behavior and remaining limitations.


## Unreleased

### Fixes after the overhaul

- Load movie and series covers on assistant recommendation cards, including saved conversations, through the shared disk/browser image cache. Match the title and release year, reuse that match for the detail dialog, and show a compact fallback when artwork is unavailable.
- Allow cancellation and library removal when a tracked SABnzbd job was deleted externally or has finished. Verify the exact job in queue/history before clearing stale state, cancel active post-processing through the correct API, and keep connection/authentication failures visible.
- Fix title and album removal after cancelling a download: cancelled jobs are no longer sent to the downloader a second time. Repeated cancellation succeeds locally, and removal retries remember transfers already cancelled before another transfer failed.

### Anime identity and TRaSH alignment

- Keep anime movies and series distinct across TMDB/AniList discovery, recommendations, metadata, library creation, and imports. Chainsaw Man and Reze Arc use separate media identities and download paths.
- Search anime films through movie queries, include anime-only indexers, and prevent episodic files from matching films. Search mapped anime episodes with both absolute and season/episode numbering, then deduplicate results.
- Add separate anime series and movie presets based on pinned TRaSH Sonarr/Radarr profiles, with 41/31 upstream custom formats, source JSON, MIT license, integrity checks, and an update script.
- Implement quality-group priority, release-group tiers, required/optional/negated conditions, source constraints, revision/service preferences, rejection penalties, minimum scores, and quality/format upgrade cutoffs. Retain legacy scoring for existing custom profiles.
- Add dual-audio preferences, advanced profile controls, and expandable release-score explanations. Preserve declared score overrides when changing preferences, and keep release lists in quality-first order.
- Enable automatic upgrades for available anime with the matching preset and a known release baseline. Bound searches, honor cutoffs, prevent duplicate grabs, and recheck profile/baseline before importing.
- Keep existing files Available through upgrade downloads, failures and cancellation. Stage replacements before switching files, roll back failed imports, preserve torrent seed data, and retain failed sources for local retry.
- Invalidate uncertain release provenance on library rescans, preventing an old release title from causing a downgrade after an external file replacement. Combined multi-episode files that cannot be matched safely remain available for manual handling.
- Choose matching anime presets for new additions and automatic imports without reassigning existing library items. Fix BD/SD quality parsing and vertically align Settings controls across desktop/mobile layouts.

See [Anime quality profiles](docs/anime-quality-profiles.md) for defaults and remaining differences from Sonarr/Radarr. Audio/subtitle track inspection, size-per-minute limits, and richer release-information filenames are not included.

### Core reliability and setup

- Replace the blocking setup wizard with quick start and reusable connection forms; manage keys, editable indexers, and SABnzbd Usenet servers in Settings.
- Automatically wire fresh bundled SABnzbd/qBittorrent installations with unique credentials and shared paths. Preserve existing downloader configuration.
- Preserve blank secret edits, redact API responses, validate settings atomically, isolate connection tests, and apply proxy/client changes without restarting.
- Fix premature successful imports, source deletion after failed processing, destination truncation, season-pack and multi-disc matching, failed/cancelled torrent cleanup, queue tracking, duplicate grabs, retries, and scanner mismatches.
- Make library/season creation transactional, enforce matching profiles and ratings validation, and prevent accidental downloads when rating music.
- Add Kimi K3/high reasoning support with streamed reasoning, opaque reasoning preservation, complete tool-call history, cancellation, and provider error handling.
- Fix assistant double submission/stale streams, music/import stale responses, keyboard workflows, and mobile setup.
- Upgrade the frontend to a compatible Svelte 5 / SvelteKit 2 / Vite 7 stack, lock Go dependencies, and add backend race/regression and browser tests to CI.
- Fix SQLite connection initialization under concurrent traffic and support current qBittorrent authentication, port-specific cookies, and torrent-add responses.

### Neon Archive UI

- Replace the default Obsidian presentation with the Neon Archive design: a dark opaque canvas, magenta/cyan accents, condensed headings, crisp icon navigation, and cut-corner panels.
- Rebuild Collection with featured media, a music shelf, live downloads, failure alerts, assistant prompts, and import/settings shortcuts, all backed by existing APIs.
- Redesign Discover and shared poster/download components; carry the design tokens into the remaining workflows.
- Add searchable media categories and URL-backed discovery/library filters, including music search and library links.
- Share download polling between the shell, Collection, and Activity, retain queue state on errors, and show failed retry/cancel actions.
- Make all eight navigation destinations reachable on mobile; fix small-screen detail and music-filter overflow.
- Add keyboard-operable poster buttons, a native discovery dialog, visible focus states, reduced-motion support, and locally bundled fonts.
- Preserve saved alternative palettes and existing setup, quality-profile, episode, release, import, rating, and assistant workflows.
- Add Playwright coverage for core flows, empty/error states, mobile layouts, theme persistence, and stale search responses.

### Upgrading and verification

- SQLite migrations run automatically and preserve saved settings, custom profiles, and existing library assignments. Back up private runtime data before upgrading; see [installation notes](docs/installation.md#storage-and-upgrades), including the bundled qBittorrent port change to 9090.
- The completed overhaul passed Go race tests and vet, 38 Playwright tests against the production build, Linux CI, Docker startup/migrations, and live service checks. [Verification details](docs/overhaul-verification.md) distinguish authenticated/search checks from transfer paths that were not exercised with external media payloads.

Earlier releases are documented in the README.
