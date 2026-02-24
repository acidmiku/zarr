-- Settings table (key-value store for configuration)
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Quality profiles
CREATE TABLE IF NOT EXISTS quality_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    qualities TEXT NOT NULL,
    tags TEXT,
    language TEXT NOT NULL DEFAULT 'en',
    reject_patterns TEXT,
    upgrade_allowed BOOLEAN DEFAULT TRUE
);

-- Core content
CREATE TABLE IF NOT EXISTS media_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL CHECK (type IN ('movie', 'series')),
    title TEXT NOT NULL,
    year INTEGER,
    anime BOOLEAN NOT NULL DEFAULT FALSE,
    tmdb_id INTEGER,
    imdb_id TEXT,
    anidb_id INTEGER,
    anilist_id INTEGER,
    tvdb_id INTEGER,
    overview TEXT,
    poster_url TEXT,
    backdrop_url TEXT,
    genres TEXT,
    tags TEXT,
    rating REAL,
    rating_source TEXT,
    status TEXT NOT NULL DEFAULT 'wanted'
        CHECK (status IN ('wanted', 'searching', 'downloading', 'available', 'unavailable')),
    quality_profile_id INTEGER REFERENCES quality_profiles(id),
    root_path TEXT,
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tmdb_id, type)
);

CREATE TABLE IF NOT EXISTS seasons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    title TEXT,
    overview TEXT,
    poster_url TEXT,
    UNIQUE(media_item_id, number)
);

CREATE TABLE IF NOT EXISTS episodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    media_item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    absolute_number INTEGER,
    episode_type TEXT NOT NULL DEFAULT 'standard'
        CHECK (episode_type IN ('standard', 'special', 'ova', 'ona', 'recap')),
    title TEXT,
    overview TEXT,
    air_date DATE,
    status TEXT NOT NULL DEFAULT 'wanted'
        CHECK (status IN ('wanted', 'searching', 'downloading', 'available')),
    file_path TEXT,
    UNIQUE(season_id, number)
);

-- Indexer configuration
CREATE TABLE IF NOT EXISTS indexers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    priority INTEGER DEFAULT 0,
    enabled BOOLEAN DEFAULT TRUE
);

-- Download tracking
CREATE TABLE IF NOT EXISTS downloads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_item_id INTEGER NOT NULL REFERENCES media_items(id),
    episode_id INTEGER REFERENCES episodes(id),
    nzb_title TEXT NOT NULL,
    sabnzbd_nzo_id TEXT,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'downloading', 'extracting', 'completed', 'failed', 'imported')),
    quality TEXT,
    score INTEGER,
    download_path TEXT,
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

-- Activity log
CREATE TABLE IF NOT EXISTS activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_item_id INTEGER REFERENCES media_items(id),
    episode_id INTEGER REFERENCES episodes(id),
    action TEXT NOT NULL,
    details TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Seed default quality profiles
INSERT OR IGNORE INTO quality_profiles (name, qualities, language, tags, reject_patterns, upgrade_allowed)
VALUES
    ('HD Series', '["bluray-1080p","web-1080p","hdtv-1080p","web-720p"]', 'en', '{}', '["cam","ts","hdts","telecine","dvdscr"]', TRUE),
    ('HD Movies', '["remux-1080p","bluray-1080p","web-1080p"]', 'en', '{"imax-enhanced":10}', '["cam","ts","hdts","telecine","dvdscr"]', TRUE),
    ('Anime', '["remux-1080p","bluray-1080p","web-1080p"]', 'ja-en', '{"10bit":10,"dual-audio":15,"uncensored":5}', '["cam","ts","dubbed","dub","raw"]', TRUE);
