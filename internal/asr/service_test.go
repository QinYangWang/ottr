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

func TestSpeakerRenameIsGlobal(t *testing.T) {
	svc, _ := setupTestASR(t)
	db, _ := svc.store()
	res, err := db.Exec(`INSERT INTO speaker (name, embedding, created_at) VALUES ('Speaker 1', x'00', datetime('now'))`)
	if err != nil {
		t.Fatal(err)
	}
	id64, _ := res.LastInsertId()
	id := int(id64)
	if err := svc.RenameSpeaker(id, "Chen Ligan"); err != nil {
		t.Fatal(err)
	}
	names, err := svc.speakerNames()
	if err != nil {
		t.Fatal(err)
	}
	if names[id] != "Chen Ligan" {
		t.Fatalf("speaker name = %q", names[id])
	}
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

// The library counts each speaker once per recording they were heard on, with
// the recording's length and date, and drops a speaker when asked.
func TestSpeakerLibrary(t *testing.T) {
	svc, _ := setupTestASR(t)
	db, _ := svc.store()
	newSpeaker := func(name string) int {
		res, err := db.Exec(`INSERT INTO speaker (name, embedding, created_at) VALUES (?, x'00', datetime('now'))`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	chen, ana := newSpeaker("Chen Ligan"), newSpeaker("Ana Ruiz")

	addMedia := func(name string, minutes int) int64 {
		res, err := db.Exec(`INSERT INTO media (name, source, path, duration_ms, created_at, has_audio)
			VALUES (?, 'recording', '/tmp/x.mp4', ?, ?, 1)`, name, minutes*60000, "2026-11-05T13:55:00Z")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	addSegment := func(mediaID int64, speaker int) {
		res, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'en', datetime('now'))`, mediaID)
		if err != nil {
			t.Fatal(err)
		}
		trID, _ := res.LastInsertId()
		if _, err := db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text, speaker)
			VALUES (?, 1, 0, 1000, 'hi', ?)`, trID, speaker); err != nil {
			t.Fatal(err)
		}
	}

	long, short := addMedia("Weekly sync", 42), addMedia("Bug triage", 18)
	addSegment(long, chen)
	addSegment(long, ana)
	addSegment(short, chen)

	speakers, err := svc.Speakers()
	if err != nil {
		t.Fatal(err)
	}
	if len(speakers) != 2 {
		t.Fatalf("library has %d speakers: %+v", len(speakers), speakers)
	}
	top := speakers[0]
	if top.ID != chen || top.Name != "Chen Ligan" || top.Recordings != 2 || top.DurationMs != 60*60000 {
		t.Errorf("most heard speaker: %+v", top)
	}
	if speakers[1].Recordings != 1 || speakers[1].DurationMs != 42*60000 {
		t.Errorf("second speaker: %+v", speakers[1])
	}

	if err := svc.RemoveSpeaker(ana); err != nil {
		t.Fatal(err)
	}
	speakers, err = svc.Speakers()
	if err != nil {
		t.Fatal(err)
	}
	if len(speakers) != 1 || speakers[0].ID != chen {
		t.Fatalf("after remove: %+v", speakers)
	}
}
