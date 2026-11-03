package native

import (
	"os/exec"
	"path/filepath"
	"testing"

	"ottr/internal/tools"
)

// testVideo makes a tiny H.264/AAC clip without relying on repository fixtures.
func testVideo(t *testing.T) string {
	t.Helper()
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		t.Skip(err)
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	cmd := exec.Command(ffmpeg, "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "0.5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot make an H.264/AAC clip: %v (%s)", err, out)
	}
	return path
}
