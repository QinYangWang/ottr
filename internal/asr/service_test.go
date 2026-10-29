package asr

import (
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
)

func setupTestASR(t *testing.T) (*Service, *library.Service) {
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
	svc := NewService(lib, mm, js)
	svc.Open(db)
	return svc, lib
}

func TestTranscriptSaveAndGet(t *testing.T) {
	svc, lib := setupTestASR(t)
	db, _ := svc.store()

	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio)
		VALUES ('test', 'import', '/tmp/test.mp4', ?, 1)`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := res.LastInsertId()

	segs := []Segment{
		{StartMs: 0, EndMs: 2000, Text: "First sentence."},
		{StartMs: 2500, EndMs: 5000, Text: "Second sentence."},
	}
	if err := svc.saveTranscript(mediaID, segs); err != nil {
		t.Fatal(err)
	}

	tr, err := svc.Get(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Segments) != 2 {
		t.Fatalf("want 2 segments, got %d", len(tr.Segments))
	}
	if tr.Segments[0].Text != "First sentence." {
		t.Errorf("seg 0: %q", tr.Segments[0].Text)
	}

	// Verify media status updated to done
	m, err := lib.Get(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if m.TranscriptStatus != "done" {
		t.Errorf("status: %s", m.TranscriptStatus)
	}

	// Search
	results, err := svc.Search(mediaID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Text != "Second sentence." {
		t.Fatalf("search result: %+v", results)
	}
}
