// Package library is the video knowledge base: it stores media records,
// imports files and serves media to the player.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"ottr/internal/media"
)

// Changed is broadcast when the library gains or loses a record.
var Changed = mygo.NewEvent[struct{}]("library:changed")

// Media is one entry of the library.
type Media struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Source           string    `json:"source"` // import | recording
	Path             string    `json:"path"`
	DurationMs       int64     `json:"durationMs"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	Size             int64     `json:"size"`
	HasAudio         bool      `json:"hasAudio"`
	HasVideo         bool      `json:"hasVideo"`
	CreatedAt        time.Time `json:"createdAt"`
	TranscriptStatus string    `json:"transcriptStatus"`
	SummaryStatus    string    `json:"summaryStatus"`
}

// Extensions accepted by Import.
var Extensions = []string{"mp4", "mov", "mkv", "webm", "mp3", "wav", "m4a"}

// Service is bound to the frontend as "Library".
type Service struct {
	mu sync.Mutex
	db *sql.DB
}

// Open connects the service to the database. Call before App.Run returns.
func (s *Service) Open(db *sql.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}

func (s *Service) store() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, errors.New("library is not ready")
	}
	return s.db, nil
}

// List returns media, newest first, filtered by search on the name.
func (s *Service) List(search string) ([]Media, error) {
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	query := `SELECT id, name, source, path, duration_ms, width, height, size,
		has_audio, has_video, created_at, transcript_status, summary_status
		FROM media`
	args := []any{}
	if search = strings.TrimSpace(search); search != "" {
		query += ` WHERE name LIKE ?`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Media{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// Get returns one media record.
func (s *Service) Get(id int64) (Media, error) {
	db, err := s.store()
	if err != nil {
		return Media{}, err
	}
	row := db.QueryRow(`SELECT id, name, source, path, duration_ms, width, height, size,
		has_audio, has_video, created_at, transcript_status, summary_status
		FROM media WHERE id = ?`, id)
	m, err := scanMedia(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, fmt.Errorf("media %d not found", id)
	}
	return m, err
}

// Import asks for media files, probes them and adds them to the library.
// It returns the imported records.
func (s *Service) Import(ctx context.Context) ([]Media, error) {
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:   mygo.CallerWindow(ctx),
		Title:    "Import media",
		Multiple: true,
		Filters: []mygo.FileFilter{
			{Name: "Media", Extensions: Extensions},
		},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	imported := []Media{}
	var errs []string
	for _, path := range paths {
		m, err := s.add(ctx, db, path, "import")
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(path), err))
			continue
		}
		imported = append(imported, m)
	}
	if len(imported) > 0 {
		Changed.Broadcast(struct{}{})
	}
	if len(errs) > 0 {
		return imported, errors.New(strings.Join(errs, "\n"))
	}
	return imported, nil
}

// AddRecording probes a finished recording and adds it to the library.
func (s *Service) AddRecording(path string) (Media, error) {
	db, err := s.store()
	if err != nil {
		return Media{}, err
	}
	m, err := s.add(context.Background(), db, path, "recording")
	if err != nil {
		return Media{}, err
	}
	Changed.Broadcast(struct{}{})
	return m, nil
}

func (s *Service) add(ctx context.Context, db *sql.DB, path, source string) (Media, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	ok := false
	for _, e := range Extensions {
		if e == ext {
			ok = true
			break
		}
	}
	if !ok {
		return Media{}, fmt.Errorf("unsupported type .%s", ext)
	}
	info, err := media.Probe(ctx, path)
	if err != nil {
		return Media{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	res, err := db.Exec(`INSERT INTO media
		(name, source, path, duration_ms, width, height, size, has_audio, has_video, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, source, abs, info.DurationMs, info.Width, info.Height, info.Size,
		info.HasAudio, info.HasVideo, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Media{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Media{}, err
	}
	return s.Get(id)
}

// Remove asks for confirmation and deletes the record. The media file on
// disk is only deleted when the user checks the box.
func (s *Service) Remove(ctx context.Context, id int64) error {
	db, err := s.store()
	if err != nil {
		return err
	}
	m, err := s.Get(id)
	if err != nil {
		return err
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:        mygo.CallerWindow(ctx),
		Type:          mygo.MessageWarning,
		Title:         "Remove from library",
		Message:       fmt.Sprintf("Remove %q from the library?", m.Name),
		Buttons:       []string{"Cancel", "Remove"},
		DefaultButton: 0,
		CancelButton:  0,
		CheckboxLabel: "Also delete the file from disk",
	})
	if err != nil || res.Button != 1 {
		return err
	}
	if _, err := db.Exec(`DELETE FROM media WHERE id = ?`, id); err != nil {
		return err
	}
	if res.CheckboxChecked {
		if err := os.Remove(m.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("record removed, but deleting the file failed: %w", err)
		}
	}
	Changed.Broadcast(struct{}{})
	return nil
}

// serveMedia serves a media file over the "media" scheme, with range
// requests for seeking: media://localhost/<id>
func (s *Service) serveMedia(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/"), 10, 64)
	if err != nil {
		http.Error(w, "bad media id", http.StatusBadRequest)
		return
	}
	m, err := s.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(m.Path)
	if err != nil {
		http.Error(w, "file unavailable: "+err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ct := contentType(m.Path); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(m.Path), st.ModTime(), f)
}

// MediaHandler serves library media over the "media" scheme. It is not a
// method, so mygo.Bind does not try to expose it to the frontend.
func MediaHandler(s *Service) http.HandlerFunc { return s.serveMedia }

func contentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/mp4"
	}
	return ""
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMedia(row scanner) (Media, error) {
	var m Media
	var created string
	err := row.Scan(&m.ID, &m.Name, &m.Source, &m.Path, &m.DurationMs,
		&m.Width, &m.Height, &m.Size, &m.HasAudio, &m.HasVideo, &created,
		&m.TranscriptStatus, &m.SummaryStatus)
	if err != nil {
		return Media{}, err
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return m, nil
}
