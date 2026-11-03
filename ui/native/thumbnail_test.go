package native

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/library"
)

func TestVideoAndScreenshotThumbnails(t *testing.T) {
	video := testVideo(t)
	a, _ := newTestApp(t)
	m, err := a.Lib.AddNativeRecording(library.Record{Path: video, Width: 640, Height: 360, HasVideo: true})
	if err != nil {
		t.Fatal(err)
	}
	id := m.ID
	a.reload()
	if _, err := decodeVideoPoster(m); err != nil {
		t.Fatalf("decode poster: %v", err)
	}
	a.ensureThumbnails()
	tt := ui.NewTester(a.view, WideWidth, WideHeight)
	for i := 0; i < 100 && a.thumbs[id] == nil; i++ {
		time.Sleep(20 * time.Millisecond)
		tt.Frame()
	}
	if a.thumbs[id] == nil {
		t.Fatalf("video did not produce a poster thumbnail: thumbs=%v loading=%v queue=%d", a.thumbs, a.thumbLoading, len(a.queue))
	}

	picture := image.NewRGBA(image.Rect(0, 0, 64, 32))
	path := filepath.Join(t.TempDir(), "shot.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, picture); err != nil {
		t.Fatal(err)
	}
	f.Close()
	m.Source, m.Path = "screenshot", path
	a.media[0] = m
	delete(a.thumbs, id)
	a.ensureThumbnails()
	for i := 0; i < 50 && a.thumbs[id] == nil; i++ {
		time.Sleep(20 * time.Millisecond)
		tt.Frame()
	}
	if a.thumbs[id] == nil {
		t.Fatal("screenshot did not produce its image thumbnail")
	}
}

func TestLibraryLayoutSwitch(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.view, WideWidth, WideHeight)
	tt.Frame()
	if err := tt.Click("Grid view"); err != nil {
		t.Fatal(err)
	}
	if !a.libraryGridView {
		t.Fatal("grid view did not activate")
	}
	if err := tt.Click("List view"); err != nil {
		t.Fatal(err)
	}
	if a.libraryGridView {
		t.Fatal("list view did not activate")
	}
}
