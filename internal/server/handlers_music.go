package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"mediaforge/internal/indexer"
	"mediaforge/internal/metadata"
	"mediaforge/internal/music"
)

// -- Discovery --

type albumResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	ArtistID  string `json:"artist_id"`
	Year      string `json:"year"`
	Type      string `json:"type"`
	CoverURL  string `json:"cover_url"`
	InLibrary bool   `json:"in_library"`
	LibraryID int    `json:"library_id,omitempty"`
	TrackName string `json:"track_name,omitempty"` // populated for track search
}

func (s *Server) handleMusicSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, 400, "q parameter required")
		return
	}

	searchType := r.URL.Query().Get("type") // "artist", "album" (default), or "track"
	if searchType == "" {
		searchType = "album"
	}

	if s.musicbrainz == nil {
		writeError(w, 503, "MusicBrainz client not initialized")
		return
	}
	if r.URL.Query().Get("local") == "true" {
		switch searchType {
		case "artist":
			writeMusicArtists(w, s.musicbrainz.SearchCatalogArtists(q, 25))
		case "track":
			writeJSON(w, 200, []albumResult{})
		default:
			writeJSON(w, 200, s.releaseGroupsToAlbumResults(s.musicbrainz.SearchCatalogReleaseGroups(q, 50), ""))
		}
		return
	}

	switch searchType {
	case "artist":
		s.searchMusicByArtist(w, q)
	case "track":
		s.searchMusicByTrack(w, q)
	default:
		s.searchMusicByAlbum(w, q)
	}
}

func (s *Server) searchMusicByArtist(w http.ResponseWriter, q string) {
	result, err := s.musicbrainz.SearchArtists(q)
	if err != nil {
		writeError(w, 502, "MusicBrainz search failed: "+err.Error())
		return
	}
	writeMusicArtists(w, result.Artists)
}

func writeMusicArtists(w http.ResponseWriter, source []metadata.MBArtist) {

	type artistResult struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		SortName string `json:"sort_name"`
		Type     string `json:"type"`
		Country  string `json:"country"`
		Score    int    `json:"score"`
	}

	var artists []artistResult
	for _, a := range source {
		artists = append(artists, artistResult{
			ID:       a.ID,
			Name:     a.Name,
			SortName: a.SortName,
			Type:     a.Type,
			Country:  a.Country,
			Score:    a.Score,
		})
	}
	if artists == nil {
		artists = []artistResult{}
	}
	writeJSON(w, 200, artists)
}

func (s *Server) searchMusicByAlbum(w http.ResponseWriter, q string) {
	result, err := s.musicbrainz.SearchReleaseGroups(q)
	if err != nil {
		writeError(w, 502, "MusicBrainz search failed: "+err.Error())
		return
	}

	albums := s.releaseGroupsToAlbumResults(result.ReleaseGroups, "")
	writeJSON(w, 200, albums)
}

func (s *Server) searchMusicByTrack(w http.ResponseWriter, q string) {
	result, err := s.musicbrainz.SearchRecordings(q)
	if err != nil {
		writeError(w, 502, "MusicBrainz search failed: "+err.Error())
		return
	}

	// Deduplicate by release group — show unique albums containing the track
	seen := map[string]bool{}
	var albums []albumResult
	for _, rec := range result.Recordings {
		for _, rel := range rec.Releases {
			if rel.ReleaseGroup == nil {
				continue
			}
			rgID := rel.ReleaseGroup.ID
			if seen[rgID] {
				continue
			}
			seen[rgID] = true
			ar := albumResult{
				ID:        rgID,
				Title:     rel.ReleaseGroup.Title,
				Year:      rel.Date,
				Type:      rel.ReleaseGroup.PrimaryType,
				CoverURL:  "/api/music/cover?rgid=" + rgID,
				TrackName: rec.Title,
			}
			if len(rec.ArtistCredit) > 0 {
				ar.Artist = rec.ArtistCredit[0].Artist.Name
				ar.ArtistID = rec.ArtistCredit[0].Artist.ID
			}
			// Check if in library
			var libID int
			if s.db.QueryRow(`SELECT id FROM albums WHERE release_group_id = ?`, rgID).Scan(&libID) == nil {
				ar.InLibrary = true
				ar.LibraryID = libID
			}
			albums = append(albums, ar)
		}
	}
	if albums == nil {
		albums = []albumResult{}
	}
	writeJSON(w, 200, albums)
}

func (s *Server) releaseGroupsToAlbumResults(groups []metadata.MBReleaseGroup, trackName string) []albumResult {
	var albums []albumResult
	for _, rg := range groups {
		ar := albumResult{
			ID:        rg.ID,
			Title:     rg.Title,
			Year:      rg.FirstRelease,
			Type:      rg.PrimaryType,
			TrackName: trackName,
		}
		if len(rg.ArtistCredit) > 0 {
			ar.Artist = rg.ArtistCredit[0].Artist.Name
			ar.ArtistID = rg.ArtistCredit[0].Artist.ID
		}
		ar.CoverURL = "/api/music/cover?rgid=" + rg.ID

		var libID int
		if s.db.QueryRow(`SELECT id FROM albums WHERE release_group_id = ?`, rg.ID).Scan(&libID) == nil {
			ar.InLibrary = true
			ar.LibraryID = libID
		}

		albums = append(albums, ar)
	}
	if albums == nil {
		albums = []albumResult{}
	}
	return albums
}

func (s *Server) handleMusicTrending(w http.ResponseWriter, r *http.Request) {
	if s.lastfm == nil {
		writeError(w, 503, "Last.fm not configured")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	result, err := s.lastfm.TopAlbums(page)
	if err != nil {
		writeError(w, 502, "Last.fm error: "+err.Error())
		return
	}

	type trendingAlbum struct {
		Provider   string `json:"provider"`
		Name       string `json:"name"`
		Artist     string `json:"artist"`
		ArtistMBID string `json:"artist_mbid"`
		MBID       string `json:"mbid"`
		ImageURL   string `json:"image_url"`
		Playcount  string `json:"playcount"`
	}

	var albums []trendingAlbum
	for _, a := range result.Albums.Album {
		albums = append(albums, trendingAlbum{
			Provider:   "lastfm",
			Name:       a.Name,
			Artist:     a.Artist.Name,
			ArtistMBID: a.Artist.MBID,
			MBID:       a.MBID,
			ImageURL:   metadata.BestImage(a.Image),
			Playcount:  a.Playcount,
		})
	}

	if albums == nil {
		albums = []trendingAlbum{}
	}
	writeJSON(w, 200, albums)
}

func (s *Server) handleMusicArtist(w http.ResponseWriter, r *http.Request) {
	mbid := r.PathValue("mbid")
	if mbid == "" {
		writeError(w, 400, "mbid required")
		return
	}

	if s.musicbrainz == nil {
		writeError(w, 503, "MusicBrainz client not initialized")
		return
	}

	artist, err := s.musicbrainz.GetArtist(mbid)
	if err != nil {
		writeError(w, 502, "MusicBrainz error: "+err.Error())
		return
	}

	// Get albums
	albums, err := s.musicbrainz.SearchArtistReleaseGroups(mbid)
	if err != nil {
		writeError(w, 502, "MusicBrainz error: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"artist": artist,
		"albums": albums.ReleaseGroups,
	})
}

func (s *Server) handleMusicAlbum(w http.ResponseWriter, r *http.Request) {
	rgid := r.PathValue("rgid")
	if rgid == "" {
		writeError(w, 400, "rgid required")
		return
	}

	if s.musicbrainz == nil {
		writeError(w, 503, "MusicBrainz client not initialized")
		return
	}

	rg, err := s.musicbrainz.GetReleaseGroup(rgid)
	if err != nil {
		writeError(w, 502, "MusicBrainz error: "+err.Error())
		return
	}

	// Get best release with tracks
	var release *metadata.MBRelease
	var errRelease error
	if selected := r.URL.Query().Get("release_id"); selected != "" {
		release, errRelease = s.musicbrainz.GetReleaseForGroup(rgid, selected)
		if errRelease != nil {
			writeError(w, http.StatusUnprocessableEntity, "Selected edition could not be verified for this album: "+errRelease.Error())
			return
		}
	} else {
		release, errRelease = s.musicbrainz.GetBestRelease(rgid)
	}
	err = errRelease
	if err != nil {
		slog.Warn("no releases found", "rgid", rgid, "error", err)
	}

	editions, _ := s.musicbrainz.GetReleasesForGroup(rgid)
	writeJSON(w, 200, map[string]interface{}{
		"release_group": rg,
		"release":       release,
		"editions":      editions,
	})
}

// -- Library Management --

func (s *Server) handleAddMusicToLibrary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReleaseGroupID string `json:"release_group_id"`
		ArtistMBID     string `json:"artist_mbid"`
		ArtistName     string `json:"artist_name"`
		AlbumTitle     string `json:"album_title"`
		Year           int    `json:"year"`
		AlbumType      string `json:"album_type"`
		ProfileID      int    `json:"quality_profile_id"`
		ReleaseID      string `json:"release_id"`
		Monitored      bool   `json:"monitored"`
		DownloadNow    bool   `json:"download_now"`
		ImageURL       string `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if strings.TrimSpace(req.ReleaseGroupID) == "" || strings.TrimSpace(req.ArtistMBID) == "" || strings.TrimSpace(req.ArtistName) == "" || strings.TrimSpace(req.AlbumTitle) == "" {
		writeError(w, 400, "release_group_id, artist_mbid, artist_name, and album_title required")
		return
	}

	if req.AlbumType == "" {
		req.AlbumType = "album"
	}

	var err error
	req.ProfileID, err = s.resolveProfile(req.ProfileID, "music")
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	// Idempotent saves should remain usable when metadata is temporarily offline.
	var alreadySaved int
	if s.db.QueryRow(`SELECT id FROM albums WHERE release_group_id=?`, req.ReleaseGroupID).Scan(&alreadySaved) == nil {
		if req.Monitored {
			if _, err = s.db.Exec(`UPDATE albums SET monitored=1 WHERE id=?`, alreadySaved); err != nil {
				writeError(w, 500, "could not enable monitoring")
				return
			}
		}
		s.musicSaveResponse(w, alreadySaved, 200, req.DownloadNow || req.Monitored)
		return
	}
	// Fetch complete metadata before opening a transaction or creating an album.
	var release *metadata.MBRelease
	if s.musicbrainz != nil {
		rg, lookupErr := s.musicbrainz.GetReleaseGroup(req.ReleaseGroupID)
		if lookupErr != nil || len(rg.ArtistCredit) == 0 {
			writeError(w, 502, "Could not verify album and artist identity with MusicBrainz.")
			return
		}
		req.AlbumTitle, req.ArtistMBID, req.ArtistName = rg.Title, rg.ArtistCredit[0].Artist.ID, rg.ArtistCredit[0].Artist.Name
		req.AlbumType = rg.PrimaryType
		if len(rg.FirstRelease) >= 4 {
			req.Year, _ = strconv.Atoi(rg.FirstRelease[:4])
		}
		req.ImageURL = "/api/music/cover?rgid=" + req.ReleaseGroupID
		if req.ReleaseID != "" {
			release, err = s.musicbrainz.GetReleaseForGroup(req.ReleaseGroupID, req.ReleaseID)
		} else {
			release, err = s.musicbrainz.GetBestRelease(req.ReleaseGroupID)
		}
		if err != nil {
			writeError(w, 502, "failed to fetch album tracks: "+err.Error())
			return
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer tx.Rollback()
	// Check if already in library
	var existing int
	if err := tx.QueryRow(`SELECT id FROM albums WHERE release_group_id = ?`, req.ReleaseGroupID).Scan(&existing); err == nil {
		if req.Monitored {
			if _, err = tx.Exec(`UPDATE albums SET monitored=1 WHERE id=?`, existing); err != nil {
				writeError(w, 500, "could not enable monitoring")
				return
			}
			if err = tx.Commit(); err != nil {
				writeError(w, 500, "could not save monitoring")
				return
			}
		} else {
			tx.Rollback()
		}
		s.musicSaveResponse(w, existing, 200, req.DownloadNow || req.Monitored)
		return
	}

	// Find or create artist
	var artistID int
	err = tx.QueryRow(`SELECT id FROM artists WHERE mbid = ?`, req.ArtistMBID).Scan(&artistID)
	if err != nil {
		result, err := tx.Exec(`INSERT INTO artists (mbid, name, sort_name) VALUES (?, ?, ?)`,
			req.ArtistMBID, req.ArtistName, req.ArtistName)
		if err != nil {
			writeError(w, 500, "create artist: "+err.Error())
			return
		}
		id, _ := result.LastInsertId()
		artistID = int(id)
	}

	// Create album
	result, err := tx.Exec(`INSERT INTO albums (artist_id, release_group_id, title, year, album_type, quality_profile_id, image_url, status,monitored)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'wanted',?)`,
		artistID, req.ReleaseGroupID, req.AlbumTitle, req.Year, req.AlbumType, req.ProfileID, req.ImageURL, req.Monitored)
	if err != nil {
		writeError(w, 500, "create album: "+err.Error())
		return
	}
	albumID, _ := result.LastInsertId()

	if release != nil {
		trackCount := 0
		for _, media := range release.Media {
			for _, track := range media.Tracks {
				if _, err := tx.Exec(`INSERT INTO tracks (album_id,mbid,title,number,disc_number,duration_ms) VALUES (?,?,?,?,?,?)`, albumID, track.Recording.ID, track.Title, track.Position, media.Position, track.Length); err != nil {
					writeError(w, 500, "failed to save tracks")
					return
				}
				trackCount++
			}
		}
		if _, err := tx.Exec(`UPDATE albums SET track_count = ?, mbid = ?,edition_title=?,edition_disambiguation=? WHERE id = ?`, trackCount, release.ID, release.Title, release.Disambiguation, albumID); err != nil {
			writeError(w, 500, "failed to save album tracks")
			return
		}
	}

	// Fetch and cache cover art

	// Log activity
	tx.Exec(`INSERT INTO activity_log (album_id, action, details) VALUES (?, 'added', ?)`,
		albumID, fmt.Sprintf("Added %s - %s", req.ArtistName, req.AlbumTitle))

	if err := tx.Commit(); err != nil {
		writeError(w, 500, "failed to save album")
		return
	}
	s.musicSaveResponse(w, int(albumID), 201, req.DownloadNow || req.Monitored)
}

func (s *Server) handleListMusicLibrary(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	query := `SELECT al.id, al.title, al.year, al.album_type, al.status, al.release_group_id, al.image_url,
		al.track_count, al.rating, al.rating_comment, a.name as artist_name, a.mbid as artist_mbid,al.monitored,al.favorite
		FROM albums al JOIN artists a ON al.artist_id = a.id`
	var args []interface{}

	if status != "" && status != "all" {
		query += ` WHERE al.status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY al.created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type libraryAlbum struct {
		ID             int               `json:"id"`
		Title          string            `json:"title"`
		Year           sql.NullInt64     `json:"-"`
		YearVal        int               `json:"year"`
		AlbumType      string            `json:"album_type"`
		Status         string            `json:"status"`
		ReleaseGroupID string            `json:"release_group_id"`
		ImageURL       sql.NullString    `json:"-"`
		ImageURLVal    string            `json:"image_url"`
		TrackCount     int               `json:"track_count"`
		Rating         sql.NullInt64     `json:"-"`
		RatingVal      int               `json:"rating,omitempty"`
		RatingComment  sql.NullString    `json:"-"`
		RatingCommentV string            `json:"rating_comment,omitempty"`
		ArtistName     string            `json:"artist_name"`
		ArtistMBID     string            `json:"artist_mbid"`
		Monitored      bool              `json:"monitored"`
		Favorite       bool              `json:"favorite"`
		Acquisition    music.Acquisition `json:"acquisition"`
	}

	var albums []libraryAlbum
	for rows.Next() {
		var a libraryAlbum
		if err := rows.Scan(&a.ID, &a.Title, &a.Year, &a.AlbumType, &a.Status, &a.ReleaseGroupID,
			&a.ImageURL, &a.TrackCount, &a.Rating, &a.RatingComment, &a.ArtistName, &a.ArtistMBID, &a.Monitored, &a.Favorite); err != nil {
			slog.Warn("music album scan failed", "error", err)
			continue
		}
		if a.Year.Valid {
			a.YearVal = int(a.Year.Int64)
		}
		if a.ImageURL.Valid {
			a.ImageURLVal = a.ImageURL.String
		}
		if a.Rating.Valid {
			a.RatingVal = int(a.Rating.Int64)
		}
		if a.RatingComment.Valid {
			a.RatingCommentV = a.RatingComment.String
		}
		albums = append(albums, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "database error")
		return
	}

	rows.Close()
	for i := range albums {
		albums[i].Acquisition = music.Snapshot(s.db, albums[i].ID)
	}
	if albums == nil {
		albums = []libraryAlbum{}
	}
	writeJSON(w, 200, albums)
}

func (s *Server) handleListMusicArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT a.id, a.mbid, a.name, a.image_url,
		COUNT(al.id) as album_count,
		SUM(CASE WHEN al.status = 'available' THEN 1 ELSE 0 END) as available_count
		FROM artists a JOIN albums al ON a.id = al.artist_id
		GROUP BY a.id ORDER BY a.name ASC`)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer rows.Close()

	type libraryArtist struct {
		ID             int            `json:"id"`
		MBID           sql.NullString `json:"-"`
		MBIDVal        string         `json:"mbid"`
		Name           string         `json:"name"`
		ImageURL       sql.NullString `json:"-"`
		ImageURLVal    string         `json:"image_url"`
		AlbumCount     int            `json:"album_count"`
		AvailableCount int            `json:"available_count"`
	}

	var artists []libraryArtist
	for rows.Next() {
		var a libraryArtist
		if err := rows.Scan(&a.ID, &a.MBID, &a.Name, &a.ImageURL, &a.AlbumCount, &a.AvailableCount); err != nil {
			slog.Warn("artist scan failed", "error", err)
			continue
		}
		if a.MBID.Valid {
			a.MBIDVal = a.MBID.String
		}
		if a.ImageURL.Valid {
			a.ImageURLVal = a.ImageURL.String
		}
		artists = append(artists, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "database error")
		return
	}
	if artists == nil {
		artists = []libraryArtist{}
	}
	writeJSON(w, 200, artists)
}

func (s *Server) handleGetMusicLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var album struct {
		ID             int               `json:"id"`
		Title          string            `json:"title"`
		Year           sql.NullInt64     `json:"-"`
		YearVal        int               `json:"year"`
		AlbumType      string            `json:"album_type"`
		Status         string            `json:"status"`
		ReleaseGroupID string            `json:"release_group_id"`
		ImageURL       sql.NullString    `json:"-"`
		ImageURLVal    string            `json:"image_url"`
		TrackCount     int               `json:"track_count"`
		Rating         sql.NullInt64     `json:"-"`
		RatingVal      int               `json:"rating,omitempty"`
		RatingComment  sql.NullString    `json:"-"`
		RatingCommentV string            `json:"rating_comment,omitempty"`
		ArtistName     string            `json:"artist_name"`
		ArtistMBID     string            `json:"artist_mbid"`
		Monitored      bool              `json:"monitored"`
		Favorite       bool              `json:"favorite"`
		Acquisition    music.Acquisition `json:"acquisition"`
		ProfileID      sql.NullInt64     `json:"-"`
		ProfileIDVal   int               `json:"quality_profile_id"`
	}

	err = s.db.QueryRow(`SELECT al.id, al.title, al.year, al.album_type, al.status, al.release_group_id,
		al.image_url, al.track_count, al.rating, al.rating_comment, a.name, a.mbid, al.quality_profile_id,al.monitored,al.favorite
		FROM albums al JOIN artists a ON al.artist_id = a.id WHERE al.id = ?`, id).Scan(
		&album.ID, &album.Title, &album.Year, &album.AlbumType, &album.Status, &album.ReleaseGroupID,
		&album.ImageURL, &album.TrackCount, &album.Rating, &album.RatingComment, &album.ArtistName,
		&album.ArtistMBID, &album.ProfileID, &album.Monitored, &album.Favorite)
	if err != nil {
		writeError(w, 404, "album not found")
		return
	}

	if album.Year.Valid {
		album.YearVal = int(album.Year.Int64)
	}
	if album.ImageURL.Valid {
		album.ImageURLVal = album.ImageURL.String
	}
	if album.Rating.Valid {
		album.RatingVal = int(album.Rating.Int64)
	}
	if album.RatingComment.Valid {
		album.RatingCommentV = album.RatingComment.String
	}
	if album.ProfileID.Valid {
		album.ProfileIDVal = int(album.ProfileID.Int64)
	}

	album.Acquisition = music.Snapshot(s.db, id)
	// Get tracks
	trackRows, err := s.db.Query(`SELECT id, title, number, disc_number, duration_ms, status, file_path
		FROM tracks WHERE album_id = ? ORDER BY disc_number, number`, id)
	if err != nil {
		writeJSON(w, 200, album)
		return
	}
	defer trackRows.Close()

	type trackEntry struct {
		ID         int            `json:"id"`
		Title      string         `json:"title"`
		Number     int            `json:"number"`
		DiscNumber int            `json:"disc_number"`
		DurationMS int            `json:"duration_ms"`
		Status     string         `json:"status"`
		FilePath   sql.NullString `json:"-"`
		HasFile    bool           `json:"has_file"`
	}

	var tracks []trackEntry
	for trackRows.Next() {
		var t trackEntry
		trackRows.Scan(&t.ID, &t.Title, &t.Number, &t.DiscNumber, &t.DurationMS, &t.Status, &t.FilePath)
		t.HasFile = t.FilePath.Valid && t.FilePath.String != ""
		tracks = append(tracks, t)
	}
	if tracks == nil {
		tracks = []trackEntry{}
	}

	writeJSON(w, 200, map[string]interface{}{
		"album":  album,
		"tracks": tracks,
	})
}

func (s *Server) handleDeleteMusicLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	var artistID int
	if err := s.db.QueryRow(`SELECT artist_id FROM albums WHERE id = ?`, id).Scan(&artistID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "album not found")
		} else {
			writeError(w, 500, "database error")
		}
		return
	}
	rows, err := s.db.Query(`SELECT id,download_type,COALESCE(sabnzbd_nzo_id,''),COALESCE(qbt_hash,'') FROM downloads WHERE album_id = ? AND status NOT IN ('imported','failed','cancelled') ORDER BY id`, id)
	if err != nil {
		writeError(w, 500, "failed to load album downloads")
		return
	}
	type transfer struct {
		id              int
		kind, nzb, hash string
	}
	var transfers []transfer
	for rows.Next() {
		var transfer transfer
		if err := rows.Scan(&transfer.id, &transfer.kind, &transfer.nzb, &transfer.hash); err != nil {
			rows.Close()
			writeError(w, 500, "failed to read downloads")
			return
		}
		transfers = append(transfers, transfer)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		writeError(w, 500, "failed to read downloads")
		return
	}
	for _, t := range transfers {
		if err := s.cancelTrackedDownload(t.id, t.kind, sql.NullString{String: t.hash, Valid: t.hash != ""}, sql.NullString{String: t.nzb, Valid: t.nzb != ""}); err != nil {
			writeError(w, 502, "failed to cancel album download: "+err.Error())
			return
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM downloads WHERE album_id = ?`, `UPDATE activity_log SET album_id = NULL WHERE album_id = ?`, `DELETE FROM albums WHERE id = ?`} {
		if _, err := tx.Exec(query, id); err != nil {
			writeError(w, 500, "failed to delete album")
			return
		}
	}
	if _, err := tx.Exec(`DELETE FROM artists WHERE id = ? AND NOT EXISTS (SELECT 1 FROM albums WHERE artist_id = ?)`, artistID, artistID); err != nil {
		writeError(w, 500, "failed to clean up artist")
		return
	}
	if _, err := tx.Exec(`INSERT INTO activity_log(action,details) VALUES ('deleted',?)`, fmt.Sprintf("Removed album ID %d from music library", id)); err != nil {
		writeError(w, 500, "failed to save activity")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "failed to delete album")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleSearchMusicAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	identity, year, profileID, err := s.musicIdentity(id)
	if err != nil {
		writeError(w, 404, "album not found")
		return
	}
	profile := s.loadProfile(profileID)
	if profile == nil {
		writeError(w, 409, "Choose a music quality profile first.")
		return
	}
	allIndexers := s.loadIndexers()
	newznabIdxs := filterIndexersByType(allIndexers, "newznab", "music")
	rtIndexers := filterIndexersByType(allIndexers, "rutracker", "music")
	if len(newznabIdxs)+len(rtIndexers) == 0 {
		writeError(w, 409, "No music sources are enabled. Configure a music indexer in Settings.")
		return
	}
	releases := []indexer.Release{}
	var searchErrors int
	if len(newznabIdxs) > 0 && s.newznab != nil {
		result := s.newznab.SearchMusicAlbum(newznabIdxs, identity, year, profile)
		releases = append(releases, result.Releases...)
		searchErrors = len(result.Errors)
	}
	if len(rtIndexers) > 0 && s.rutracker != nil {
		query := fmt.Sprintf("%s %s", identity.Artist, identity.Album)
		releases = append(releases, s.searchRutracker(rtIndexers, query, "music")...)
	}
	for i := range releases {
		indexer.ScoreMusicRelease(&releases[i], profile, identity)
	}
	if len(releases) == 0 && searchErrors > 0 {
		writeError(w, 502, "Music sources could not be searched. Check their connections in Settings and retry.")
		return
	}

	writeJSON(w, 200, releases)
}

func (s *Server) handleRateMusicAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var req struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		writeError(w, 400, "rating must be between 1 and 5")
		return
	}
	result, err := s.db.Exec(`UPDATE albums SET rating = ?, rating_comment = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		req.Rating, req.Comment, id)
	if err != nil {
		writeError(w, 500, "database error")
		return
	}

	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, 404, "album not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// -- Releases --

func (s *Server) handleMusicReleases(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	// Alias to search
	r.URL.RawQuery = fmt.Sprintf("id=%d", id)
	s.handleSearchMusicAlbum(w, r)
}

func (s *Server) handleGrabMusicRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReleaseURL   string `json:"release_url"`
		AlbumID      int    `json:"album_id"`
		DownloadType string `json:"download_type"`
		TopicID      int    `json:"topic_id"`
		Title        string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AlbumID <= 0 {
		writeError(w, 400, "valid album_id and JSON required")
		return
	}
	identity, _, profileID, err := s.musicIdentity(req.AlbumID)
	if err != nil {
		writeError(w, 404, "album not found")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, 400, "Release title is required to verify the artist and album.")
		return
	}
	parsed := indexer.ParseMusicReleaseName(req.Title)
	release := indexer.Release{Title: req.Title, NZBURL: req.ReleaseURL, DownloadType: req.DownloadType, TopicID: req.TopicID, Quality: parsed.Quality, Tags: parsed.Tags}
	indexer.ScoreMusicRelease(&release, s.loadProfile(profileID), identity)
	if !release.Acceptable {
		writeError(w, 400, release.RejectReason)
		return
	}
	nzoID, err := s.enqueueRelease(0, 0, req.AlbumID, release)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "grabbed", "nzo_id": nzoID})
}

// -- Cover Art --

func (s *Server) handleMusicCover(w http.ResponseWriter, r *http.Request) { s.serveMusicArtwork(w, r) }

// -- Helpers --
// loadIndexers is defined in handlers_episodes.go

func (s *Server) loadProfile(profileID int) *indexer.QualityProfile {
	var profile indexer.QualityProfile
	err := s.db.QueryRow(`SELECT id, name, qualities, COALESCE(tags,'{}'), language, COALESCE(reject_patterns,'[]'), upgrade_allowed, COALESCE(profile_type, 'video'), scoring_config
		FROM quality_profiles WHERE id = ?`, profileID).Scan(
		&profile.ID, &profile.Name, &profile.Qualities, &profile.Tags,
		&profile.Language, &profile.RejectPatterns, &profile.UpgradeAllowed, &profile.ProfileType, &profile.ScoringConfig)
	if err != nil {
		return nil
	}
	return &profile
}
