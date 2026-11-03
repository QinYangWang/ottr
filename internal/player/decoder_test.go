package player

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/tools"
)

func testClip(t *testing.T) string {
	t.Helper()
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		t.Skip(err)
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	cmd := exec.Command(ffmpeg, "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "0.25", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", path)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot make an H.264/AAC clip: %v (%s)", err, out)
	}
	return path
}

func TestVideoDecoderReadsAndRewinds(t *testing.T) {
	d, err := OpenVideo(context.Background(), testClip(t), 320, 240)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for i := 0; i < 2; i++ {
		frame, err := d.Next()
		if err != nil {
			t.Fatal(err)
		}
		if frame.Image.Bounds().Dx() != 320 || frame.Image.Bounds().Dy() != 240 {
			t.Fatalf("frame is %v", frame.Image.Bounds())
		}
	}
	if err := d.Rewind(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Next(); err != nil {
		t.Fatal(err)
	}
}

func TestVideoDecoderOpensAtPosition(t *testing.T) {
	d, err := OpenVideoAt(context.Background(), testClip(t), 320, 240, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Next(); err != nil {
		t.Fatal(err)
	}
}

func TestAudioDecoderReadsPCM(t *testing.T) {
	d, err := OpenAudio(context.Background(), testClip(t), 250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.SampleRate() != audioRate || d.Channels() != 2 || d.Len() != 12000 {
		t.Fatalf("unexpected format: rate=%d channels=%d len=%d", d.SampleRate(), d.Channels(), d.Len())
	}
	buf := make([]float32, 2048)
	n, err := d.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("Read = %d, %v", n, err)
	}
	if err := d.SetPosition(0); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Read(buf); err != nil {
		t.Fatal(err)
	}
}
