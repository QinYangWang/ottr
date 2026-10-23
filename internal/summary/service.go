// Package summary coordinates AI summarization of media transcripts.
package summary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/egoist/mygo"

	"ottr/internal/asr"
	"ottr/internal/library"
)

// SummaryReady is broadcast when an AI summary is generated.
var SummaryReady = mygo.NewEvent[Summary]("summary:ready")

// Service coordinates summary generation and persistence.
type Service struct {
	mu       sync.Mutex
	db       *sql.DB
	lib      *library.Service
	asr      *asr.Service
	settings Settings
}

func NewService(lib *library.Service, asrSvc *asr.Service) *Service {
	return &Service{
		lib: lib,
		asr: asrSvc,
		settings: Settings{
			BaseURL: "https://api.deepseek.com/v1",
			Model:   "deepseek-chat",
		},
	}
}

// Open connects the database and loads saved settings.
func (s *Service) Open(db *sql.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.loadSettingsLocked()
}

func (s *Service) store() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, errors.New("database not ready")
	}
	return s.db, nil
}

func (s *Service) loadSettingsLocked() {
	if s.db == nil {
		return
	}
	s.db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`)
	var baseURL, apiKey, model string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'llm_base_url'`).Scan(&baseURL)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'llm_api_key'`).Scan(&apiKey)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'llm_model'`).Scan(&model)

	if baseURL != "" {
		s.settings.BaseURL = baseURL
	}
	if apiKey != "" {
		s.settings.APIKey = apiKey
	}
	if model != "" {
		s.settings.Model = model
	}
}

// GetSettings returns the current remote AI model configuration.
func (s *Service) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// SaveSettings updates and persists the AI model configuration.
func (s *Service) SaveSettings(cfg Settings) error {
	db, err := s.store()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.settings = cfg
	s.mu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	upsert := `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	if _, err := tx.Exec(upsert, "llm_base_url", cfg.BaseURL); err != nil {
		return err
	}
	if _, err := tx.Exec(upsert, "llm_api_key", cfg.APIKey); err != nil {
		return err
	}
	if _, err := tx.Exec(upsert, "llm_model", cfg.Model); err != nil {
		return err
	}
	return tx.Commit()
}

// Get retrieves an existing summary for a media item.
func (s *Service) Get(mediaID int64) (Summary, error) {
	db, err := s.store()
	if err != nil {
		return Summary{}, err
	}
	var sum Summary
	var kpJSON, aiJSON, chJSON, hlJSON string
	err = db.QueryRow(`SELECT id, media_id, content_markdown, overview,
		key_points_json, action_items_json, chapters_json, highlights_json,
		model, created_at FROM summary WHERE media_id = ?`, mediaID).
		Scan(&sum.ID, &sum.MediaID, &sum.ContentMarkdown, &sum.Overview,
			&kpJSON, &aiJSON, &chJSON, &hlJSON, &sum.Model, &sum.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Summary{MediaID: mediaID}, nil
	}
	if err != nil {
		return Summary{}, err
	}

	_ = json.Unmarshal([]byte(kpJSON), &sum.KeyPoints)
	_ = json.Unmarshal([]byte(aiJSON), &sum.ActionItems)
	_ = json.Unmarshal([]byte(chJSON), &sum.Chapters)
	_ = json.Unmarshal([]byte(hlJSON), &sum.Highlights)

	return sum, nil
}

// Generate generates a structured summary using the remote AI API.
func (s *Service) Generate(ctx context.Context, mediaID int64) (Summary, error) {
	m, err := s.lib.Get(mediaID)
	if err != nil {
		return Summary{}, err
	}

	tr, err := s.asr.Get(mediaID)
	if err != nil {
		return Summary{}, err
	}
	if len(tr.Segments) == 0 {
		return Summary{}, errors.New("no transcript found; please transcribe the media first")
	}

	s.mu.Lock()
	cfg := s.settings
	s.mu.Unlock()

	db, err := s.store()
	if err != nil {
		return Summary{}, err
	}

	db.Exec(`UPDATE media SET summary_status = 'pending' WHERE id = ?`, mediaID)
	library.Changed.Broadcast(struct{}{})

	res, err := GenerateSummary(ctx, cfg, m.Name, tr)
	if err != nil {
		db.Exec(`UPDATE media SET summary_status = 'error' WHERE id = ?`, mediaID)
		library.Changed.Broadcast(struct{}{})
		return Summary{}, err
	}

	res.MediaID = mediaID

	kpJSON, _ := json.Marshal(res.KeyPoints)
	aiJSON, _ := json.Marshal(res.ActionItems)
	chJSON, _ := json.Marshal(res.Chapters)
	hlJSON, _ := json.Marshal(res.Highlights)

	tx, err := db.Begin()
	if err != nil {
		return Summary{}, err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM summary WHERE media_id = ?`, mediaID)
	insertRes, err := tx.Exec(`INSERT INTO summary
		(media_id, content_markdown, overview, key_points_json, action_items_json, chapters_json, highlights_json, model, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mediaID, res.ContentMarkdown, res.Overview, string(kpJSON), string(aiJSON), string(chJSON), string(hlJSON),
		res.Model, res.CreatedAt)
	if err != nil {
		return Summary{}, err
	}
	res.ID, _ = insertRes.LastInsertId()

	if _, err := tx.Exec(`UPDATE media SET summary_status = 'done' WHERE id = ?`, mediaID); err != nil {
		return Summary{}, err
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}

	SummaryReady.Broadcast(*res)
	library.Changed.Broadcast(struct{}{})
	return *res, nil
}

// Export saves the Markdown summary to a local .md file.
func (s *Service) Export(ctx context.Context, mediaID int64) (string, error) {
	sum, err := s.Get(mediaID)
	if err != nil {
		return "", err
	}
	if sum.ContentMarkdown == "" {
		return "", errors.New("no summary available to export")
	}

	m, err := s.lib.Get(mediaID)
	if err != nil {
		return "", err
	}

	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       "Export Markdown Summary",
		DefaultPath: fmt.Sprintf("%s — Summary.md", m.Name),
		Filters: []mygo.FileFilter{
			{Name: "Markdown Files", Extensions: []string{"md", "markdown"}},
		},
	})
	if err != nil || path == "" {
		return "", err
	}

	if err := os.WriteFile(path, []byte(sum.ContentMarkdown), 0o644); err != nil {
		return "", fmt.Errorf("write summary file: %w", err)
	}
	return path, nil
}
