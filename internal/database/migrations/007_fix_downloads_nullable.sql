-- Fix: allow media_item_id to be NULL for music downloads

CREATE TABLE downloads_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_item_id INTEGER REFERENCES media_items(id),
    episode_id INTEGER REFERENCES episodes(id),
    nzb_title TEXT NOT NULL,
    sabnzbd_nzo_id TEXT,
    status TEXT NOT NULL DEFAULT 'queued',
    quality TEXT,
    score INTEGER,
    download_path TEXT,
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    album_id INTEGER REFERENCES albums(id),
    download_type TEXT NOT NULL DEFAULT 'nzb',
    qbt_hash TEXT,
    seed_ratio REAL,
    completed_at_ts INTEGER
);

INSERT INTO downloads_new SELECT
    id, media_item_id, episode_id, nzb_title, sabnzbd_nzo_id, status,
    quality, score, download_path, started_at, completed_at,
    album_id, download_type, qbt_hash, seed_ratio, completed_at_ts
FROM downloads;

DROP TABLE downloads;
ALTER TABLE downloads_new RENAME TO downloads;
