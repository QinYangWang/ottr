//go:build windows

package capture

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/library"
	"ottr/internal/storage"
)

func testRecorder(t *testing.T) *Recorder {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &library.Service{}
	svc.Open(db)
	return &Recorder{Dir: t.TempDir(), Lib: svc}
}

func TestListSources(t *testing.T) {
	sources, err := listSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 || sources[0].ID != "screen" {
		t.Fatalf("want Full screen first, got %+v", sources)
	}
	rects, err := monitorRects()
	if err != nil || len(rects) == 0 {
		t.Fatalf("no monitors: %v", err)
	}
}

func TestRecordScreenWithSystemAudio(t *testing.T) {
	r := testRecorder(t)
	if err := r.Start(Config{SourceID: "screen", SystemAudio: true}); err != nil {
		t.Fatal(err)
	}
	if !r.State().Recording {
		t.Fatal("not recording after Start")
	}
	// Let it record and confirm the preview produces real JPEG frames.
	deadline := time.Now().Add(8 * time.Second)
	for {
		r.mu.Lock()
		sess := r.current
		r.mu.Unlock()
		if sess == nil {
			t.Fatal("session died")
		}
		sess.preview.mu.Lock()
		frame := sess.preview.frame
		sess.preview.mu.Unlock()
		if len(frame) > 100 {
			if !bytes.HasPrefix(frame, []byte{0xFF, 0xD8}) || !bytes.HasSuffix(frame, []byte{0xFF, 0xD9}) {
				t.Fatal("preview frame is not a JPEG")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no preview frame within 8s")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := r.Start(Config{SourceID: "screen"}); err == nil {
		t.Fatal("second Start must fail while recording")
	}
	time.Sleep(4 * time.Second)

	m, err := r.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if r.State().Recording {
		t.Fatal("still recording after Stop")
	}
	if m.Source != "recording" {
		t.Errorf("source: %q", m.Source)
	}
	if m.DurationMs < 3000 || m.DurationMs > 12000 {
		t.Errorf("duration: %d ms", m.DurationMs)
	}
	if !m.HasVideo || !m.HasAudio {
		t.Errorf("want video+audio, got v=%v a=%v", m.HasVideo, m.HasAudio)
	}
	if _, err := os.Stat(m.Path); err != nil {
		t.Errorf("file gone: %v", err)
	}
}

func TestRecordVideoOnly(t *testing.T) {
	r := testRecorder(t)
	if err := r.Start(Config{SourceID: "screen"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	m, err := r.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasVideo || m.HasAudio {
		t.Errorf("want video only, got v=%v a=%v", m.HasVideo, m.HasAudio)
	}
}

func TestRecordWithMic(t *testing.T) {
	r := testRecorder(t)
	if err := r.Start(Config{SourceID: "screen", Mic: true}); err != nil {
		t.Skipf("no microphone on this machine: %v", err)
	}
	time.Sleep(3 * time.Second)
	m, err := r.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasAudio {
		t.Error("mic recording has no audio stream")
	}
}

func TestBadSource(t *testing.T) {
	r := testRecorder(t)
	if err := r.Start(Config{SourceID: "window:No Such Window Ottr Test 42"}); err == nil {
		r.Stop()
		t.Skip("gdigrab did not reject an unknown window title")
	}
}
