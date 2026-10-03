package postprocess

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"mediaforge/internal/database"
	"mediaforge/internal/indexer"
)

type Processor struct {
	db            *database.DB
	mediaRoot     string
	coverCacheDir string
	mu            sync.Mutex
}

func New(db *database.DB, mediaRoot, coverCacheDir string) *Processor {
	return &Processor{db: db, mediaRoot: mediaRoot, coverCacheDir: coverCacheDir}
}
func (p *Processor) UpdateMediaRoot(root string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mediaRoot = root
}
func (p *Processor) Process(downloadID int, downloadPath string) error {
	return p.process(downloadID, downloadPath, false)
}
func (p *Processor) ProcessTorrent(downloadID int, contentPath string) error {
	return p.process(downloadID, contentPath, true)
}

type importFile struct {
	src, dst           string
	episodeID, trackID int
	replace            bool
}

// Build and validate the entire import before touching files. Source files survive
// failed imports, and torrent files always remain available for seeding.
func (p *Processor) process(downloadID int, downloadPath string, torrent bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(p.mediaRoot) == "" {
		return fmt.Errorf("media root is not configured")
	}
	if strings.TrimSpace(downloadPath) == "" {
		return fmt.Errorf("download path is empty")
	}
	var mediaID, episodeID, albumID sql.NullInt64
	var status string
	if err := p.db.QueryRow(`SELECT media_item_id, episode_id, album_id, status FROM downloads WHERE id = ?`, downloadID).Scan(&mediaID, &episodeID, &albumID, &status); err != nil {
		return fmt.Errorf("lookup download: %w", err)
	}
	if status == "cancelled" {
		return fmt.Errorf("download was cancelled")
	}
	if status == "imported" || status == "seeding" {
		return nil
	}
	var plan []importFile
	var mediaType, title string
	var err error
	if albumID.Valid {
		plan, err = p.albumPlan(int(albumID.Int64), downloadPath)
	} else {
		var year sql.NullInt64
		var libraryRoot sql.NullString
		var anime bool
		err = p.db.QueryRow(`SELECT type, title, year, anime, root_path FROM media_items WHERE id = ?`, mediaID.Int64).Scan(&mediaType, &title, &year, &anime, &libraryRoot)
		if err != nil {
			return fmt.Errorf("lookup media: %w", err)
		}
		var files []string
		files, err = findVideoFiles(downloadPath)
		if err == nil && len(files) == 0 {
			err = fmt.Errorf("no video files found")
		}
		if err == nil {
			if mediaType == "movie" {
				src := LargestFile(files)
				if indexer.IsEpisodicRelease(filepath.Base(src)) {
					return fmt.Errorf("episode file cannot be imported as a movie")
				}
				var importedBefore int
				if err := p.db.QueryRow(`SELECT COUNT(*) FROM downloads WHERE media_item_id=? AND id!=? AND status IN ('imported','seeding')`, mediaID.Int64, downloadID).Scan(&importedBefore); err != nil {
					return err
				}
				dst := MoviePath(p.mediaRoot, title, int(year.Int64), filepath.Ext(src))
				plan = []importFile{{src: src, dst: dst, replace: importedBefore > 0 && libraryRoot.Valid && filepath.Clean(libraryRoot.String) == filepath.Dir(dst)}}
			} else {
				plan, err = p.episodePlan(int(mediaID.Int64), episodeID, title, int(year.Int64), anime, files)
			}
		}
	}
	if err != nil {
		return err
	}
	if len(plan) == 0 {
		return fmt.Errorf("no files matched this download")
	}
	seen := map[string]bool{}
	for _, f := range plan {
		if seen[f.dst] {
			return fmt.Errorf("multiple files target the same library path: %s", f.dst)
		}
		seen[f.dst] = true
		rel, err := filepath.Rel(p.mediaRoot, f.dst)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("import path escapes media root")
		}
		if err := validateDestination(p.mediaRoot, f.dst); err != nil {
			return err
		}
	}
	var installed []stagedFile
	committed := false
	defer func() {
		if !committed {
			for i := len(installed) - 1; i >= 0; i-- {
				installed[i].rollback()
			}
		}
	}()
	for _, f := range plan {
		staged, err := stageFile(f.src, f.dst, f.replace)
		if err != nil {
			return fmt.Errorf("import %s: %w", filepath.Base(f.src), err)
		}
		installed = append(installed, staged)
	}

	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, f := range plan {
		if f.episodeID > 0 {
			_, err = tx.Exec(`UPDATE episodes SET file_path = ?, status = 'available' WHERE id = ?`, f.dst, f.episodeID)
		}
		if f.trackID > 0 {
			_, err = tx.Exec(`UPDATE tracks SET file_path = ?, status = 'available' WHERE id = ?`, f.dst, f.trackID)
		}
		if err != nil {
			return err
		}
	}
	if albumID.Valid {
		_, err = tx.Exec(`UPDATE albums SET status = CASE WHEN (SELECT COUNT(*) FROM tracks WHERE album_id = ?) > 0 AND NOT EXISTS (SELECT 1 FROM tracks WHERE album_id = ? AND status != 'available') THEN 'available' ELSE 'wanted' END, root_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, albumID.Int64, albumID.Int64, filepath.Dir(plan[0].dst), albumID.Int64)
	} else if mediaType == "movie" {
		_, err = tx.Exec(`UPDATE media_items SET root_path = ?, status = 'available', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, filepath.Dir(plan[0].dst), mediaID.Int64)
	} else {
		_, err = tx.Exec(`UPDATE media_items SET status = CASE WHEN EXISTS (SELECT 1 FROM episodes WHERE media_item_id = ? AND episode_type = 'standard') AND NOT EXISTS (SELECT 1 FROM episodes WHERE media_item_id = ? AND episode_type = 'standard' AND status != 'available') THEN 'available' ELSE 'wanted' END, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, mediaID.Int64, mediaID.Int64, mediaID.Int64)
	}
	if err != nil {
		return err
	}
	nextStatus := "imported"
	if torrent {
		nextStatus = "seeding"
	}
	res, err := tx.Exec(`UPDATE downloads SET status = ?, download_path = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ? AND status != 'cancelled'`, nextStatus, downloadPath, downloadID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("download was cancelled during import")
	}
	_, err = tx.Exec(`INSERT INTO activity_log (media_item_id, episode_id, album_id, action, details) VALUES (?, ?, ?, 'imported', ?)`, mediaID, episodeID, albumID, fmt.Sprintf("Imported %d media files", len(plan)))
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	for _, f := range installed {
		f.commit()
	}
	if albumID.Valid {
		p.SaveCoverToAlbumDir(int(albumID.Int64), filepath.Dir(plan[0].dst), downloadPath)
	}
	if !torrent {
		for _, f := range plan {
			if filepath.Clean(f.src) != filepath.Clean(f.dst) {
				if err := os.Remove(f.src); err != nil {
					slog.Warn("could not remove imported source", "path", f.src, "error", err)
				}
			}
		}
	}
	return nil
}

var episodeNumber = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,3})e(\d{1,4})(?:[^0-9]|$)`)
var alternateEpisodeNumber = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,3})x(\d{1,4})(?:[^0-9]|$)`)

func (p *Processor) episodePlan(mediaID int, requested sql.NullInt64, title string, year int, anime bool, files []string) ([]importFile, error) {
	type episode struct {
		id, season, number int
		absolute           sql.NullInt64
		title              sql.NullString
		path               sql.NullString
		kind               string
	}
	rows, err := p.db.Query(`SELECT e.id, s.number, e.number, e.absolute_number, e.title, e.episode_type, e.file_path FROM episodes e JOIN seasons s ON s.id = e.season_id WHERE e.media_item_id = ?`, mediaID)
	if err != nil {
		return nil, err
	}
	var episodes []episode
	for rows.Next() {
		var e episode
		if err := rows.Scan(&e.id, &e.season, &e.number, &e.absolute, &e.title, &e.kind, &e.path); err != nil {
			rows.Close()
			return nil, err
		}
		episodes = append(episodes, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var plan []importFile
	for _, src := range files {
		name := filepath.Base(src)
		if indexer.IsMultiEpisodeRelease(name) || indexer.HasAbsoluteEpisodeRange(name, title) {
			return nil, fmt.Errorf("combined episode file requires manual import: %s", name)
		}
		match := episodeNumber.FindStringSubmatch(name)
		if match == nil {
			match = alternateEpisodeNumber.FindStringSubmatch(name)
		}
		var selected *episode
		if match != nil {
			season, _ := strconv.Atoi(match[1])
			number, _ := strconv.Atoi(match[2])
			for i := range episodes {
				if episodes[i].season == season && episodes[i].number == number {
					selected = &episodes[i]
					break
				}
			}
		} else if anime {
			n, ok := indexer.AbsoluteEpisodeNumber(strings.TrimSuffix(name, filepath.Ext(name)), title)
			// Numeric-only files are common inside correctly labelled season packs.
			if !ok && regexp.MustCompile(`^\d{1,4}(?:v\d+)?(?:[ ._-]|$)`).MatchString(strings.TrimSuffix(name, filepath.Ext(name))) {
				n, ok = indexer.AbsoluteEpisodeNumber(title+" "+strings.TrimSuffix(name, filepath.Ext(name)), title)
			}
			if ok {
				for i := range episodes {
					if episodes[i].absolute.Valid && int(episodes[i].absolute.Int64) == n {
						if selected != nil && selected.id != episodes[i].id {
							return nil, fmt.Errorf("ambiguous anime episode: %s", name)
						}
						selected = &episodes[i]
					}
				}
			}
		}
		if selected == nil && indexer.LooksLikeMovie(name, title) {
			return nil, fmt.Errorf("movie release cannot be imported as an episode: %s", name)
		}

		if selected == nil && match == nil && len(files) == 1 && requested.Valid {
			for i := range episodes {
				if episodes[i].id == int(requested.Int64) {
					selected = &episodes[i]
					break
				}
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("cannot match episode file: %s", name)
		}
		if requested.Valid && selected.id != int(requested.Int64) {
			continue
		}
		e := selected
		ext := filepath.Ext(src)
		var dst string
		if anime && (e.kind == "special" || e.kind == "ova" || e.kind == "ona") {
			dst = AnimeSpecialPath(p.mediaRoot, title, e.number, e.title.String, ext)
		} else if anime {
			absolute := e.number
			if e.absolute.Valid {
				absolute = int(e.absolute.Int64)
			}
			dst = AnimeEpisodePath(p.mediaRoot, title, e.season, e.number, absolute, e.title.String, ext)
		} else {
			dst = SeriesEpisodePath(p.mediaRoot, title, year, e.season, e.number, e.title.String, ext)
		}
		plan = append(plan, importFile{src: src, dst: dst, episodeID: e.id, replace: e.path.Valid && filepath.Clean(e.path.String) == filepath.Clean(dst)})
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("download does not contain the requested episode")
	}
	return plan, nil
}

var discDirectory = regexp.MustCompile(`(?i)^(?:cd|disc|disk)[ _.-]*(\d+)$`)

func (p *Processor) albumPlan(albumID int, path string) ([]importFile, error) {
	var artist, title string
	var year sql.NullInt64
	if err := p.db.QueryRow(`SELECT a.name, al.title, al.year FROM albums al JOIN artists a ON a.id = al.artist_id WHERE al.id = ?`, albumID).Scan(&artist, &title, &year); err != nil {
		return nil, err
	}
	files, err := findAudioFiles(path)
	if err != nil {
		return nil, err
	}
	rows, err := p.db.Query(`SELECT id, number, COALESCE(disc_number,1), title, file_path FROM tracks WHERE album_id = ?`, albumID)
	if err != nil {
		return nil, err
	}
	type track struct {
		id, number, disc int
		title            string
		path             sql.NullString
	}
	var tracks []track
	for rows.Next() {
		var t track
		if err := rows.Scan(&t.id, &t.number, &t.disc, &t.title, &t.path); err != nil {
			rows.Close()
			return nil, err
		}
		tracks = append(tracks, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var plan []importFile
	for _, src := range files {
		number, disc := parseTrackNumber(filepath.Base(src))
		if m := discDirectory.FindStringSubmatch(filepath.Base(filepath.Dir(src))); m != nil {
			disc, _ = strconv.Atoi(m[1])
		}
		found := false
		for _, t := range tracks {
			if t.number == number && t.disc == disc {
				dst := MusicTrackPath(p.mediaRoot, artist, title, int(year.Int64), t.number, t.disc, t.title, filepath.Ext(src))
				plan = append(plan, importFile{src: src, dst: dst, trackID: t.id, replace: t.path.Valid && filepath.Clean(t.path.String) == filepath.Clean(dst)})
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("cannot match audio track: %s", filepath.Base(src))
		}
	}
	covered := map[int]bool{}
	for _, file := range plan {
		covered[file.trackID] = true
	}
	for _, track := range tracks {
		if covered[track.id] {
			continue
		}
		if track.path.Valid {
			if info, err := os.Stat(track.path.String); err == nil && info.Mode().IsRegular() {
				continue
			}
		}
		return nil, fmt.Errorf("album download is incomplete: missing disc %d track %d; source retained", track.disc, track.number)
	}

	return plan, nil
}

func findAudioFiles(path string) ([]string, error) { return findMediaFiles(path, IsAudioFile) }

var sampleName = regexp.MustCompile(`(?i)(?:^|[ ._-])sample(?:[ ._-]|$)`)

func findVideoFiles(path string) ([]string, error) {
	return findMediaFiles(path, func(name string) bool {
		return IsVideoFile(name) && !sampleName.MatchString(name)
	})
}
func findMediaFiles(root string, accept func(string) bool) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in download: %s", path)
		}
		if info.Mode().IsRegular() && accept(info.Name()) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
func parseTrackNumber(filename string) (trackNum, discNum int) {
	discNum = 1
	var d, t int
	if n, _ := fmt.Sscanf(filename, "%d-%02d", &d, &t); n == 2 && d > 0 && d <= 20 && t > 0 {
		return t, d
	}
	if n, _ := fmt.Sscanf(filename, "%d", &t); n == 1 && t > 0 && t <= 999 {
		return t, 1
	}
	return 0, 1
}
func LargestFile(files []string) string {
	var largest string
	var size int64 = -1
	for _, f := range files {
		info, err := os.Stat(f)
		if err == nil && info.Mode().IsRegular() && info.Size() > size {
			size = info.Size()
			largest = f
		}
	}
	return largest
}

// Reject symlink components below the configured root, including pre-existing
// library folders, so a file import cannot be redirected outside the library.
func validateDestination(root, dst string) error {
	rel, err := filepath.Rel(root, dst)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("destination escapes media root")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination contains a symlink: %s", current)
		}
	}
	return nil
}

// Upgrades replace a directory entry, never the old inode. Keep the old
// library file until the database commits, so failed imports can be rolled back.
type stagedFile struct {
	destination, backup string
	created             bool
}

func (f stagedFile) rollback() {
	if f.created {
		if err := os.Remove(f.destination); err != nil && !os.IsNotExist(err) {
			slog.Error("remove failed import", "path", f.destination, "error", err)
			return
		}
	}
	if f.backup != "" {
		if err := os.Rename(f.backup, f.destination); err != nil {
			slog.Error("restore import backup", "path", f.backup, "error", err)
		}
	}
}
func (f stagedFile) commit() {
	if f.backup != "" {
		if err := os.Remove(f.backup); err != nil {
			slog.Warn("remove import backup", "path", f.backup, "error", err)
		}
	}
}
func stageFile(src, dst string, replace bool) (stagedFile, error) {
	f := stagedFile{destination: dst}
	source, err := os.Stat(src)
	if err != nil {
		return f, err
	}
	if target, err := os.Lstat(dst); err == nil {
		if target.Mode()&os.ModeSymlink != 0 || !target.Mode().IsRegular() {
			return f, fmt.Errorf("destination is not a regular file")
		}
		if os.SameFile(source, target) {
			return f, nil
		}
		if !replace {
			return f, fmt.Errorf("destination already exists and is not tracked for this item")
		}
		tmp, err := os.CreateTemp(filepath.Dir(dst), ".zarr-backup-*")
		if err != nil {
			return f, err
		}
		backup := tmp.Name()
		tmp.Close()
		os.Remove(backup)
		if err := os.Rename(dst, backup); err != nil {
			return f, err
		}
		f.backup = backup
	} else if !os.IsNotExist(err) {
		return f, err
	}
	if err := hardlinkFile(src, dst); err != nil {
		f.rollback()
		return stagedFile{}, err
	}
	f.created = true
	return f, nil
}

// Never open an existing library inode for writing: it may be hardlinked to a
// torrent still seeding. Exclusive creation makes retries and conflicts safe.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
func moveFile(src, dst string) error {
	if err := hardlinkFile(src, dst); err != nil {
		return err
	}
	if filepath.Clean(src) == filepath.Clean(dst) {
		return nil
	}
	return os.Remove(src)
}
func hardlinkFile(src, dst string) error {
	source, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !source.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if target, err := os.Lstat(dst); err == nil {
		if target.Mode()&os.ModeSymlink == 0 && os.SameFile(source, target) {
			return nil
		}
		return fmt.Errorf("destination already exists: %s", dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

// Cleanup only prunes empty directories. Unmatched files, extras, and other
// jobs in a shared client directory must never be recursively deleted.
func CleanDownloadDir(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if err := CleanDownloadDir(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	entries, err = os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(root)
	}
	return nil
}

// SaveCoverToAlbumDir saves cover art as cover.jpg in the album directory.
// It first checks the download directory for existing cover images, then
// falls back to the cached cover art from CoverArtArchive.
func (p *Processor) SaveCoverToAlbumDir(albumID int, albumDir, downloadDir string) {
	coverDest := filepath.Join(albumDir, "cover.jpg")

	// Already exists
	if _, err := os.Stat(coverDest); err == nil {
		return
	}

	os.MkdirAll(albumDir, 0755)

	// 1. Look for cover images in the download directory
	if src := findCoverInDir(downloadDir); src != "" {
		if err := copyFile(src, coverDest); err == nil {
			slog.Info("saved album cover from download", "dest", coverDest)
			return
		}
	}

	// 2. Fall back to cached cover art
	if p.coverCacheDir != "" {
		var rgid string
		p.db.QueryRow(`SELECT release_group_id FROM albums WHERE id = ?`, albumID).Scan(&rgid)
		if rgid != "" {
			cached := filepath.Join(p.coverCacheDir, "mb_"+rgid+".jpg")
			if _, err := os.Stat(cached); err == nil {
				if err := copyFile(cached, coverDest); err == nil {
					slog.Info("saved album cover from cache", "dest", coverDest)
					return
				}
			}
		}
	}

	slog.Debug("no cover art found for album", "albumID", albumID)
}

// findCoverInDir looks for common cover art filenames in a directory.
func findCoverInDir(dir string) string {
	coverNames := []string{
		"cover.jpg", "cover.jpeg", "cover.png",
		"folder.jpg", "folder.jpeg", "folder.png",
		"front.jpg", "front.jpeg", "front.png",
		"album.jpg", "album.jpeg", "album.png",
		"Cover.jpg", "Folder.jpg", "Front.jpg",
	}

	for _, name := range coverNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Also check subdirectories one level deep
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range coverNames[:6] { // just lowercase variants
			path := filepath.Join(dir, entry.Name(), name)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}
