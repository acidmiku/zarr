-- Release blacklist: tracks failed releases so they are not re-grabbed.
CREATE TABLE IF NOT EXISTS release_blacklist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_item_id INTEGER REFERENCES media_items(id) ON DELETE CASCADE,
    episode_id INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    album_id INTEGER REFERENCES albums(id) ON DELETE CASCADE,
    release_title TEXT NOT NULL,
    reason TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_blacklist_media ON release_blacklist(media_item_id);
CREATE INDEX IF NOT EXISTS idx_blacklist_episode ON release_blacklist(episode_id);
CREATE INDEX IF NOT EXISTS idx_blacklist_album ON release_blacklist(album_id);
