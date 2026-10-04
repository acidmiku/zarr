-- Preserve existing monitoring intent; new saves are a wishlist until requested.
ALTER TABLE albums ADD COLUMN monitored INTEGER NOT NULL DEFAULT 0;
UPDATE albums SET monitored = 1;
ALTER TABLE albums ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0;
ALTER TABLE albums ADD COLUMN edition_title TEXT NOT NULL DEFAULT '';
ALTER TABLE albums ADD COLUMN edition_disambiguation TEXT NOT NULL DEFAULT '';
ALTER TABLE downloads ADD COLUMN error_message TEXT NOT NULL DEFAULT '';

CREATE TABLE music_search_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued',
    message TEXT NOT NULL DEFAULT '',
    automatic INTEGER NOT NULL DEFAULT 0,
    after_download_id INTEGER NOT NULL DEFAULT 0,
    download_id INTEGER REFERENCES downloads(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX music_one_pending_search ON music_search_requests(album_id)
    WHERE status IN ('queued','searching');
CREATE INDEX music_search_history ON music_search_requests(album_id,id DESC);

CREATE TABLE music_catalog_artists (
    mbid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sort_name TEXT NOT NULL DEFAULT '',
    aliases TEXT NOT NULL DEFAULT '[]',
    genres TEXT NOT NULL DEFAULT '[]',
    metadata TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE music_catalog_releases (
    release_group_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    artist TEXT NOT NULL,
    artist_mbid TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    release_type TEXT NOT NULL DEFAULT 'Album',
    genres TEXT NOT NULL DEFAULT '[]',
    metadata TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX music_catalog_artist ON music_catalog_releases(artist_mbid);
