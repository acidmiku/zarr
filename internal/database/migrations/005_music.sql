-- Music support: artists, albums, tracks, cache

CREATE TABLE IF NOT EXISTS artists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mbid TEXT UNIQUE,
    name TEXT NOT NULL,
    sort_name TEXT,
    overview TEXT,
    image_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS albums (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artist_id INTEGER NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    mbid TEXT,
    release_group_id TEXT,
    title TEXT NOT NULL,
    year INTEGER,
    album_type TEXT DEFAULT 'album',
    status TEXT DEFAULT 'wanted',
    quality_profile_id INTEGER REFERENCES quality_profiles(id),
    root_path TEXT,
    overview TEXT,
    image_url TEXT,
    rating INTEGER,
    rating_comment TEXT,
    track_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    mbid TEXT,
    title TEXT NOT NULL,
    number INTEGER NOT NULL,
    disc_number INTEGER DEFAULT 1,
    duration_ms INTEGER,
    status TEXT DEFAULT 'wanted',
    file_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cache (
    cache_key TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    expires_at DATETIME NOT NULL
);

-- Add album_id to downloads
ALTER TABLE downloads ADD COLUMN album_id INTEGER REFERENCES albums(id);

-- Add album_id to activity_log
ALTER TABLE activity_log ADD COLUMN album_id INTEGER REFERENCES albums(id);

-- Add profile_type to quality_profiles
ALTER TABLE quality_profiles ADD COLUMN profile_type TEXT DEFAULT 'video';

-- Seed "Lossless Music" quality profile
INSERT INTO quality_profiles (name, qualities, tags, language, reject_patterns, upgrade_allowed, profile_type)
VALUES (
    'Lossless Music',
    '["flac-24bit","flac","mp3-320","mp3-v0"]',
    '{}',
    'any',
    '[]',
    TRUE,
    'music'
);
