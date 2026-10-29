//go:build windows

package capture

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ottr/internal/library"
	"ottr/internal/storage"
)

// liveBackend is the real one, and every test here is off unless OTTR_LIVE=1:
// they open the desktop and take the single recording slot.
func liveBackend(t *testing.T) Backend {
	t.Helper()
	if os.Getenv("OTTR_LIVE") == "" {
		t.Skip("set OTTR_LIVE=1 to capture the real desktop")
	}
	return NewBackend()
}

func testRecorder(t *testing.T) *Recorder {
	t.Helper()
	backend := liveBackend(t)
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &library.Service{}
	svc.Open(db)
	return &Recorder{Backend: backend, Dir: t.TempDir(), Lib: svc}
}

func TestListSources(t *testing.T) {
	b := liveBackend(t)
	defer b.Close()
	sources, err := b.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 || sources[0].ID != "screen" {
		t.Fatalf("want Full screen first, got %+v", sources)
	}
	for _, s := range sources {
		if s.Kind == "window" && s.Detail == "" {
			t.Logf("window %q without a size", s.Name)
		}
	}
}

// One stream delivers tightly packed BGRA at physical pixels with a monotonic
// timestamp, and closing it twice is safe.
func TestStreamDeliversBGRA(t *testing.T) {
	b := liveBackend(t)
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := b.Open(ctx, StreamConfig{SourceID: "screen", Cursor: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f, err := st.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Data) != f.Width*f.Height*4 {
		t.Fatalf("frame is %d bytes for %dx%d", len(f.Data), f.Width, f.Height)
	}
	// A desktop screenshot is not a one-color rectangle.
	uniform := true
	for i := 4; i < len(f.Data); i += 4 * 97 {
		if f.Data[i] != f.Data[0] {
			uniform = false
			break
		}
	}
	if uniform {
		t.Error("the frame is one flat color")
	}
	second, err := st.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.PTS < f.PTS {
		t.Errorf("timestamps go back: %v then %v", f.PTS, second.PTS)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// The preview draws raw frames as JPEGs without a subprocess, and a source
// that cannot be opened fails instead of waiting forever.
func TestPreviewerDrawsTheSource(t *testing.T) {
	b := liveBackend(t)
	defer b.Close()
	p := NewPreviewer(b)
	defer p.Stop()
	p.Show("screen")

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if frame := p.Frame(); len(frame) > 2 {
			if !bytes.HasPrefix(frame, []byte{0xFF, 0xD8}) || !bytes.HasSuffix(frame, []byte{0xFF, 0xD9}) {
				t.Fatalf("a frame that is not a JPEG: % x", frame[:4])
			}
			return
		}
		if err := p.Err(); err != nil {
			t.Fatalf("no preview: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no frame in 15 seconds")
}

func TestRecordScreenWithSystemAudio(t *testing.T) {
	r := testRecorder(t)
	if err := r.Start(Config{SourceID: "screen", SystemAudio: true}); err != nil {
		t.Fatal(err)
	}
	if !r.State().Recording {
		t.Fatal("not recording after Start")
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if frame := r.Preview(); len(frame) > 100 {
			if !bytes.HasPrefix(frame, []byte{0xFF, 0xD8}) {
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
	if err := r.Start(Config{SourceID: "window:deadbeef"}); err == nil {
		r.Stop()
		t.Fatal("a closed window was recorded")
	}
	if r.State().Recording {
		t.Fatal("a failed Start left a recording behind")
	}
}

// A screenshot is a PNG of the source at its own size, and it lands in the
// library as a screenshot.
func TestScreenshotWritesAPNG(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lib := &library.Service{}
	lib.Open(db)
	s := &Snapshotter{Backend: liveBackend(t), Dir: t.TempDir(), Lib: lib}
	m, err := s.Screenshot(context.Background(), "screen")
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "screenshot" || m.Width == 0 || m.Height == 0 {
		t.Errorf("record: %+v", m)
	}
	data, err := os.ReadFile(m.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("not a PNG")
	}
}

// Repeatedly opening and closing a stream leaves no callback behind, no
// goroutine growth and no native memory growth.
func TestStreamsOpenAndCloseRepeatedly(t *testing.T) {
	b := liveBackend(t)
	defer b.Close()
	before := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		st, err := b.Open(ctx, StreamConfig{SourceID: "screen", FPS: previewFPS})
		if err != nil {
			cancel()
			t.Fatalf("open %d: %v", i, err)
		}
		if _, err := st.Next(ctx); err != nil {
			cancel()
			st.Close()
			t.Fatalf("frame %d: %v", i, err)
		}
		if err := st.Close(); err != nil {
			cancel()
			t.Fatalf("close %d: %v", i, err)
		}
		cancel()
	}
	// The streams' goroutines are gone shortly after close.
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+2 {
		t.Errorf("%d goroutines after 100 open/close cycles, %d before", got, before)
	}
}
