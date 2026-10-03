# Changelog

## Unreleased

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
