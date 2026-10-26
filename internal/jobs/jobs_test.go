package jobs

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/storage"
)

func setupTestJobs(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	js := &Service{}
	js.Open(db)
	return js, db
}

// addMedia inserts the record a job hangs off, which the foreign key needs.
func addMedia(t *testing.T, db *sql.DB, transcriptStatus string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio, transcript_status)
		VALUES ('demo', 'import', '/tmp/demo.mp4', ?, 1, ?)`, time.Now().UTC().Format(time.RFC3339), transcriptStatus)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// waitEnd waits for the newest job of a media file to stop running.
func waitEnd(t *testing.T, js *Service, mediaID int64) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := js.Latest(mediaID); ok && !j.Running() {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the job did not end")
	return Job{}
}

func TestRunRecordsStateAndProgress(t *testing.T) {
	js, db := setupTestJobs(t)
	mediaID := addMedia(t, db, "pending")

	release := make(chan struct{})
	jobID, err := js.Run(mediaID, KindTranscribe, func(ctx context.Context, progress func(float64)) error {
		progress(0.5)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// While it runs, the progress it reported is readable.
	deadline := time.Now().Add(2 * time.Second)
	for {
		j, ok := js.Latest(mediaID)
		if ok && j.Progress == 0.5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("progress never reached the job: %+v", j)
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)

	job := waitEnd(t, js, mediaID)
	if job.ID != jobID || job.Kind != KindTranscribe || job.MediaID != mediaID {
		t.Errorf("job: %+v", job)
	}
	if job.State != StateDone || job.Progress != 1 || job.Error != "" {
		t.Errorf("finished job: %+v", job)
	}
}

func TestRunRecordsFailure(t *testing.T) {
	js, db := setupTestJobs(t)
	mediaID := addMedia(t, db, "pending")

	if _, err := js.Run(mediaID, KindSummarize, func(context.Context, func(float64)) error {
		return context.DeadlineExceeded
	}); err != nil {
		t.Fatal(err)
	}

	job := waitEnd(t, js, mediaID)
	if job.State != StateFailed || job.Error == "" {
		t.Errorf("failed job: %+v", job)
	}
	if job.Progress >= 1 {
		t.Errorf("a failed job looks finished: %+v", job)
	}
}

func TestCancelStopsWork(t *testing.T) {
	js, db := setupTestJobs(t)
	mediaID := addMedia(t, db, "pending")

	started := make(chan struct{})
	jobID, err := js.Run(mediaID, KindSummarize, func(ctx context.Context, _ func(float64)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the work never started")
	}
	if err := js.Cancel(jobID); err != nil {
		t.Fatal(err)
	}

	job := waitEnd(t, js, mediaID)
	if job.State != StateCancelled {
		t.Errorf("cancelled job: %+v", job)
	}
	if err := js.Cancel(jobID); err == nil {
		t.Error("cancelling a job that already ended reported no error")
	}
}

// Work does not outlive the process that started it: what a crash left
// running is failed, and the media it held pending can be tried again.
func TestOpenRepairsInterruptedWork(t *testing.T) {
	js, db := setupTestJobs(t)
	mediaID := addMedia(t, db, "pending")
	at := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO job (media_id, kind, state, progress, created_at, updated_at)
		VALUES (?, ?, 'running', 0.4, ?, ?)`, mediaID, KindTranscribe, at, at); err != nil {
		t.Fatal(err)
	}
	js.Open(db)

	job, ok := js.Latest(mediaID)
	if !ok || job.State != StateFailed || job.Error != "interrupted" {
		t.Errorf("interrupted job: %+v (found %v)", job, ok)
	}
	var status string
	if err := db.QueryRow(`SELECT transcript_status FROM media WHERE id = ?`, mediaID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "error" {
		t.Errorf("transcript_status is %q, want the pending record marked as an error", status)
	}
}
