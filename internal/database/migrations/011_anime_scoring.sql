-- Existing profiles and assignments retain legacy scoring until explicitly changed.
ALTER TABLE quality_profiles ADD COLUMN scoring_config TEXT NOT NULL DEFAULT '{}';
ALTER TABLE media_items ADD COLUMN current_release_title TEXT;
ALTER TABLE media_items ADD COLUMN upgrade_checked_at DATETIME;
ALTER TABLE episodes ADD COLUMN current_release_title TEXT;
ALTER TABLE episodes ADD COLUMN upgrade_checked_at DATETIME;
ALTER TABLE downloads ADD COLUMN upgrade_from_title TEXT;

INSERT OR IGNORE INTO quality_profiles
    (name, qualities, tags, language, reject_patterns, upgrade_allowed, profile_type, scoring_config)
VALUES
    ('Anime Series · TRaSH', '["remux-1080p","bluray-1080p","hdtv-1080p","web-1080p","bluray-720p","hdtv-720p","web-720p","bluray-480p","web-480p","dvd","sdtv"]', '{}', 'any', '[]', TRUE, 'video', '{"preset":"sonarr-anime"}'),
    ('Anime Movies · TRaSH', '["remux-1080p","bluray-1080p","hdtv-1080p","web-1080p","bluray-720p","hdtv-720p","web-720p","bluray-576p","bluray-480p","web-480p","dvd","sdtv"]', '{}', 'any', '[]', TRUE, 'video', '{"preset":"radarr-anime"}');
