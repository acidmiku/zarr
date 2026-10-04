# Music discovery and downloads

Zarr keeps a local MusicBrainz catalog and manages music directly through its configured indexers and download clients. It does not require Lidarr, Prowlarr, Soularr or slskd.

## Save, download, or monitor

- **Save to library** keeps an album for later. New saves do not download anything unless you explicitly enable monitoring.
- **Download now** requests a single acquisition. Repeated clicks reuse a pending search or an active transfer.
- **Monitor album** searches now and periodically checks for a matching release while the album is missing. Turning monitoring off stops pending searches and leaves an already submitted transfer running. Cancel that transfer in Activity when needed.
- Existing albums keep their previous automatic monitoring behavior after upgrading. You can turn it off per album.
- **Favorite** and ratings shape discovery without starting downloads.

Choose a music quality profile when saving. Profiles list the formats you actually allow: a FLAC-only profile will not silently accept an unidentified codec. The existing default profile also allows MP3-320 and V0 as fallbacks; edit it if you require lossless files.

Music searches distinguish a release group (the album) from a release (a specific edition and tracklist). Zarr defaults to an official standard digital/CD edition, rather than the edition with the most bonus tracks. You can choose another edition before saving. Existing saved tracklists are preserved.

## Discovery and catalog

Search artists, albums or tracks. Artist pages include albums, EPs and singles, with complete paginated discographies instead of the former 100-album limit. Locally cached artist aliases and release metadata improve repeat searches and remain usable during upstream outages. Expired metadata can be refreshed without deleting the last usable snapshot.

The music home page recommends albums using saved artists, favorites, ratings and shared genres, with a reason on each card. Saved albums are excluded from recommendations. Equally relevant albums appear before EPs and singles; broadcasts and miscellaneous recordings remain in artist browsing. Metadata enrichment happens in the background, so a new or small library can take a short time to populate its shelves.

Last.fm is optional. Its API key enables similar-artist discovery and chart suggestions; an optional username in Settings enables a **Played but not owned** shelf from listening history when the artist and album uniquely match the local catalog. Unknown history entries are not guessed into the library. External album names are resolved to verified MusicBrainz release groups before acquisition. Ambiguous artist/title matches require a more specific search.

## Artwork caching

Cover images use the backend's persistent disk cache and browser caching. Missing Cover Art Archive artwork can fall back to Deezer or iTunes only after the artist and album title match. A verified provider choice is saved too, avoiding repeated metadata lookups. Missing artwork is briefly cached to prevent repeated requests; a missing cover never blocks saving or downloading an album.

The configured proxy applies to music metadata and image requests. No API credentials are embedded in image URLs returned to the browser.

## Search and import safety

Before automatic selection, candidates must match the requested artist, album and edition. Discographies, different versions, unrelated artists and unidentified codecs do not win just because they appear in indexer results. Searches can relax the release year and try the formats allowed by the profile.

Search requests survive restarts and expose separate states for queued searches, no acceptable results, unavailable sources, transfers, imports and failures. A finished transfer is not marked Available until the importer verifies the album's tracks.

The same validation is used for completed downloads and manual music imports:

- Read embedded artist, album, title, disc and track tags, plus MusicBrainz IDs when present.
- Verify the selected tracklist and actual codec family. A lossless-only profile rejects a lossy file even if its release title says FLAC.
- Use an untagged filename only when its track title and numbering provide an unambiguous match. Numeric filenames alone are insufficient.
- Validate the entire album before changing library files. Reject duplicate, mismatched and incomplete tracks, preserving the source for correction or retry.
- Preserve torrent source files for seeding and retain transaction/rollback protection during import.

Use **Retry acquisition** for a failed attempt. If downloaded files still exist, Zarr retries their import first. If there are no retained files, it saves a fresh search request. Source failures are shown separately from an empty successful search.

## Current limits

- Source support remains Newznab/Usenet and the existing RuTracker/qBittorrent integration. Prowlarr/Torznab and Soulseek adapters are separate future work.
- Strict matching can reject unusually named releases or poorly tagged files. Zarr favors a visible mismatch over silently renaming the wrong audio.
- Codec/container and FLAC bit-depth checks do not perform acoustic fingerprinting, full audio decoding or independent verification of lossy bitrates.
- An existing saved edition is not automatically replaced when metadata changes. Re-add an unacquired album with the desired edition if its old tracklist is incorrect.
- Music quality upgrades of an already available album are not automated by this pass.

## Verification (2026-10-04)

- `go test -race ./...` and `go vet ./...` pass, including migration, concurrent acquisition, cancellation, source selection, import rollback and persistent-cache regressions.
- The production UI build and all 55 browser tests pass, including 14 music scenarios. Desktop and 320px mobile views were inspected.
- The rebuilt Docker app was checked with live MusicBrainz search and edition metadata, canonical album resolution, a 14-track save with no queued search or download, and favorite changes.
- Cover bytes and ETags remained identical across a container restart; conditional requests returned HTTP 304. Live Last.fm artwork also passed through the disk cache.
- Transfer selection and tagged imports were tested with controlled clients and audio fixtures. This pass did not download a full commercial album from the configured providers.
