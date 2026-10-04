package server

import (
	"database/sql"
	"encoding/json"
	"mediaforge/internal/indexer"
	"mediaforge/internal/music"
	"net/http"
)

func (s *Server) handleDownloadMusicAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	var tracks int
	if err = s.db.QueryRow(`SELECT track_count FROM albums WHERE id=?`, id).Scan(&tracks); err != nil {
		writeError(w, 404, "album not found")
		return
	}
	if tracks < 1 {
		writeError(w, 409, "This album has no track metadata. Remove and save it again after choosing a complete edition.")
		return
	}
	acquisition, err := music.Request(s.db, id, false)
	if err != nil {
		writeError(w, 500, "could not save the download request")
		return
	}
	var monitored bool
	s.db.QueryRow(`SELECT monitored FROM albums WHERE id=?`, id).Scan(&monitored)
	writeJSON(w, 202, map[string]any{"acquisition": acquisition, "monitored": monitored})
}

func (s *Server) handleMonitorMusicAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	var body struct {
		Monitored *bool `json:"monitored"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Monitored == nil {
		writeError(w, 400, "monitored must be true or false")
		return
	}
	var exists int
	if s.db.QueryRow(`SELECT id FROM albums WHERE id=?`, id).Scan(&exists) != nil {
		writeError(w, 404, "album not found")
		return
	}
	if *body.Monitored {
		_, err = s.db.Exec(`UPDATE albums SET monitored=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
		if err == nil {
			_, err = music.Request(s.db, id, true)
		}
	} else {
		err = music.Pause(s.db, id)
	}
	if err != nil {
		writeError(w, 500, "could not update album monitoring")
		return
	}
	writeJSON(w, 200, map[string]any{"monitored": *body.Monitored, "acquisition": music.Snapshot(s.db, id)})
}

func (s *Server) handleFavoriteMusicAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	var body struct {
		Favorite *bool `json:"favorite"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Favorite == nil {
		writeError(w, 400, "favorite must be true or false")
		return
	}
	result, err := s.db.Exec(`UPDATE albums SET favorite=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, *body.Favorite, id)
	if err != nil {
		writeError(w, 500, "could not update favorite")
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		writeError(w, 404, "album not found")
		return
	}
	writeJSON(w, 200, map[string]any{"favorite": *body.Favorite})
}

// A saved album stays saved unless the caller explicitly asks for acquisition.
func (s *Server) musicSaveResponse(w http.ResponseWriter, id int, status int, downloadNow bool) {
	response := map[string]any{"id": id}
	if downloadNow {
		acquisition, err := music.Request(s.db, id, false)
		if err != nil && err != sql.ErrNoRows {
			response["warning"] = "Album saved, but the download request could not be queued. Retry from the album."
		} else {
			response["acquisition"] = acquisition
		}
	}
	writeJSON(w, status, response)
}

func (s *Server) musicIdentity(id int) (indexer.MusicIdentity, int, int, error) {
	var identity indexer.MusicIdentity
	var year, profileID int
	var edition, mbid, aliases string
	err := s.db.QueryRow(`SELECT a.name,al.title,COALESCE(al.year,0),COALESCE(al.quality_profile_id,0),al.album_type,al.edition_title,al.edition_disambiguation,COALESCE(a.mbid,'') FROM albums al JOIN artists a ON a.id=al.artist_id WHERE al.id=?`, id).
		Scan(&identity.Artist, &identity.Album, &year, &profileID, &identity.AlbumType, &edition, &identity.Edition, &mbid)
	if edition != "" && edition != identity.Album {
		identity.AlbumAliases = []string{identity.Album}
		identity.Album = edition
	}
	if s.db.QueryRow(`SELECT aliases FROM music_catalog_artists WHERE mbid=?`, mbid).Scan(&aliases) == nil {
		json.Unmarshal([]byte(aliases), &identity.ArtistAliases)
	}
	return identity, year, profileID, err
}
