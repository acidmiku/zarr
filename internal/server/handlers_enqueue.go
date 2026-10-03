package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"mediaforge/internal/indexer"
	"strings"
)

// enqueueRelease reserves a tracking row before submitting to either client.
// A failed submission never leaves the library stuck in downloading state.
func (s *Server) enqueueRelease(mediaID, episodeID, albumID int, rel indexer.Release) (string, error) {
	var media, episode, album any
	if mediaID > 0 {
		media = mediaID
	}
	if episodeID > 0 {
		episode = episodeID
	}
	if albumID > 0 {
		album = albumID
	}
	if rel.DownloadType == "" {
		rel.DownloadType = "nzb"
	}
	if rel.DownloadType != "nzb" && rel.DownloadType != "torrent" {
		return "", fmt.Errorf("unsupported download type")
	}
	if rel.Title == "" {
		rel.Title = "manual-grab"
	}
	if rel.DownloadType == "torrent" && (s.qbt == nil || s.rutracker == nil || rel.TopicID <= 0) {
		return "", fmt.Errorf("torrent client and topic are required")
	}
	if rel.DownloadType == "nzb" && (s.grabber == nil || rel.NZBURL == "") {
		return "", fmt.Errorf("NZB URL and client are required")
	}
	var valid int
	var err error
	if albumID > 0 {
		err = s.db.QueryRow(`SELECT id FROM albums WHERE id=?`, albumID).Scan(&valid)
	} else if episodeID > 0 {
		err = s.db.QueryRow(`SELECT id FROM episodes WHERE id=? AND media_item_id=?`, episodeID, mediaID).Scan(&valid)
	} else {
		err = s.db.QueryRow(`SELECT id FROM media_items WHERE id=?`, mediaID).Scan(&valid)
	}
	if err != nil {
		return "", fmt.Errorf("library item not found")
	}
	quality, _ := json.Marshal(rel.Quality)
	res, err := s.db.Exec(`INSERT INTO downloads (media_item_id,episode_id,album_id,nzb_title,quality,score,download_type)
 SELECT ?,?,?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM downloads WHERE media_item_id IS ? AND episode_id IS ? AND album_id IS ? AND status NOT IN ('imported','failed','cancelled','seeding'))`, media, episode, album, rel.Title, string(quality), rel.Score, rel.DownloadType, media, episode, album)
	if err != nil {
		return "", fmt.Errorf("record download: %w", err)
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return "", fmt.Errorf("a download is already active for this item")
	}
	id, _ := res.LastInsertId()
	submitted := false
	defer func() {
		if !submitted {
			s.db.Exec(`UPDATE downloads SET status='failed' WHERE id=?`, id)
		}
	}()
	nzoID := ""
	if rel.DownloadType == "torrent" {
		idxs := filterIndexersByType(s.loadIndexers(), "rutracker", "")
		if len(idxs) == 0 {
			return "", fmt.Errorf("no torrent indexer configured")
		}
		idx := idxs[0]
		for _, candidate := range idxs {
			if candidate.Name == rel.Indexer {
				idx = candidate
				break
			}
		}
		data, err := s.rutracker.DownloadTorrent(rel.TopicID, idx.Username, idx.Password)
		if err != nil {
			return "", err
		}
		var hash string
		hash, err = s.qbt.AddTorrent(data, fmt.Sprintf("dl_%d.torrent", id), int(id))
		if err != nil {
			return "", err
		}
		if hash != "" {
			if _, err := s.db.Exec(`UPDATE downloads SET qbt_hash=? WHERE id=?`, hash, id); err != nil {
				s.qbt.DeleteTorrent(hash, false)
				return "", err
			}
		}

	} else {
		nzoID, err = s.grabber.GrabNZB(rel.NZBURL, rel.Title)
		if err != nil {
			return "", err
		}
		if _, err = s.db.Exec(`UPDATE downloads SET sabnzbd_nzo_id=? WHERE id=?`, nzoID, id); err != nil {
			s.grabber.DeleteFromQueue(nzoID)
			return "", err
		}
	}
	submitted = true
	if albumID > 0 {
		_, err = s.db.Exec(`UPDATE albums SET status=CASE WHEN status='available' THEN 'available' ELSE 'downloading' END,updated_at=CURRENT_TIMESTAMP WHERE id=?`, albumID)
	} else if episodeID > 0 {
		_, err = s.db.Exec(`UPDATE episodes SET status=CASE WHEN status='available' THEN 'available' ELSE 'downloading' END WHERE id=?`, episodeID)
	} else {
		_, err = s.db.Exec(`UPDATE media_items SET status=CASE WHEN status='available' THEN 'available' ELSE 'downloading' END WHERE id=?`, mediaID)
	}
	if err != nil {
		return "", err
	}
	s.db.Exec(`INSERT INTO activity_log (media_item_id,episode_id,album_id,action,details) VALUES (?,?,?,'grabbed',?)`, media, episode, album, "Grabbed "+rel.Title)
	return nzoID, nil
}

// Record each confirmed cancellation before continuing a multi-download removal.
// A later client or filesystem failure must not make retries cancel it again.
func (s *Server) cancelTrackedDownload(id int, kind string, hash, nzoID sql.NullString) error {
	var status string
	if err := s.db.QueryRow(`SELECT status FROM downloads WHERE id=?`, id).Scan(&status); err != nil {
		return fmt.Errorf("read download state: %w", err)
	}
	if status == "cancelled" {
		return nil
	}
	if err := s.cancelClientDownload(id, kind, hash, nzoID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE downloads SET status='cancelled' WHERE id=?`, id); err != nil {
		return fmt.Errorf("save cancelled download state: %w", err)
	}
	return nil
}

func (s *Server) cancelClientDownload(id int, kind string, hash, nzoID sql.NullString) error {
	if kind == "torrent" {
		if s.qbt == nil {
			return fmt.Errorf("qBittorrent is not configured")
		}
		if !hash.Valid || hash.String == "" {
			torrents, err := s.qbt.GetTorrents()
			if err != nil {
				return err
			}
			for _, t := range torrents {
				for _, tag := range strings.Split(t.Tags, ",") {
					if strings.TrimSpace(tag) == fmt.Sprintf("dl_%d", id) {
						hash = sql.NullString{String: t.Hash, Valid: true}
					}
				}
			}
		}
		if hash.Valid && hash.String != "" {
			return s.qbt.DeleteTorrent(hash.String, true)
		}
		return nil
	}
	if nzoID.Valid && nzoID.String != "" {
		if s.grabber == nil {
			return fmt.Errorf("SABnzbd is not configured")
		}
		return s.grabber.DeleteFromQueue(nzoID.String)
	}
	return nil
}
