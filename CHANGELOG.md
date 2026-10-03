# Changelog

## Unreleased

### Core reliability and setup

- Replace the blocking setup wizard with quick start and reusable connection forms; manage keys, editable indexers, and SABnzbd Usenet servers in Settings.
- Automatically wire fresh bundled SABnzbd/qBittorrent installations with unique credentials and shared paths. Preserve existing downloader configuration.
- Preserve blank secret edits, redact API responses, validate settings atomically, isolate connection tests, and apply proxy/client changes without restarting.
- Fix premature successful imports, source deletion after failed processing, destination truncation, season-pack and multi-disc matching, failed/cancelled torrent cleanup, queue tracking, duplicate grabs, retries, and scanner mismatches.
- Make library/season creation transactional, enforce matching profiles and ratings validation, and prevent accidental downloads when rating music.
- Add Kimi K3/high reasoning support with streamed reasoning, opaque reasoning preservation, complete tool-call history, cancellation, and provider error handling.
- Fix assistant double submission/stale streams, music/import stale responses, keyboard workflows, and mobile setup.
- Upgrade the frontend to a compatible Svelte 5 / SvelteKit 2 / Vite 7 stack, lock Go dependencies, and add backend race/regression and browser tests to CI.

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

Earlier releases are documented in the README.
