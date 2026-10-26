// Package settings reads and writes the app's key-value settings, the table
// migration 0006 created. The services whose configuration lives there keep
// their own keys; this is for what belongs to the window itself.
package settings

import (
	"database/sql"
	"errors"
	"sync"
)

// Keys the window reads.
const (
	// Theme is "system", "light" or "dark".
	Theme = "ui_theme"
	// FFmpegPath is a folder, or the ffmpeg program itself, chosen in the
	// settings when Ottr could not find FFmpeg on its own.
	FFmpegPath = "ffmpeg_path"
)

// Service reads and writes settings.
type Service struct {
	mu sync.Mutex
	db *sql.DB
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
		return nil, errors.New("settings are not ready")
	}
	return s.db, nil
}

// Get returns the value of a key, or "" when it was never set.
func (s *Service) Get(key string) string {
	db, err := s.store()
	if err != nil {
		return ""
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil {
		return ""
	}
	return value
}

// Set stores a value, replacing what the key held.
func (s *Service) Set(key, value string) error {
	db, err := s.store()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
