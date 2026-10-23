package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProbe(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	cmd := exec.Command(ffmpeg, "-y", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-shortest", path)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make fixture: %v\n%s", err, out)
	}
	info, err := Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasVideo || !info.HasAudio {
		t.Errorf("want audio+video, got %+v", info)
	}
	if info.Width != 320 || info.Height != 240 {
		t.Errorf("want 320x240, got %dx%d", info.Width, info.Height)
	}
	if info.DurationMs < 900 || info.DurationMs > 1100 {
		t.Errorf("want ~1000ms, got %d", info.DurationMs)
	}
	st, _ := os.Stat(path)
	if info.Size != st.Size() {
		t.Errorf("size: got %d want %d", info.Size, st.Size())
	}
}

func TestProbeMissing(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	if _, err := Probe(context.Background(), filepath.Join(t.TempDir(), "nope.mp4")); err == nil {
		t.Fatal("want error for missing file")
	}
}
