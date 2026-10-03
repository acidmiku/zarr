# Anime quality profiles

Zarr includes separate presets based on the [Sonarr anime guide](https://trash-guides.info/Sonarr/sonarr-setup-quality-profiles-anime/) and [Radarr anime guide](https://trash-guides.info/Radarr/radarr-setup-quality-profiles-anime/). They use the actual upstream profile and referenced custom-format definitions pinned at revision `e7c97a676743d7430fc1c5808701c48252a2ac63`, rather than approximating release tiers with generic tag bonuses.

## Getting started

New anime series use **Anime Series · TRaSH**; new anime films use **Anime Movies · TRaSH**. An explicit profile choice takes precedence. Existing profiles and library assignments remain unchanged. To enable this scoring for an existing title, change its profile on its Library detail page.

In **Settings → Quality profiles**, edit a preset to choose a dual-audio preference. Advanced scoring exposes quality groups, minimum score, upgrade targets, and individual custom-format score overrides. Custom video and music profiles continue to use the original scoring engine.

## Selection rules

Quality groups rank first. Only releases within the same group compete on custom-format score. A large format bonus cannot make a lower-quality group outrank a higher one. Each candidate must also meet the minimum score and any explicit reject patterns.

The upstream defaults include:

- 1080p Blu-ray and remux in the same highest group, followed by 1080p WEB/HDTV, Blu-ray 720p, WEB/HDTV 720p, then the enabled SD groups. Radarr also includes Blu-ray 576p. 2160p is not enabled by these anime presets.
- Minimum custom-format score **100**. Unrecognized releases can fall below this threshold; that is intentional. Review the explanation before lowering it.
- Quality upgrade target **Blu-ray 1080p**, and custom-format upgrade target **10000**.
- **41 Sonarr** and **31 Radarr** custom formats, covering the guide's release-group tiers, revision and service preferences, and negative formats such as low-quality groups, AV1, VOSTFR, raws and dubs-only.

The matcher implements required, optional and negated conditions, grouped by specification type. Title/group regexes use a bounded .NET-compatible engine for upstream expressions. Source, language and remux specifications are evaluated too; unsupported future specification types fail validation rather than silently passing.

Release search results expose the quality rank, format total, matched format names and contributions, rejection reason, preset, and snapshot version. The UI preserves server ranking.

## Dual audio

| Choice | Behavior |
| --- | --- |
| Optional | Guide default; the dual-audio format has no bonus unless you override it |
| Prefer within tier | +10 for the dual-audio format |
| Prefer above tier | +101 for the dual-audio format |
| Required | +2000 and an explicit requirement that the dual-audio format matches |

These preferences apply within a quality group. They do not lower a custom minimum score. Named preferences take precedence over the generic dual-audio score override while enabled.

Audio matching is based on release titles and supplied language metadata. An unspecified “Dual Audio” label is interpreted as Japanese plus English in these anime presets. This is a search heuristic, not verification of the downloaded tracks or English subtitles.

## Automatic upgrades

With **Allow upgrades** enabled, available anime items assigned to the matching preset are checked in batches of up to ten every 30 minutes, with at least six hours between checks of the same item. Active downloads are excluded. Mapped anime episodes search both absolute numbering and season/episode numbering, merging duplicate results.

The current release title is retained on import. Older successful download history can supply a baseline; unknown quality or missing files cause automatic upgrades to be skipped. This avoids guessing that an unidentifiable file is inferior.

**Scan Library clears release provenance for every matched file.** A matching path cannot prove that the file has not been replaced outside Zarr, and file identities are not stored. Scanned files remain Available, but automatic upgrades wait for a verified import to establish their release provenance again. An explicit unknown marker prevents an older download from becoming their baseline.

The scheduler compares the current release and candidate under the current profile. It respects both upgrade cutoffs and never replaces a higher quality group with a lower one. The importer checks again before replacement, so a changed profile or better intervening import can invalidate the queued job. Existing items remain Available during an upgrade, cancellation, or failure. File staging rolls back failed imports, and an old canonical file with a different extension is retired only after the new import commits.

If an automatic import fails but its downloaded source still exists, retry that local import or cancel it before another automatic grab. This prevents repeatedly downloading a release that is already on disk.

## Compatibility and maintenance

This implements the pinned anime profile and custom-format selection rules within Zarr. It does not make Zarr a complete Sonarr/Radarr implementation. Remaining differences include MediaInfo verification of audio/subtitles, size-per-minute limits, full release-information filename templates, and safely splitting combined multi-episode files. Ambiguous multi-episode imports still retain their source for manual handling. Title parsing may differ from Sonarr/Radarr on unusual release names.

Definitions are bundled locally; no guide fetch is needed while searching. Upstream changes are reviewed and vendored with `python scripts/update-trash-presets.py <full-reviewed-commit-sha>`, followed by the indexer regression suite and a review of source mappings and profile changes. The upstream MIT license and a SHA-256 manifest accompany the data in `internal/indexer/trash`. Saved custom profiles are not automatically reassigned or reset by the updater.

The [earlier audit](anime-quality-audit.md) records the baseline that motivated this work.
