//go:build windows

package capture

import (
	"os"
	"testing"
	"time"
)

// The previewer really grabs the screen, and the frames it writes are JPEGs.
// It needs FFmpeg and a desktop, so it is off unless OTTR_LIVE=1.
func TestPreviewerShowsTheScreen(t *testing.T) {
	if os.Getenv("OTTR_LIVE") == "" {
		t.Skip("set OTTR_LIVE=1 to grab the real screen")
	}
	p := NewPreviewer()
	defer p.Stop()
	p.Show("screen")

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if frame := p.Frame(); len(frame) > 2 {
			if frame[0] != 0xFF || frame[1] != 0xD8 {
				t.Fatalf("a frame that is not a JPEG: % x", frame[:4])
			}
			t.Logf("got a frame of %d bytes", len(frame))
			return
		}
		if err := p.Err(); err != nil {
			t.Fatalf("no preview: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no frame in 15 seconds")
}
