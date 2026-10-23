package native

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
	"ottr/internal/summary"
	"ottr/internal/translate"
)

// newTestApp builds an app over a real database, with one transcribed media
// file in it.
func newTestApp(t *testing.T) (*App, int64) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	lib := &library.Service{}
	lib.Open(db)
	mm := models.NewManager(t.TempDir(), t.TempDir())
	asrSvc := asr.NewService(lib, mm)
	trSvc, sumSvc := translate.NewService(lib, asrSvc), summary.NewService(lib, asrSvc)
	asrSvc.Open(db)
	trSvc.Open(db)
	sumSvc.Open(db)
	a := &App{
		Lib: lib,
		Rec: &capture.Recorder{Lib: lib, Dir: t.TempDir()},
		ASR: asrSvc, Trans: trSvc, Sum: sumSvc,
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO media (name, source, path, duration_ms, has_audio, has_video, created_at, transcript_status)
		VALUES ('standup', 'recording', '/tmp/standup.mp4', 61000, 1, 1, ?, 'done')`, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	tr, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'zh', ?)`, id, now)
	if err != nil {
		t.Fatal(err)
	}
	trID, _ := tr.LastInsertId()
	if _, err := db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text)
		VALUES (?, 1, 0, 2000, 'first line'), (?, 2, 2000, 4000, 'second line')`, trID, trID); err != nil {
		t.Fatal(err)
	}
	a.reload()
	return a, id
}

// The window lists what it recorded and shows the transcript of the file
// chosen, which its tabs switch away from and back to.
func TestWindowShowsTranscript(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	// The library lists the file, and the transcript tab shows its lines.
	if !tt.HasText("standup") {
		t.Fatalf("the library does not list the recording: %q", tt.Texts())
	}
	tt.Frame()
	for i := 0; i < 5 && !tt.HasText("first line"); i++ {
		tt.Frame()
	}
	if !tt.HasText("first line") || !tt.HasText("second line") {
		t.Fatalf("the transcript did not show: %q", tt.Texts())
	}
	// The other tabs ask for what they need rather than showing nothing.
	if err := tt.Click("Summary"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5 && !tt.HasText("No summary yet"); i++ {
		tt.Frame()
	}
	if !tt.HasText("No summary yet") {
		t.Errorf("the Summary tab says %q", tt.Texts())
	}
	if err := tt.Click("Transcript"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("second line") {
		t.Errorf("back on Transcript, the window says %q", tt.Texts())
	}
}

// Searching the library keeps the file listed while it matches, and says so
// when nothing does.
func TestSearchFilters(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	a.query = "nothing-like-this"
	a.reload()
	tt.Frame()
	if !tt.HasText("Nothing matches that search.") {
		t.Errorf("search: %q", tt.Texts())
	}
	a.query = "stand"
	a.reload()
	tt.Frame()
	if !tt.HasText("standup") {
		t.Errorf("search kept: %q", tt.Texts())
	}
}

// Deleting a record leaves the library empty, and the window says so.
func TestDeleteMedia(t *testing.T) {
	a, id := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	a.removeMedia(id, false)
	for i := 0; i < 20 && len(a.media) > 0; i++ {
		time.Sleep(10 * time.Millisecond)
		tt.Frame()
	}
	if len(a.media) != 0 {
		t.Fatalf("still %d records", len(a.media))
	}
	if !tt.HasText("Nothing yet: record the screen, or import a file.") {
		t.Errorf("the empty library says %q", tt.Texts())
	}
}
