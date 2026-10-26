// Package jobs persists background work: a long task gets a status, a
// progress, an error and a way to be cancelled, and a window that missed its
// events can still read what happened.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// The kinds of work, which also say what a job leaves in the library.
const (
	KindTranscribe = "transcribe"
	KindTranslate  = "translate"
	KindSummarize  = "summarize"
	// KindImport is transcribing an imported file, then translating and
	// summarizing it when the import asked for that.
	KindImport = "import"
)

// The states of a job.
const (
	StateRunning   = "running"
	StateDone      = "done"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
)

// Changed is broadcast whenever a job starts, advances or ends.
var Changed = mygo.NewEvent[Job]("job:progress")

// Job is one background task over one media file.
type Job struct {
	ID        int64   `json:"id"`
	MediaID   int64   `json:"mediaId"`
	Kind      string  `json:"kind"`
	State     string  `json:"state"`
	Progress  float64 `json:"progress"` // 0..1
	Error     string  `json:"error"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// Running reports whether the job still has work to do.
func (j Job) Running() bool { return j.State == StateRunning }

// timeLimit bounds any one job: work that outlives it is cancelled.
const timeLimit = 30 * time.Minute

// Service stores jobs and keeps the way to cancel the ones running.
type Service struct {
	mu     sync.Mutex
	db     *sql.DB
	cancel map[int64]context.CancelFunc
}

// Open connects the service to the database. Work does not outlive the
// process that started it, so a job still marked running was interrupted:
// it and the media it was working on are left in a state the window can act
// on rather than pending forever.
func (s *Service) Open(db *sql.DB) {
	s.mu.Lock()
	s.db = db
	s.cancel = map[int64]context.CancelFunc{}
	s.mu.Unlock()

	if _, err := db.Exec(`UPDATE job SET state = ?, error = 'interrupted', updated_at = ?
		WHERE state = ?`, StateFailed, now(), StateRunning); err != nil {
		return
	}
	db.Exec(`UPDATE media SET transcript_status = 'error' WHERE transcript_status = 'pending'
		AND id IN (SELECT media_id FROM job WHERE error = 'interrupted' AND kind IN (?, ?))`,
		KindTranscribe, KindImport)
	db.Exec(`UPDATE media SET summary_status = 'error' WHERE summary_status = 'pending'
		AND id IN (SELECT media_id FROM job WHERE error = 'interrupted' AND kind IN (?, ?))`,
		KindSummarize, KindImport)
}

func (s *Service) store() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, errors.New("jobs are not ready")
	}
	return s.db, nil
}

// Run records a job and runs work in the background, reporting its progress
// through progress, which work calls with 0..1. Work stops when its context
// is cancelled, either by Cancel or by the time limit.
func (s *Service) Run(mediaID int64, kind string, work func(ctx context.Context, progress func(float64)) error) (int64, error) {
	id, err := s.start(mediaID, kind)
	if err != nil {
		return 0, err
	}

	ctx, stop := context.WithTimeout(context.Background(), timeLimit)
	s.mu.Lock()
	s.cancel[id] = stop
	s.mu.Unlock()

	go func() {
		defer stop()
		err := work(ctx, func(p float64) { s.setProgress(id, p) })
		// Leave the registry before the row says the job ended, so that a Cancel
		// that sees a finished job always hears that there is nothing to stop.
		s.mu.Lock()
		delete(s.cancel, id)
		s.mu.Unlock()
		s.finish(id, err, ctx.Err() != nil)
	}()
	return id, nil
}

func (s *Service) start(mediaID int64, kind string) (int64, error) {
	db, err := s.store()
	if err != nil {
		return 0, err
	}
	at := now()
	res, err := db.Exec(`INSERT INTO job (media_id, kind, state, progress, error, created_at, updated_at)
		VALUES (?, ?, ?, 0, '', ?, ?)`, mediaID, kind, StateRunning, at, at)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	Changed.Broadcast(Job{ID: id, MediaID: mediaID, Kind: kind, State: StateRunning, CreatedAt: at, UpdatedAt: at})
	return id, nil
}

// setProgress records how far a job has come, in steps big enough to be
// worth a write and a frame.
func (s *Service) setProgress(id int64, p float64) {
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	db, err := s.store()
	if err != nil {
		return
	}
	var cur float64
	if err := db.QueryRow(`SELECT progress FROM job WHERE id = ? AND state = ?`, id, StateRunning).Scan(&cur); err != nil {
		return // the job ended, or its media is gone
	}
	if p < 1 && p-cur < 0.01 {
		return
	}
	if _, err := db.Exec(`UPDATE job SET progress = ?, updated_at = ? WHERE id = ?`, p, now(), id); err != nil {
		return
	}
	Changed.Broadcast(Job{ID: id, State: StateRunning, Progress: p})
}

func (s *Service) finish(id int64, err error, cancelled bool) {
	db, storeErr := s.store()
	if storeErr != nil {
		return
	}
	state, msg := StateDone, ""
	switch {
	case err != nil && cancelled:
		// Work that returned an error after its context ended was stopped, not
		// broken; work that finished anyway is done.
		state, msg = StateCancelled, "cancelled"
	case err != nil:
		state, msg = StateFailed, err.Error()
	}
	var done any // nil keeps the progress the job had reached
	if state == StateDone {
		done = 1.0
	}
	if _, execErr := db.Exec(`UPDATE job SET state = ?, error = ?, progress = COALESCE(?, progress), updated_at = ?
		WHERE id = ?`, state, msg, done, now(), id); execErr != nil {
		return
	}
	if job, ok := s.get(id); ok {
		Changed.Broadcast(job)
	}
}

// Cancel stops a running job. The work stops at its next context check; the
// job is marked cancelled when it does.
func (s *Service) Cancel(id int64) error {
	s.mu.Lock()
	stop, ok := s.cancel[id]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %d is not running", id)
	}
	stop()
	return nil
}

// Latest returns the most recent job of a media file, running or ended, so
// that a window can show what became of the last attempt.
func (s *Service) Latest(mediaID int64) (Job, bool) {
	db, err := s.store()
	if err != nil {
		return Job{}, false
	}
	var j Job
	err = db.QueryRow(`SELECT id, media_id, kind, state, progress, error, created_at, updated_at
		FROM job WHERE media_id = ? ORDER BY id DESC LIMIT 1`, mediaID).
		Scan(&j.ID, &j.MediaID, &j.Kind, &j.State, &j.Progress, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return Job{}, false
	}
	return j, true
}

func (s *Service) get(id int64) (Job, bool) {
	db, err := s.store()
	if err != nil {
		return Job{}, false
	}
	var j Job
	err = db.QueryRow(`SELECT id, media_id, kind, state, progress, error, created_at, updated_at
		FROM job WHERE id = ?`, id).
		Scan(&j.ID, &j.MediaID, &j.Kind, &j.State, &j.Progress, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return Job{}, false
	}
	return j, true
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
