// Package translate manages subtitle translation, persistence and export.
package translate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"ottr/internal/asr"
	"ottr/internal/library"
)

// TranslationReady is broadcast when a translation completes.
var TranslationReady = mygo.NewEvent[Translation]("translation:ready")

// Service coordinates subtitle translation.
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
			Provider: "mymemory",
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
	var provider, baseURL, apiKey, model string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'trans_provider'`).Scan(&provider)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'trans_base_url'`).Scan(&baseURL)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'trans_api_key'`).Scan(&apiKey)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'trans_model'`).Scan(&model)

	if provider != "" {
		s.settings.Provider = provider
	}
	s.settings.BaseURL = baseURL
	s.settings.APIKey = apiKey
	s.settings.Model = model
}

// GetSettings returns the current online translation configuration.
func (s *Service) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// SaveSettings updates and persists translation settings.
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
	if _, err := tx.Exec(upsert, "trans_provider", cfg.Provider); err != nil {
		return err
	}
	if _, err := tx.Exec(upsert, "trans_base_url", cfg.BaseURL); err != nil {
		return err
	}
	if _, err := tx.Exec(upsert, "trans_api_key", cfg.APIKey); err != nil {
		return err
	}
	if _, err := tx.Exec(upsert, "trans_model", cfg.Model); err != nil {
		return err
	}
	return tx.Commit()
}

// Get retrieves an existing translation by media ID and target language.
func (s *Service) Get(mediaID int64, targetLang string) (Translation, error) {
	db, err := s.store()
	if err != nil {
		return Translation{}, err
	}
	targetLang = strings.ToLower(strings.TrimSpace(targetLang))
	var tr Translation
	err = db.QueryRow(`SELECT id, media_id, target_lang, source_lang, created_at
		FROM translation WHERE media_id = ? AND target_lang = ?`, mediaID, targetLang).
		Scan(&tr.ID, &tr.MediaID, &tr.TargetLang, &tr.SourceLang, &tr.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Translation{MediaID: mediaID, TargetLang: targetLang}, nil
	}
	if err != nil {
		return Translation{}, err
	}

	rows, err := db.Query(`SELECT id, segment_id, idx, start_ms, end_ms, original_text, translated_text
		FROM translation_segment WHERE translation_id = ? ORDER BY idx ASC`, tr.ID)
	if err != nil {
		return Translation{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var seg TranslationSegment
		if err := rows.Scan(&seg.ID, &seg.SegmentID, &seg.Idx, &seg.StartMs, &seg.EndMs, &seg.OriginalText, &seg.TranslatedText); err != nil {
			return Translation{}, err
		}
		tr.Segments = append(tr.Segments, seg)
	}
	return tr, rows.Err()
}

// Translate translates the transcript of the media into targetLang.
func (s *Service) Translate(ctx context.Context, mediaID int64, targetLang string) (Translation, error) {
	targetLang = strings.ToLower(strings.TrimSpace(targetLang))
	if targetLang == "" {
		return Translation{}, errors.New("target language is required")
	}

	orig, err := s.asr.Get(mediaID)
	if err != nil {
		return Translation{}, err
	}
	if len(orig.Segments) == 0 {
		return Translation{}, errors.New("no transcript found for this media; please transcribe first")
	}

	s.mu.Lock()
	cfg := s.settings
	s.mu.Unlock()

	var translator Translator
	if cfg.Provider == "custom" && cfg.BaseURL != "" {
		translator = NewOpenAICompatibleTranslator(cfg.BaseURL, cfg.APIKey, cfg.Model)
	} else {
		translator = NewMyMemoryTranslator()
	}

	texts := make([]string, len(orig.Segments))
	for i, s := range orig.Segments {
		texts[i] = s.Text
	}

	srcLang := orig.Language
	if srcLang == "" {
		srcLang = "zh"
	}

	translatedTexts, err := translator.Translate(ctx, texts, srcLang, targetLang)
	if err != nil {
		return Translation{}, fmt.Errorf("translation failed: %w", err)
	}

	db, err := s.store()
	if err != nil {
		return Translation{}, err
	}

	tx, err := db.Begin()
	if err != nil {
		return Translation{}, err
	}
	defer tx.Rollback()

	// Clear previous translation for this target language if any
	tx.Exec(`DELETE FROM translation WHERE media_id = ? AND target_lang = ?`, mediaID, targetLang)

	res, err := tx.Exec(`INSERT INTO translation (media_id, target_lang, source_lang, created_at)
		VALUES (?, ?, ?, ?)`, mediaID, targetLang, srcLang, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Translation{}, err
	}
	trID, err := res.LastInsertId()
	if err != nil {
		return Translation{}, err
	}

	stmt, err := tx.Prepare(`INSERT INTO translation_segment
		(translation_id, segment_id, idx, start_ms, end_ms, original_text, translated_text)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return Translation{}, err
	}
	defer stmt.Close()

	var segments []TranslationSegment
	for i, origSeg := range orig.Segments {
		transText := ""
		if i < len(translatedTexts) {
			transText = translatedTexts[i]
		}
		if transText == "" {
			transText = origSeg.Text // fallback to original if untranslated
		}
		res, err := stmt.Exec(trID, origSeg.ID, origSeg.Idx, origSeg.StartMs, origSeg.EndMs, origSeg.Text, transText)
		if err != nil {
			return Translation{}, err
		}
		segID, _ := res.LastInsertId()
		segments = append(segments, TranslationSegment{
			ID:             segID,
			SegmentID:      origSeg.ID,
			Idx:            origSeg.Idx,
			StartMs:        origSeg.StartMs,
			EndMs:          origSeg.EndMs,
			OriginalText:   origSeg.Text,
			TranslatedText: transText,
		})
	}

	if err := tx.Commit(); err != nil {
		return Translation{}, err
	}

	result := Translation{
		ID:         trID,
		MediaID:    mediaID,
		TargetLang: targetLang,
		SourceLang: srcLang,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		Segments:   segments,
	}
	TranslationReady.Broadcast(result)
	return result, nil
}

// Export saves the translated subtitles to an SRT, VTT, TXT or bilingual SRT file.
func (s *Service) Export(ctx context.Context, mediaID int64, targetLang, format string) (string, error) {
	tr, err := s.Get(mediaID, targetLang)
	if err != nil {
		return "", err
	}
	if len(tr.Segments) == 0 {
		return "", errors.New("no translated subtitles found to export")
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
		ext = fmt.Sprintf("%s.srt", targetLang)
	case "bilingual_srt", "bi_srt":
		content = FormatBilingualSRT(tr.Segments)
		ext = fmt.Sprintf("bilingual_%s.srt", targetLang)
	case "vtt":
		content = FormatVTT(tr.Segments)
		ext = fmt.Sprintf("%s.vtt", targetLang)
	case "txt":
		content = FormatTXT(tr.Segments)
		ext = fmt.Sprintf("%s.txt", targetLang)
	default:
		return "", fmt.Errorf("unsupported export format: %s", format)
	}

	defaultName := fmt.Sprintf("%s.%s", m.Name, ext)
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       "Export Translated Subtitles",
		DefaultPath: defaultName,
	})
	if err != nil || path == "" {
		return "", err
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write subtitle file: %w", err)
	}
	return path, nil
}
