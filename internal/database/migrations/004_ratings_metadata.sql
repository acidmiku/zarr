-- Store title/poster/year/anime directly on ratings so they survive library deletion.
ALTER TABLE user_ratings ADD COLUMN title TEXT;
ALTER TABLE user_ratings ADD COLUMN poster_url TEXT;
ALTER TABLE user_ratings ADD COLUMN year INTEGER;
ALTER TABLE user_ratings ADD COLUMN anime BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill existing ratings from current library items.
UPDATE user_ratings SET
    title = (SELECT m.title FROM media_items m WHERE m.tmdb_id = user_ratings.tmdb_id AND m.type = user_ratings.media_type),
    poster_url = (SELECT m.poster_url FROM media_items m WHERE m.tmdb_id = user_ratings.tmdb_id AND m.type = user_ratings.media_type),
    year = (SELECT m.year FROM media_items m WHERE m.tmdb_id = user_ratings.tmdb_id AND m.type = user_ratings.media_type),
    anime = COALESCE((SELECT m.anime FROM media_items m WHERE m.tmdb_id = user_ratings.tmdb_id AND m.type = user_ratings.media_type), 0);
