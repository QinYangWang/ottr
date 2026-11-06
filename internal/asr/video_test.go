package asr

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
)

// TestVideoTranscribes covers an imported video with a supported offline model.
// OTTR_TEST_VIDEO and OTTR_TEST_OFFLINE_MODEL opt into real local fixtures.
func TestVideoTranscribes(t *testing.T) {
	video := os.Getenv("OTTR_TEST_VIDEO")
	if video == "" {
		t.Skip("set OTTR_TEST_VIDEO to a real speech video")
	}
	modelDir := os.Getenv("OTTR_TEST_OFFLINE_MODEL")
	if modelDir == "" {
		t.Skip("set OTTR_TEST_OFFLINE_MODEL to a supported model directory")
	}
	if _, err := os.Stat(modelDir); err != nil {
		t.Skipf("no offline Japanese model: %v", err)
	}

	db, err := storage.Open(filepath.Join(t.TempDir(), "example.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lib := &library.Service{}
	lib.Open(db)
	js := &jobs.Service{}
	js.Open(db)
	imported, err := lib.AddFiles([]string{video})
	if err != nil || len(imported) != 1 {
		t.Fatalf("import: %v (%d files)", err, len(imported))
	}

	mm := models.NewManager(modelDir, filepath.Join("../../resources", runtime.GOOS+"-"+runtime.GOARCH, "bin"))
	svc := NewService(lib, mm, js)
	svc.Open(db)
	if err := svc.TranscribeNow(context.Background(), imported[0].ID, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	tr, err := svc.Get(imported[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, seg := range tr.Segments {
		text.WriteString(seg.Text)
	}
	if text.Len() == 0 {
		t.Fatal("the test video produced an empty transcript")
	}
	t.Logf("%d segments, %d characters: %s", len(tr.Segments), text.Len(), text.String())

	var liveErr string
	sink, err := svc.StartLive(16000, 1, func(sub LiveSubtitle) { liveErr = sub.Error })
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 16000*4)
	if _, err := sink.Write(pcm); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if liveErr != "" {
		t.Fatalf("offline live transcription: %s", liveErr)
	}
}
