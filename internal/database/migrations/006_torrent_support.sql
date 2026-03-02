-- Torrent support: indexer types, qBittorrent integration

-- Indexer type and credentials
ALTER TABLE indexers ADD COLUMN type TEXT NOT NULL DEFAULT 'newznab';
ALTER TABLE indexers ADD COLUMN username TEXT;
ALTER TABLE indexers ADD COLUMN password TEXT;
ALTER TABLE indexers ADD COLUMN content_types TEXT NOT NULL DEFAULT '["movie","series","anime","music"]';

-- Downloads: torrent support
ALTER TABLE downloads ADD COLUMN download_type TEXT NOT NULL DEFAULT 'nzb';
ALTER TABLE downloads ADD COLUMN qbt_hash TEXT;
ALTER TABLE downloads ADD COLUMN seed_ratio REAL;
ALTER TABLE downloads ADD COLUMN completed_at_ts INTEGER;
