package translate

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
)

func setupTestTranslate(t *testing.T) (*Service, *asr.Service, *library.Service) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	lib := &library.Service{}
	lib.Open(db)

	js := &jobs.Service{}
	js.Open(db)
	mm := models.NewManager(t.TempDir(), t.TempDir())
	asrSvc := asr.NewService(lib, mm, js)
	asrSvc.Open(db)

	trSvc := NewService(lib, asrSvc, js)
	trSvc.Open(db)

	return trSvc, asrSvc, lib
}

func TestSettingsPersist(t *testing.T) {
	trSvc, _, _ := setupTestTranslate(t)
	cfg := Settings{
		Provider: "custom",
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-test-123",
		Model:    "gpt-4o",
	}
	if err := trSvc.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	got := trSvc.GetSettings()
	if got.Provider != "custom" || got.BaseURL != "https://api.openai.com/v1" || got.APIKey != "sk-test-123" || got.Model != "gpt-4o" {
		t.Fatalf("settings mismatch: %+v", got)
	}
}

func TestRealOnlineTranslation(t *testing.T) {
	trSvc, asrSvc, lib := setupTestTranslate(t)

	// Create test media
	db, _ := trSvc.store()
	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio, transcript_status)
		VALUES ('demo', 'import', '/tmp/demo.mp4', ?, 1, 'done')`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := res.LastInsertId()

	// Insert original transcript segments
	resTr, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'zh', ?)`,
		mediaID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	trID, _ := resTr.LastInsertId()
	_, err = db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text) VALUES (?, 1, 0, 1500, '你好世界')`, trID)
	if err != nil {
		t.Fatal(err)
	}

	// Translate to English using real online MyMemory endpoint
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tr, err := trSvc.Translate(ctx, mediaID, "en")
	if err != nil {
		t.Skipf("online translation endpoint unreachable: %v", err)
	}
	if len(tr.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(tr.Segments))
	}
	t.Logf("Translated text: %q", tr.Segments[0].TranslatedText)
	if tr.Segments[0].TranslatedText == "" {
		t.Fatal("empty translation")
	}

	// Test Get from SQLite
	loaded, err := trSvc.Get(mediaID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Segments) != 1 || loaded.Segments[0].TranslatedText != tr.Segments[0].TranslatedText {
		t.Fatalf("loaded translation mismatch: %+v", loaded)
	}
	_ = asrSvc
	_ = lib
}
