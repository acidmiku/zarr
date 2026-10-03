# Anime quality audit

Reviewed 2026-10-04 against the current [Sonarr anime guide](https://trash-guides.info/Sonarr/sonarr-setup-quality-profiles-anime/) and [Radarr anime guide](https://trash-guides.info/Radarr/radarr-setup-quality-profiles-anime/).

**Assessment: Zarr supports basic anime identification and release selection, but its scoring is not TRaSH-equivalent.** A percentage would suggest a compatibility test that does not exist. Both guides aim for the best overall release and use grouped qualities plus custom formats, including ranked release groups; Zarr has a flat quality list and a small set of title tags.

| Area | Zarr behavior | Assessment |
| --- | --- | --- |
| Movie / series identity | Separate `type=movie` / `type=series`, with an independent anime flag; discovery and UI now preserve both | Fixed in this pass |
| Episode search | Season/episode and absolute-number paths; mapped anime uses an absolute query, rather than always issuing both queries. Tightened matching prevents a movie's audio-channel numbers becoming episode numbers | Partial alignment with Sonarr |
| Quality ordering | Seeded Anime profile gives remux 300, Blu-ray 200, WEB 100 before bonuses. HDTV and 720p are excluded | Does not express the guides' quality groups |
| Release groups | No group tier lists, regex conditions, source constraints, or group maintenance | Major gap |
| Preferences | Default bonuses: 10bit +10, dual audio +15, uncensored +5 | Similar attributes, different scoring model |
| Revisions and streaming source | No anime v0/v1/v2 scoring or streaming-service tie breakers | Missing |
| Rejections | Whole-word title rejection for cam/ts/dubbed/dub/raw; no maintained low-quality group lists, AV1 or VOSTFR defaults | Partial and sometimes overbroad |
| Language / subtitles | `ja-en` rejects raw/dub words; other unlabelled releases pass. No inspection proving Japanese audio or English subtitles. Multi-audio is tagged like dual audio | Weak heuristic |
| Thresholds | No separate custom-format score, minimum score, or quality/score upgrade cutoff. A negative total alone does not make a release unacceptable | Missing |
| Automatic upgrades | `upgrade_allowed` is stored; `ShouldUpgrade` has no callers. Scheduled searches select wanted items, not available items needing upgrades | Missing operational behavior |
| Size / naming | No per-minute size limits. Anime filenames keep season/episode/absolute numbers but omit quality, codec, languages and group; combined multi-episode imports fail safely | Partial |

The guides treat qualities in groups and use custom formats to decide within them. Optional dual-audio preference can be strengthened or required with a minimum custom-format score. Their upgrade settings have both quality and score targets. These cannot be replicated merely by pasting their scores into Zarr's tag map. See the linked [Sonarr](https://trash-guides.info/Sonarr/sonarr-setup-quality-profiles-anime/#quality-profile) and [Radarr](https://trash-guides.info/Radarr/radarr-setup-quality-profiles-anime/#quality-profile) configurations.

## Reproduced examples

Synthetic names were evaluated through `ParseReleaseName` and `ScoreRelease` using the seeded Anime profile:

| Release attributes | Actual result |
| --- | --- |
| 1080p BluRay Remux | Accepted, 300 |
| 1080p BluRay, 10bit, Dual Audio | Accepted, 225 |
| Same release labelled BD instead of BluRay | Previously WEB/125; fixed to Blu-ray/225, with regression tests |
| 1080p WEB-DL episode 01v0 versus 01v2 | Both 100; revision ignored |
| 1080p WEB-DL AV1 VOSTFR | Accepted, 100 |
| 1080p WEB-DL Dual Audio English Dub | Rejected by the default `dub` word, even though dual audio is stated |

These are parser/scorer probes, not claims about any group's real releases or the contents of downloaded files.

## Next scoring work

Implement a separate, versioned anime profile system with quality groups and explicit custom-format conditions/scores; preserve user profiles during migration. Then add minimum scores, upgrade cutoffs and a scheduler that evaluates available items. Add fixtures comparing actual selection decisions with the guide, including original audio, dual audio, revisions and blocked groups. Group definitions need a documented update mechanism and provenance; naming a profile “TRaSH” without that behavior would be misleading.

Code reviewed: `internal/indexer/parser.go`, `scorer.go`, `newznab.go`; `internal/server/handlers_profiles.go`; `internal/scheduler/scheduler.go`; `internal/postprocess/templates.go`; seeded profiles in migration 001.
