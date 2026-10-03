-- Enforce new identities without deleting or invalidating historical duplicates.
CREATE TRIGGER media_anilist_unique_insert BEFORE INSERT ON media_items
WHEN NEW.anilist_id IS NOT NULL AND NEW.anilist_id > 0 AND EXISTS (
 SELECT 1 FROM media_items WHERE anilist_id = NEW.anilist_id
)
BEGIN SELECT RAISE(ABORT, 'UNIQUE constraint failed: media_items.anilist_id'); END;
CREATE TRIGGER media_anilist_unique_update BEFORE UPDATE OF anilist_id ON media_items
WHEN NEW.anilist_id IS NOT NULL AND NEW.anilist_id > 0 AND EXISTS (
 SELECT 1 FROM media_items WHERE anilist_id = NEW.anilist_id AND id != NEW.id
)
BEGIN SELECT RAISE(ABORT, 'UNIQUE constraint failed: media_items.anilist_id'); END;
CREATE TRIGGER album_release_group_unique_insert BEFORE INSERT ON albums
WHEN NEW.release_group_id IS NOT NULL AND NEW.release_group_id != '' AND EXISTS (
 SELECT 1 FROM albums WHERE release_group_id = NEW.release_group_id
)
BEGIN SELECT RAISE(ABORT, 'UNIQUE constraint failed: albums.release_group_id'); END;
CREATE TRIGGER album_release_group_unique_update BEFORE UPDATE OF release_group_id ON albums
WHEN NEW.release_group_id IS NOT NULL AND NEW.release_group_id != '' AND EXISTS (
 SELECT 1 FROM albums WHERE release_group_id = NEW.release_group_id AND id != NEW.id
)
BEGIN SELECT RAISE(ABORT, 'UNIQUE constraint failed: albums.release_group_id'); END;
CREATE INDEX IF NOT EXISTS idx_media_anilist ON media_items(anilist_id);
CREATE INDEX IF NOT EXISTS idx_album_release_group ON albums(release_group_id);
