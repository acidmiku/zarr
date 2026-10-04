package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mediaforge/internal/metadata"
	"net/http"
	"os"
	"regexp"
	"time"
)

var musicArtworkID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Cover resolution and image bytes are cached separately: every card for the
// same MusicBrainz group reuses a verified provider choice and the disk image.
func (s *Server) serveMusicArtwork(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("rgid")
	if !musicArtworkID.MatchString(id) {
		writeError(w, 400, "valid release-group ID required")
		return
	}
	if s.imgCache == nil {
		writeError(w, 503, "image cache unavailable")
		return
	}
	key := "music-cover:v1:" + id
	var saved string
	var resolution struct {
		URL     string `json:"url"`
		Missing bool   `json:"missing"`
	}
	if s.db.QueryRow(`SELECT data FROM cache WHERE cache_key=? AND datetime(expires_at)>datetime('now')`, key).Scan(&saved) == nil {
		json.Unmarshal([]byte(saved), &resolution)
	}
	if resolution.URL != "" && allowedImageURL(resolution.URL) {
		if cached := s.imgCache.get(resolution.URL); cached != nil {
			serveCached(w, r, cached)
			return
		}
	}
	if resolution.Missing {
		writeError(w, 404, "no verified cover available")
		return
	}
	// Honor previously downloaded Cover Art Archive files from older installations.
	if s.coverart != nil && s.coverart.HasCover(id) {
		path := s.coverart.CoverPath(id)
		if info, err := os.Stat(path); err == nil && info.Size() > 0 && info.Size() <= 20<<20 && validLegacyCover(path) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeFile(w, r, path)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 24*time.Second)
	defer cancel()
	request := r.WithContext(ctx)
	save := func(rawURL string) {
		data, _ := json.Marshal(map[string]string{"url": rawURL})
		s.db.Exec(`INSERT INTO cache(cache_key,data,expires_at) VALUES(?,?,datetime('now','+10 years')) ON CONFLICT(cache_key) DO UPDATE SET data=excluded.data,expires_at=excluded.expires_at`, key, string(data))
	}
	try := func(rawURL string, budget time.Duration) bool {
		if rawURL == "" || !allowedImageURL(rawURL) {
			return false
		}
		if cached := s.imgCache.get(rawURL); cached != nil {
			save(rawURL)
			serveCached(w, request, cached)
			return true
		}
		if s.imgCache.isNegative(rawURL) {
			return false
		}
		fetchCtx, fetchCancel := context.WithTimeout(ctx, budget)
		defer fetchCancel()
		result := s.imgCache.fetchOrJoin(fetchCtx, s.proxyClient, rawURL)
		if result.status != http.StatusOK {
			return false
		}
		save(rawURL)
		if cached := s.imgCache.get(rawURL); cached != nil {
			serveCached(w, request, cached)
		} else {
			w.Header().Set("Content-Type", result.ct)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write(result.data)
		}
		return true
	}
	if try(resolution.URL, 6*time.Second) {
		return
	}
	if try(fmt.Sprintf("https://coverartarchive.org/release-group/%s/front-250", id), 6*time.Second) {
		return
	}
	var artist, title string
	s.db.QueryRow(`SELECT artist,title FROM music_catalog_releases WHERE release_group_id=?`, id).Scan(&artist, &title)
	if artist == "" {
		s.db.QueryRow(`SELECT a.name,al.title FROM albums al JOIN artists a ON a.id=al.artist_id WHERE al.release_group_id=?`, id).Scan(&artist, &title)
	}
	if artist == "" && s.musicbrainz != nil {
		if group, err := s.musicbrainz.GetReleaseGroupContext(ctx, id); err == nil && len(group.ArtistCredit) > 0 {
			artist, title = group.ArtistCredit[0].Artist.Name, group.Title
		}
	}
	if alternate, err := metadata.ResolveFallbackCover(ctx, s.proxyClient, s.db, artist, title); err == nil && try(alternate, 8*time.Second) {
		return
	}
	// A thumbnail may be absent while original artwork exists.
	if try(fmt.Sprintf("https://coverartarchive.org/release-group/%s/front", id), 4*time.Second) {
		return
	}
	// Brief negative resolution cache prevents a grid repeatedly searching absent
	// art. It expires quickly enough to recover from a temporary provider outage.
	if ctx.Err() == nil {
		s.db.Exec(`INSERT INTO cache(cache_key,data,expires_at) VALUES(?,'{"missing":true}',datetime('now','+10 minutes')) ON CONFLICT(cache_key) DO UPDATE SET data=excluded.data,expires_at=excluded.expires_at`, key)
	}
	writeError(w, 404, "no verified cover available")
}

// Earlier versions cached arbitrary HTTP bodies as .jpg. Do not promote a
// cached HTML/error page to executable same-origin content when upgrading.
func validLegacyCover(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	data := make([]byte, 512)
	n, err := io.ReadFull(file, data)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false
	}
	switch http.DetectContentType(data[:n]) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}
