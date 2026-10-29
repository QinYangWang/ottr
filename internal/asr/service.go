// Package asr provides automated speech recognition, transcript persistence and export.
package asr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
)

// TranscriptReady is broadcast when a media file finishes transcribing.
var TranscriptReady = mygo.NewEvent[int64]("transcript:ready")

// LiveEvent is broadcast during live recording transcription.
var LiveEvent = mygo.NewEvent[LiveSubtitle]("transcript:live")

// Service manages speech recognition and transcripts.
type Service struct {
	mu     sync.Mutex
	db     *sql.DB
	lib    *library.Service
	models *models.Manager
	jobs   *jobs.Service
}

// NewService creates a transcription service.
func NewService(lib *library.Service, mm *models.Manager, js *jobs.Service) *Service {
	return &Service{lib: lib, models: mm, jobs: js}
}

// Open connects the service to the database.
func (s *Service) Open(db *sql.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}

func (s *Service) store() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, errors.New("database not ready")
	}
	return s.db, nil
}

// Get returns the transcript and segments for a media record.
func (s *Service) Get(mediaID int64) (Transcript, error) {
	db, err := s.store()
	if err != nil {
		return Transcript{}, err
	}
	var tr Transcript
	err = db.QueryRow(`SELECT id, media_id, language, created_at FROM transcript WHERE media_id = ?`, mediaID).
		Scan(&tr.ID, &tr.MediaID, &tr.Language, &tr.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Transcript{MediaID: mediaID}, nil
	}
	if err != nil {
		return Transcript{}, err
	}

	rows, err := db.Query(`SELECT id, idx, start_ms, end_ms, text FROM segment WHERE transcript_id = ? ORDER BY idx ASC`, tr.ID)
	if err != nil {
		return Transcript{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var seg Segment
		if err := rows.Scan(&seg.ID, &seg.Idx, &seg.StartMs, &seg.EndMs, &seg.Text); err != nil {
			return Transcript{}, err
		}
		tr.Segments = append(tr.Segments, seg)
	}
	return tr, rows.Err()
}

// Search searches for segments containing query text.
func (s *Service) Search(mediaID int64, query string) ([]Segment, error) {
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		tr, err := s.Get(mediaID)
		return tr.Segments, err
	}
	rows, err := db.Query(`SELECT s.id, s.idx, s.start_ms, s.end_ms, s.text
		FROM segment s
		JOIN transcript t ON s.transcript_id = t.id
		WHERE t.media_id = ? AND s.text LIKE ?
		ORDER BY s.idx ASC`, mediaID, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Segment
	for rows.Next() {
		var seg Segment
		if err := rows.Scan(&seg.ID, &seg.Idx, &seg.StartMs, &seg.EndMs, &seg.Text); err != nil {
			return nil, err
		}
		list = append(list, seg)
	}
	return list, rows.Err()
}

// Transcribe starts speech recognition for the given media in the background
// and returns the job that runs it. The transcript and its segments replace
// any earlier ones when it ends; TranscriptReady says so.
func (s *Service) Transcribe(mediaID int64) (int64, error) {
	job, err := s.prepare(mediaID)
	if err != nil {
		return 0, err
	}
	jobID, err := s.jobs.Run(mediaID, jobs.KindTranscribe, func(ctx context.Context, progress func(float64)) error {
		return s.recognize(ctx, job, progress)
	})
	if err != nil {
		s.setStatus(mediaID, "error")
		return 0, err
	}
	return jobID, nil
}

// TranscribeNow does the same work in the caller's goroutine and context, for
// work that is already a job of its own — importing a file, which goes on to
// translate and summarize it.
func (s *Service) TranscribeNow(ctx context.Context, mediaID int64, progress func(float64)) error {
	job, err := s.prepare(mediaID)
	if err != nil {
		return err
	}
	return s.recognize(ctx, job, progress)
}

// recognition is what one transcription needs: where the media is, and which
// model reads it.
type recognition struct {
	mediaID int64
	path    string
	model   models.ASRModel
}

// prepare checks that the media can be transcribed and marks it pending, so
// that the window shows the work before it starts.
func (s *Service) prepare(mediaID int64) (recognition, error) {
	m, err := s.lib.Get(mediaID)
	if err != nil {
		return recognition{}, err
	}
	if !m.HasAudio {
		return recognition{}, errors.New("this media file has no audio stream to transcribe")
	}
	if err := s.ready(); err != nil {
		return recognition{}, err
	}
	mdl, err := s.models.FindASRModel()
	if err != nil {
		return recognition{}, err
	}
	db, err := s.store()
	if err != nil {
		return recognition{}, err
	}
	if _, err := db.Exec(`UPDATE media SET transcript_status = 'pending' WHERE id = ?`, mediaID); err != nil {
		return recognition{}, err
	}
	library.Changed.Broadcast(struct{}{})
	return recognition{mediaID: mediaID, path: m.Path, model: *mdl}, nil
}

// recognize turns the audio into segments and stores them.
func (s *Service) recognize(ctx context.Context, r recognition, progress func(float64)) error {
	tmpWav := filepath.Join(os.TempDir(), fmt.Sprintf("ottr-asr-%d-%d.wav", r.mediaID, time.Now().UnixNano()))
	defer os.Remove(tmpWav)

	progress(0.05)
	if err := ExtractWav(ctx, r.path, tmpWav); err != nil {
		return s.endFailed(r.mediaID, ctx, err)
	}
	progress(0.15) // extraction done; sherpa-onnx then runs the file in one go
	segments, err := recognizeWav(ctx, r.model, tmpWav)
	if err != nil {
		return s.endFailed(r.mediaID, ctx, err)
	}
	progress(0.9)
	if err := s.saveTranscript(r.mediaID, segments); err != nil {
		return s.endFailed(r.mediaID, ctx, err)
	}
	TranscriptReady.Broadcast(r.mediaID)
	library.Changed.Broadcast(struct{}{})
	return nil
}

// endFailed records why recognition stopped, and leaves the record as it was
// when the user cancelled rather than failed.
func (s *Service) endFailed(mediaID int64, ctx context.Context, err error) error {
	status := "error"
	if ctx.Err() != nil {
		status = "none"
	}
	s.setStatus(mediaID, status)
	return err
}

func (s *Service) setStatus(mediaID int64, status string) {
	db, err := s.store()
	if err != nil {
		return
	}
	if _, err := db.Exec(`UPDATE media SET transcript_status = ? WHERE id = ?`, status, mediaID); err != nil {
		return
	}
	library.Changed.Broadcast(struct{}{})
}

func (s *Service) saveTranscript(mediaID int64, segments []Segment) error {
	db, err := s.store()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear old transcript if present
	if _, err := tx.Exec(`DELETE FROM transcript WHERE media_id = ?`, mediaID); err != nil {
		return err
	}

	res, err := tx.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, ?, ?)`,
		mediaID, DetectLanguage(segments), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	trID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, seg := range segments {
		if _, err := stmt.Exec(trID, i+1, seg.StartMs, seg.EndMs, seg.Text); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`UPDATE media SET transcript_status = 'done' WHERE id = ?`, mediaID); err != nil {
		return err
	}
	return tx.Commit()
}

// Export prompts user to save the transcript as SRT, VTT or TXT.
func (s *Service) Export(ctx context.Context, mediaID int64, format string) (string, error) {
	tr, err := s.Get(mediaID)
	if err != nil {
		return "", err
	}
	if len(tr.Segments) == 0 {
		return "", errors.New("no transcript available to export")
	}

	m, err := s.lib.Get(mediaID)
	if err != nil {
		return "", err
	}

	format = strings.ToLower(strings.TrimPrefix(format, "."))
	var content, ext string
	switch format {
	case "srt":
		content = FormatSRT(tr.Segments)
		ext = "srt"
	case "vtt":
		content = FormatVTT(tr.Segments)
		ext = "vtt"
	case "txt":
		content = FormatTXT(tr.Segments)
		ext = "txt"
	default:
		return "", fmt.Errorf("unsupported export format: %s", format)
	}

	defaultName := m.Name + "." + ext
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       "Export Subtitles",
		DefaultPath: defaultName,
		Filters: []mygo.FileFilter{
			{Name: strings.ToUpper(ext) + " Files", Extensions: []string{ext}},
		},
	})
	if err != nil || path == "" {
		return "", err
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write subtitle file: %w", err)
	}
	return path, nil
}
