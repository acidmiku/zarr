package scheduler

import (
	"encoding/json"
	"fmt"
	"mediaforge/internal/indexer"
	"mediaforge/internal/music"
	"mediaforge/internal/rutracker"
)

func (s *Scheduler) searchWantedAlbums() {
	rows, err := s.db.Query(`SELECT al.id FROM albums al WHERE al.status='wanted' AND al.monitored=1
  AND NOT EXISTS(SELECT 1 FROM downloads d WHERE d.album_id=al.id AND d.status='failed' AND COALESCE(d.download_path,'')!='')
  AND NOT EXISTS(SELECT 1 FROM music_search_requests r WHERE r.album_id=al.id AND r.created_at>datetime('now','-30 minutes')) ORDER BY al.id`)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		music.Request(s.db, id, true)
	}
}

func (s *Scheduler) processMusicRequests() {
	rows, err := s.db.Query(`SELECT id,album_id FROM music_search_requests WHERE status='queued' ORDER BY id LIMIT 20`)
	if err != nil {
		return
	}
	type request struct{ id, album int }
	var requests []request
	for rows.Next() {
		var r request
		if rows.Scan(&r.id, &r.album) == nil {
			requests = append(requests, r)
		}
	}
	rows.Close()
	for _, r := range requests {
		if s.ctx != nil && s.ctx.Err() != nil {
			return
		}
		s.runMusicRequest(r.id, r.album)
	}
}

func (s *Scheduler) runMusicRequest(requestID, albumID int) {
	claimed, err := s.db.Exec(`UPDATE music_search_requests SET status='searching',message='Searching music sources.',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, requestID)
	if err != nil {
		return
	}
	n, _ := claimed.RowsAffected()
	if n != 1 {
		return
	}
	defer func() {
		music.Finish(s.db, requestID, "failed", "The music search was interrupted. You can retry it.", 0)
	}()
	var identity indexer.MusicIdentity
	var year, profileID, trackCount int
	var status, artistID, aliases, edition, editionNote string
	var monitored, automatic bool
	err = s.db.QueryRow(`SELECT a.name,al.title,COALESCE(al.year,0),COALESCE(al.quality_profile_id,0),al.album_type,al.status,al.monitored,r.automatic,COALESCE(a.mbid,''),al.edition_title,al.edition_disambiguation,al.track_count
  FROM albums al JOIN artists a ON a.id=al.artist_id JOIN music_search_requests r ON r.album_id=al.id WHERE al.id=? AND r.id=?`, albumID, requestID).
		Scan(&identity.Artist, &identity.Album, &year, &profileID, &identity.AlbumType, &status, &monitored, &automatic, &artistID, &edition, &editionNote, &trackCount)
	if err != nil {
		return
	}
	if status == "available" {
		music.Finish(s.db, requestID, "available", "Album is already available.", 0)
		return
	}
	if automatic && !monitored {
		music.Finish(s.db, requestID, "cancelled", "Album monitoring is off.", 0)
		return
	}
	if edition != "" && edition != identity.Album {
		identity.AlbumAliases = []string{identity.Album}
		identity.Album = edition
	}
	identity.Edition = editionNote
	if s.db.QueryRow(`SELECT aliases FROM music_catalog_artists WHERE mbid=?`, artistID).Scan(&aliases) == nil {
		json.Unmarshal([]byte(aliases), &identity.ArtistAliases)
	}
	var active int
	if s.db.QueryRow(`SELECT id FROM downloads WHERE album_id=? AND status NOT IN ('imported','failed','cancelled','seeding') LIMIT 1`, albumID).Scan(&active) == nil {
		music.Finish(s.db, requestID, "download_queued", "A download is already active for this album.", active)
		return
	}
	s.db.Exec(`UPDATE albums SET status='searching' WHERE id=? AND status='wanted'`, albumID)
	defer s.db.Exec(`UPDATE albums SET status='wanted' WHERE id=? AND status='searching'`, albumID)
	if trackCount < 1 {
		music.Finish(s.db, requestID, "blocked", "This saved album has no complete track metadata. Re-add it with a complete edition before downloading.", 0)
		return
	}
	profile := s.loadProfile(profileID)
	if profile == nil || profile.ProfileType != "music" {
		music.Finish(s.db, requestID, "blocked", "Choose a music quality profile before downloading.", 0)
		return
	}
	sources := s.loadIndexers()
	nzbs := filterIndexersByType(sources, "newznab", "music")
	torrents := filterIndexersByType(sources, "rutracker", "music")
	if len(nzbs)+len(torrents) == 0 {
		music.Finish(s.db, requestID, "blocked", "No music sources are enabled. Add or enable a music indexer in Settings.", 0)
		return
	}
	var releases []indexer.Release
	failures := 0
	qbtEnabled, _ := s.db.GetSetting("qbittorrent_enabled")
	if len(torrents) > 0 && (s.qbt == nil || qbtEnabled != "true") {
		failures++
		torrents = nil
	}
	if len(nzbs) > 0 {
		if s.newznab == nil {
			failures += len(nzbs)
		} else {
			result := s.newznab.SearchMusicAlbum(nzbs, identity, year, profile)
			releases = append(releases, result.Releases...)
			if len(result.Errors) > 0 {
				failures++
			}
		}
	}
	for _, source := range torrents {
		if s.rtClient == nil {
			failures++
			continue
		}
		results, err := s.rtClient.Search(fmt.Sprintf("%s %s", identity.Artist, identity.Album), rutracker.DefaultForumIDs["music"], source.Username, source.Password)
		if err != nil {
			failures++
			continue
		}
		for _, r := range results {
			parsed := indexer.ParseMusicReleaseName(r.Title)
			releases = append(releases, indexer.Release{Title: r.Title, Size: r.Size, Quality: parsed.Quality, Tags: parsed.Tags, Indexer: source.Name, DownloadType: "torrent", Seeders: r.Seeders, Leechers: r.Leechers, TopicID: r.TopicID})
		}
	}
	releases = indexer.FilterBlacklisted(releases, s.loadAlbumBlacklist(albumID))
	for i := range releases {
		indexer.ScoreMusicRelease(&releases[i], profile, identity)
		if releases[i].Acceptable && releases[i].DownloadType != "torrent" && s.grabberSvc == nil {
			releases[i].Acceptable = false
			releases[i].RejectReason = "Usenet download client is not configured"
			failures++
		}
	}
	best := indexer.BestRelease(releases)
	if best == nil {
		if failures > 0 {
			music.Finish(s.db, requestID, "blocked", "No acceptable release found, and one or more music sources could not be searched. Check source connections in Settings, then retry.", 0)
		} else {
			music.Finish(s.db, requestID, "no_results", "No release matched this artist, album, edition and quality profile. You can review releases or retry later.", 0)
		}
		return
	}
	// A stopped search or deleted album must not enqueue a late network response.
	var stillActive int
	if s.db.QueryRow(`SELECT id FROM music_search_requests WHERE id=? AND status='searching'`, requestID).Scan(&stillActive) != nil {
		return
	}
	if err = s.enqueueReleaseForRequest(requestID, 0, 0, albumID, *best); err != nil {
		music.Finish(s.db, requestID, "blocked", "A matching release was found but the download client could not accept it. Check its connection in Settings, then retry.", 0)
		return
	}
	var downloadID int
	s.db.QueryRow(`SELECT id FROM downloads WHERE album_id=? ORDER BY id DESC LIMIT 1`, albumID).Scan(&downloadID)
	music.Finish(s.db, requestID, "download_queued", "Matching release sent to the download client.", downloadID)
}

// Kept as an entry point for existing callers; the request is durable before work.
func (s *Scheduler) searchAndGrabAlbum(albumID int, artist, title string, year, profileID int) {
	request, err := music.Request(s.db, albumID, false)
	if err == nil && request.RequestID > 0 {
		s.runMusicRequest(request.RequestID, albumID)
	}
}
