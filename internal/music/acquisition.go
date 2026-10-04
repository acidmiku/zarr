// Package music keeps saved intent, search attempts and actual files separate.
package music

import (
	"database/sql"
	"errors"
	"mediaforge/internal/database"
	"os"
)

type Acquisition struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	RequestID  int    `json:"request_id,omitempty"`
	DownloadID int    `json:"download_id,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// Request atomically deduplicates requests and transfers. It does not implicitly
// enable monitoring: a one-off download and ongoing monitoring are distinct.
func Request(db *database.DB, albumID int, automatic bool) (Acquisition, error) {
	tx, err := db.Begin()
	if err != nil {
		return Acquisition{}, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow(`SELECT status FROM albums WHERE id=?`, albumID).Scan(&status); err != nil {
		return Acquisition{}, err
	}
	if status != "available" {
		var active int
		err = tx.QueryRow(`SELECT id FROM downloads WHERE album_id=? AND status NOT IN ('imported','failed','cancelled','seeding') LIMIT 1`, albumID).Scan(&active)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Acquisition{}, err
		}
		// Never re-grab when there is a failed import with a source still on disk.
		// Return that tracked attempt so the UI retries import instead.
		if active == 0 {
			retainedID, _, _, lookupErr := retainedSource(tx, albumID)
			if lookupErr != nil {
				return Acquisition{}, lookupErr
			}
			if retainedID > 0 {
				tx.Rollback()
				return Snapshot(db, albumID), nil
			}
		}
		if active == 0 {
			_, err = tx.Exec(`INSERT INTO music_search_requests(album_id,automatic,after_download_id,message)
    SELECT ?,?,COALESCE((SELECT MAX(id) FROM downloads WHERE album_id=?),0),'Request saved. Waiting to search music sources.'
    WHERE NOT EXISTS(SELECT 1 FROM music_search_requests WHERE album_id=? AND status IN ('queued','searching'))`, albumID, automatic, albumID, albumID)
			if err != nil {
				return Acquisition{}, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return Acquisition{}, err
	}
	return Snapshot(db, albumID), nil
}

func Finish(db *database.DB, requestID int, status, message string, downloadID int) {
	var id any
	if downloadID > 0 {
		id = downloadID
	}
	db.Exec(`UPDATE music_search_requests SET status=?,message=?,download_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('queued','searching')`, status, message, id, requestID)
}

// Snapshot uses tracked download IDs, never a title guess. Completed transfers
// aren't available until the importer has verified every expected track.
func Snapshot(db *database.DB, albumID int) Acquisition {
	a := Acquisition{Status: "saved", Message: "Saved to your library. Choose Download now when you want the files.", Retryable: true}
	var albumStatus string
	if err := db.QueryRow(`SELECT status FROM albums WHERE id=?`, albumID).Scan(&albumStatus); err != nil {
		return a
	}
	var requestStatus, requestMessage string
	var afterID int
	db.QueryRow(`SELECT id,status,message,after_download_id,CAST(updated_at AS TEXT) FROM music_search_requests WHERE album_id=? ORDER BY id DESC LIMIT 1`, albumID).
		Scan(&a.RequestID, &requestStatus, &requestMessage, &afterID, &a.UpdatedAt)
	if a.RequestID > 0 {
		a.Status, a.Message = requestStatus, requestMessage
		a.Retryable = requestStatus != "queued" && requestStatus != "searching"
	}
	if albumStatus == "available" {
		a.Status = "available"
		a.Message = "Album imported and available in your library."
		a.Retryable = false
		return a
	}
	var downloadStatus, path, failure string
	// An active transfer takes precedence over an old failure. For terminal rows,
	// the request boundary prevents a previous attempt masquerading as the new one.
	err := db.QueryRow(`SELECT id,status,COALESCE(download_path,''),error_message FROM downloads WHERE album_id=?
  AND (id>? OR status NOT IN ('imported','failed','cancelled','seeding'))
  ORDER BY CASE WHEN status NOT IN ('imported','failed','cancelled','seeding') THEN 0 ELSE 1 END,id DESC LIMIT 1`, albumID, afterID).
		Scan(&a.DownloadID, &downloadStatus, &path, &failure)
	if err != nil || downloadStatus == "failed" || downloadStatus == "cancelled" || downloadStatus == "imported" || downloadStatus == "seeding" {
		if id, retainedPath, message, lookupErr := retainedSource(db, albumID); lookupErr == nil && id > 0 {
			a.DownloadID, downloadStatus, path, failure, err = id, "failed", retainedPath, message, nil
		}
	}
	if err == nil {
		a.Retryable = false
		switch downloadStatus {
		case "failed":
			a.Status = "failed"
			a.Retryable = true
			a.Message = "Download failed. Retry the search or choose another release."
			if path != "" {
				a.Message = "Import failed. The downloaded files were retained; check the activity details before retrying."
			}
			if failure != "" {
				a.Message = failure
			}
		case "cancelled":
			a.Status = "cancelled"
			a.Message = "Download cancelled."
			a.Retryable = true
		case "completed", "extracting", "importing", "seeding", "imported":
			a.Status = "importing"
			a.Message = "Transfer finished. Waiting for all album tracks to be verified and imported."
		case "queued":
			a.Status = "queued"
			a.Message = "Release queued in the download client."
		default:
			a.Status = "downloading"
			a.Message = "Downloading album."
		}
	}
	if a.Status == "download_queued" && a.DownloadID == 0 {
		a.Status = "failed"
		a.Message = "The previous download record is no longer available. You can search again."
		a.Retryable = true
	}
	return a
}

// Pause cancels pending searches, leaving actual transfers under their explicit
// download controls. Scheduler rechecks status before submitting any result.
func Pause(db *database.DB, albumID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE albums SET monitored=0,updated_at=CURRENT_TIMESTAMP WHERE id=?`, albumID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE music_search_requests SET status='cancelled',message='Search cancelled. Album monitoring is off.',updated_at=CURRENT_TIMESTAMP WHERE album_id=? AND status IN ('queued','searching') AND NOT EXISTS(SELECT 1 FROM downloads d WHERE d.album_id=music_search_requests.album_id AND d.status NOT IN ('imported','failed','cancelled','seeding'))`, albumID); err != nil {
		return err
	}
	return tx.Commit()
}

// An older failed import can still have its payload after a later failed attempt
// has been removed. Inspect all retained paths, rather than only the latest row.
func retainedSource(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, albumID int) (int, string, string, error) {
	rows, err := q.Query(`SELECT id,download_path,error_message FROM downloads WHERE album_id=? AND status='failed' AND COALESCE(download_path,'')!='' ORDER BY id DESC`, albumID)
	if err != nil {
		return 0, "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var path, message string
		if err = rows.Scan(&id, &path, &message); err != nil {
			return 0, "", "", err
		}
		if _, err = os.Stat(path); err == nil {
			return id, path, message, nil
		}
	}
	return 0, "", "", rows.Err()
}
