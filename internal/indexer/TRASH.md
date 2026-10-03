# Pinned TRaSH anime scoring

MediaForge embeds the **complete custom-format sets referenced by** TRaSH's
Sonarr and Radarr `anime-remux-1080p` profiles at commit
`e7c97a676743d7430fc1c5808701c48252a2ac63`: 41 Sonarr definitions and 31 Radarr
definitions. The unmodified source JSON, upstream MIT license, SHA-256 manifest,
and derived bundle are in [`trash/`](trash/). No runtime guide download is needed.

Source repository: <https://github.com/TRaSH-Guides/Guides>

The separate presets use each profile's `trash_score_set`, including its anime
Remux scores. Qualities and default minimum/cutoff scores come from the pinned
profile, not from an approximate hand-maintained tier list. Members of each
quality group compare equally. Groups are ordered best first. WEB-DL and WEBRip
share MediaForge's `web-*` quality IDs, but their source inputs remain distinct
for custom-format evaluation.

## Matching and ranking

The evaluator groups specifications by `implementation`, ANDs the groups, and
applies negation before group evaluation. Each group must have all required
conditions satisfied and at least one condition satisfied. The last requirement
includes required conditions: an optional condition does not also need to match
when a required one already matches. This follows Sonarr's
[calculation service](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/CustomFormats/CustomFormatCalculationService.cs),
[group evaluator](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/CustomFormats/SpecificationMatchesGroup.cs),
and [negation implementation](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/CustomFormats/Specifications/CustomFormatSpecificationBase.cs).

All specification implementations present in the snapshot are supported:
release title, release group, source, quality modifier, and language. Source and
modifier enum numbers are mapped separately for Sonarr and Radarr. An unknown
implementation prevents the bundle from loading; it is never silently skipped.

Upstream patterns are compiled unchanged with `github.com/dlclark/regexp2`
v1.11.5 in case-insensitive .NET mode. This preserves negative lookaheads,
lookbehinds, and backreferences unsupported by Go's standard regexp engine.
Every regex has a 50 ms timeout (the library checks its clock approximately every
100 ms), and input titles/groups are capped at 4,096 bytes. A timeout rejects the
release, including for negated patterns; it never converts a failed match into
acceptance. Only the pinned patterns are executable: profile overrides contain
integer scores, not arbitrary regexes.

Ranking is `(quality group rank, custom format score)`. Format scores cannot
override quality-group order. `Score` remains a compatibility display field;
callers must use `CompareReleases`/`BestRelease`. `IsUpgrade` requires an acceptable
candidate and enabled upgrades, forbids lower quality groups, stops quality
upgrades at their cutoff, and stops equal-group format upgrades at their score
cutoff. Both releases must be rescored using their original release names and the
same current profile. This follows the relevant quality/format checks in
[Sonarr's upgrade specification](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/DecisionEngine/Specifications/UpgradableSpecification.cs).
There is no separate automatic proper/repack override; the upstream v0-v4 custom
formats provide revision scores and the profile's upgrade switch remains binding.

## Configuration and compatibility

An empty string or `{}` means the original scoring formula and comparator.
Legacy tag bonuses, language checks, and ranking remain unchanged in that mode.
TRaSH presets use the custom-format rules instead of the old blanket language
filter or additive tag bonuses; explicit user reject patterns still apply.

`{"preset":"sonarr-anime"}` and `{"preset":"radarr-anime"}` expand to full
defaults. Explicit zeros are preserved. `format_scores` overrides are merged by
upstream TRaSH ID; unknown IDs, quality IDs, controls, and nulls are rejected.
`DefaultScoringConfig`, `ResolveScoringConfig`, `ValidateScoringConfig`, and
`ScoringPresets` are the shared API/UI contract. The preset endpoint includes
the source revision and all formats; scored releases expose matched IDs, names,
and actual applied scores.

`dual_audio` supports `optional`, `within-tier` (+10), `above-tier` (+101), and
`required` (+2000 plus a mandatory actual dual-audio custom-format match). These
bonuses follow [TRaSH's preference guidance](https://trash-guides.info/Sonarr/sonarr-setup-quality-profiles-anime/#dual-audio-scoring).
They apply only during evaluation, preserving declared `format_scores` when a
user changes modes. The minimum format score is independent; a large unrelated
bonus cannot satisfy the explicit `required` check. Higher quality groups always
remain preferred.

## Explicit limits

This is **TRaSH-aligned title scoring**, not a full Sonarr/Radarr metadata parser
or MediaInfo track probe. Release quality/source is parsed from the name unless
the caller already supplied it; resolution-only names retain MediaForge's WEB
fallback. Release groups use bracketed prefix or scene suffix conventions.
Explicit `Release.Languages` values override title inference. Without them, named
Japanese/Chinese/Korean language markers are used; an otherwise unspecified
`Dual Audio` marker is interpreted as original Japanese for these anime presets.
Those labels do not establish that actual files contain particular audio tracks
or English subtitles. Contradictory single-language markers are rejected by the
upstream dual-audio conditions. Original-language metadata and unknown audio are
not fabricated as verified track information.

There is no SeaDex lookup or per-title release-group override, and no import of
unrelated TRaSH quality-size limits, indexer flags, or other profile families.
The full supported condition set is tested against the bundled profile data.

## Updating the snapshot

1. Review a specific commit in TRaSH-Guides/Guides; use its full 40-character SHA.
2. Run `python scripts/update-trash-presets.py <sha>` from the repository root.
   The script downloads only that revision, reads archive entries without
   extracting arbitrary paths, and vendors the two profiles plus every
   referenced custom format and the upstream license. It writes an integrity
   manifest; obsolete snapshot files should be removed after reviewing the diff.
3. Review upstream changes, scores, new specification kinds, and regex changes.
   Update the pinned revision/count assertions in `trash_test.go`. An unfamiliar
   rule must be implemented and tested or the update must remain unmerged.
4. Run `go test -race ./internal/indexer ./internal/server ./internal/scheduler ./internal/postprocess`
   and `go vet ./...`. Keep the new snapshot, manifest, license, script, and tests
   together in the review. Update the revision in this document.

The updater does not rewrite user profiles or contact download clients.
