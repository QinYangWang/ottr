// Package library is the video knowledge base: it stores media records,
// imports files and serves media to the player.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	Source           string    `json:"source"` // import | recording | screenshot
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

// Extensions accepted by Import. Screenshots are written as PNG but are not
// imported: they have no media streams to transcribe.
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
	return s.AddFiles(paths)
}

// AddFiles probes existing files and adds them to the library, as recordings:
// the files Ottr wrote that no record holds any more, found by Orphans.
func (s *Service) AddFiles(paths []string) ([]Media, error) {
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	added := []Media{}
	var errs []string
	for _, path := range paths {
		m, err := s.add(context.Background(), db, path, "recording")
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(path), err))
			continue
		}
		added = append(added, m)
	}
	if len(added) > 0 {
		Changed.Broadcast(struct{}{})
	}
	if len(errs) > 0 {
		return added, errors.New(strings.Join(errs, "\n"))
	}
	return added, nil
}

// Orphans lists the media files in dir, the folder Ottr writes recordings to,
// that no record refers to: files kept when their record was deleted, or left
// by a recording that never landed in the library.
func (s *Service) Orphans(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	known := map[string]bool{}
	rows, err := db.Query(`SELECT path FROM media`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		known[filepath.Clean(path)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var orphans []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if known[filepath.Clean(path)] {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(e.Name()), "."))
		if !slices.Contains(Extensions, ext) {
			continue // not a file Ottr wrote
		}
		orphans = append(orphans, path)
	}
	sort.Strings(orphans)
	return orphans, nil
}

// DeleteFiles removes files from disk. It only ever touches files, never
// records: deleting a record stays Remove's business.
func (s *Service) DeleteFiles(paths []string) error {
	var errs []string
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(path), err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
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

// Remove deletes the record, and the file with it when withFile is set. It
// asks nothing: the caller confirms, so that the window that asked and the
// window that answers are the same.
func (s *Service) Remove(id int64, withFile bool) error {
	db, err := s.store()
	if err != nil {
		return err
	}
	m, err := s.Get(id)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM media WHERE id = ?`, id); err != nil {
		return err
	}
	if withFile {
		if err := os.Remove(m.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("record removed, but deleting the file failed: %w", err)
		}
	}
	Changed.Broadcast(struct{}{})
	return nil
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

// Record is what a caller that already knows a file's metadata — native
// capture does — tells the library about it, without probing anything.
type Record struct {
	Name       string
	Path       string
	DurationMs int64
	Width      int
	Height     int
	Size       int64
	HasAudio   bool
	HasVideo   bool
	Ext        string // written with sources "recording"; defaults to path's
}

// AddScreenshot adds an image to the library. A screenshot is its own source,
// has no duration and is never transcribed.
func (s *Service) AddScreenshot(r Record) (Media, error) {
	if r.Size == 0 {
		if st, err := os.Stat(r.Path); err == nil {
			r.Size = st.Size()
		}
	}
	return s.insert(r, "screenshot")
}

// AddNativeRecording adds a finished recording from the metadata its writer
// reported, instead of probing the file.
func (s *Service) AddNativeRecording(r Record) (Media, error) {
	return s.insert(r, "recording")
}

func (s *Service) insert(r Record, source string) (Media, error) {
	db, err := s.store()
	if err != nil {
		return Media{}, err
	}
	abs, err := filepath.Abs(r.Path)
	if err != nil {
		abs = r.Path
	}
	name := r.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(r.Path), filepath.Ext(r.Path))
	}
	res, err := db.Exec(`INSERT INTO media
		(name, source, path, duration_ms, width, height, size, has_audio, has_video, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, source, abs, r.DurationMs, r.Width, r.Height, r.Size,
		r.HasAudio, r.HasVideo, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Media{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Media{}, err
	}
	Changed.Broadcast(struct{}{})
	return s.Get(id)
}
