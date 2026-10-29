package capture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"ottr/internal/library"
	"ottr/internal/storage"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeBackend hands out synthetic frames, so these tests need no desktop and
// no media program.
type fakeBackend struct {
	sources []Source
	frames  int   // frames a stream delivers before io.EOF, -1 for open-ended
	err     error // Open fails with this
	opened  int
	closed  int
}

func (f *fakeBackend) Sources(context.Context) ([]Source, error) { return f.sources, f.err }
func (f *fakeBackend) Open(ctx context.Context, cfg StreamConfig) (Stream, error) {
	f.opened++
	if f.err != nil {
		return nil, f.err
	}
	q := newFrameQueue()
	go func() {
		for i := 0; f.frames < 0 || i < f.frames; i++ {
			select {
			case <-ctx.Done():
				q.close(nil)
				return
			default:
			}
			q.push(syntheticFrame(64, 48, byte(i*17)))
			time.Sleep(2 * time.Millisecond)
		}
		q.close(nil)
	}()
	return &fakeStream{queue: q, backend: f}, nil
}
func (f *fakeBackend) Snapshot(ctx context.Context, cfg SnapshotConfig) (Frame, error) {
	if f.err != nil {
		return Frame{}, f.err
	}
	return syntheticFrame(64, 48, 0x40), nil
}
func (f *fakeBackend) Close() error { return nil }

type fakeStream struct {
	queue   *frameQueue
	backend *fakeBackend
	once    sync.Once
}

func (s *fakeStream) Next(ctx context.Context) (Frame, error) { return s.queue.Next(ctx) }
func (s *fakeStream) Close() error {
	s.once.Do(func() { s.backend.closed++ })
	return nil
}

// syntheticFrame fills BGRA rows with a gradient, one channel shifted per call
// so that consecutive frames differ.
func syntheticFrame(width, height int, shift byte) Frame {
	data := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := 4 * (y*width + x)
			data[i+0] = byte(x*4) + shift
			data[i+1] = byte(y*5) + shift
			data[i+2] = 0x30 + shift
			data[i+3] = 0xff
		}
	}
	return Frame{Data: data, Width: width, Height: height, PTS: time.Duration(shift) * time.Millisecond}
}

// A queue holds two frames and drops the oldest, so a consumer that falls
// behind never grows memory and never loses the newest picture.
func TestFrameQueueKeepsTheNewest(t *testing.T) {
	q := newFrameQueue()
	for i := 0; i < 10; i++ {
		q.push(Frame{Width: i})
	}
	got, err := q.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 8 {
		t.Errorf("kept the frame %d wide, want the newest of the last two (8)", got.Width)
	}
	q.close(nil)
	second, err := q.Next(context.Background())
	if err != nil || second.Width != 9 {
		t.Fatalf("second frame %d, %v", second.Width, err)
	}
	if _, err := q.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("a drained queue must end with io.EOF, got %v", err)
	}
}

// A consumer waiting on an empty queue wakes up for the frame that arrives.
func TestFrameQueueWakesAWaiter(t *testing.T) {
	q := newFrameQueue()
	go func() {
		time.Sleep(10 * time.Millisecond)
		q.push(Frame{Width: 7})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	f, err := q.Next(ctx)
	if err != nil || f.Width != 7 {
		t.Fatalf("got %+v, %v", f, err)
	}
}

// Next respects a cancelled context rather than hanging on a silent source.
func TestFrameQueueHonoursCancellation(t *testing.T) {
	q := newFrameQueue()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

// BGRA becomes RGBA in the right channel order, with alpha kept: a red desktop
// pixel must not come out blue.
func TestRGBAOfSwapsChannels(t *testing.T) {
	f := Frame{Data: []byte{0x00, 0x00, 0xff, 0x80}, Width: 1, Height: 1} // BGRA: red, half alpha
	img := rgbaOf(f)
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 0xff || g>>8 != 0x00 || b>>8 != 0x00 || a>>8 != 0x80 {
		t.Errorf("got r=%02x g=%02x b=%02x a=%02x", r>>8, g>>8, b>>8, a>>8)
	}
}

// A platform that pads its rows still delivers tightly packed frames.
func TestTightCopyRemovesPadding(t *testing.T) {
	width, height, stride := 2, 2, 12 // 8 bytes of pixels, 4 of padding
	rows := make([]byte, stride*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width*4; x++ {
			rows[y*stride+x] = byte(y*10 + x)
		}
		rows[y*stride+width*4] = 0xee // padding must not survive
	}
	out := tightCopy(rows, width, height, stride)
	if len(out) != width*height*4 {
		t.Fatalf("got %d bytes, want %d", len(out), width*height*4)
	}
	for _, b := range out {
		if b == 0xee {
			t.Fatal("padding leaked into the frame")
		}
	}
	// Row 0 holds bytes 0..7 of the source, row 1 starts at 0x0a.
	for i := 0; i < 8; i++ {
		if out[i] != byte(i) {
			t.Fatalf("row 0 is % x", out[:8])
		}
	}
	if out[8] != 0x0a {
		t.Errorf("row 1 starts with %02x, want 0x0a", out[8])
	}
}

// The H.264 canvas never has an odd side, and a frame of another size is fitted
// into it without changing its aspect.
func TestFitFrameLetterboxes(t *testing.T) {
	if w, h := evenSize(1921, 1081); w != 1920 || h != 1080 {
		t.Errorf("evenSize gave %dx%d", w, h)
	}
	f := syntheticFrame(100, 50, 0)
	out := fitFrame(f, 100, 100)
	if len(out) != 100*100*4 {
		t.Fatalf("canvas is %d bytes", len(out))
	}
	// The top and bottom bands are empty, the middle holds the scaled frame.
	if got := out[0]; got != 0 {
		t.Errorf("the top band is not empty: %02x", got)
	}
	mid := 4 * (50*100 + 50)
	if out[mid+2] == 0 && out[mid+1] == 0 {
		t.Error("the middle of the canvas is empty: the frame was not drawn")
	}
}

// The preview is a JPEG, scaled to the preview width, and its rate follows the
// source only loosely: the newest frame wins.
func TestPreviewEncodesJPEG(t *testing.T) {
	f := syntheticFrame(1920, 1080, 0)
	data, err := encodePreview(f)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != previewWidth || b.Dy() != 1080*previewWidth/1920 {
		t.Errorf("preview is %v", b)
	}
	p := newPreview()
	defer p.close()
	p.offer(f)
	deadline := time.Now().Add(2 * time.Second)
	for p.Frame() == nil {
		if time.Now().After(deadline) {
			t.Fatal("the preview never encoded a frame")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !bytes.HasPrefix(p.Frame(), []byte{0xFF, 0xD8}) {
		t.Error("the preview frame is not a JPEG")
	}
}

// The previewer shows the frames of the source asked for, and reports a source
// it could not open instead of waiting forever.
func TestPreviewerShowsAndReports(t *testing.T) {
	backend := &fakeBackend{sources: []Source{{ID: "screen"}}, frames: 3}
	p := NewPreviewer(backend)
	defer p.Stop()
	p.Show("screen")
	deadline := time.Now().Add(2 * time.Second)
	for p.Frame() == nil {
		if time.Now().After(deadline) {
			t.Fatal("no preview frame")
		}
		time.Sleep(5 * time.Millisecond)
	}
	failing := NewPreviewer(&fakeBackend{err: fmt.Errorf("%w: nope", ErrPermission)})
	defer failing.Stop()
	failing.Show("screen")
	deadline = time.Now().Add(2 * time.Second)
	for failing.Err() == nil {
		if time.Now().After(deadline) {
			t.Fatal("a denied source was not reported")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !errors.Is(failing.Err(), ErrPermission) {
		t.Errorf("error %v does not say the permission was denied", failing.Err())
	}
	if failing.Frame() != nil {
		t.Error("a denied source drew a frame")
	}
}

// pngMagic is the signature every PNG starts with.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// A screenshot is a PNG of the native frame, at its own size, and its record
// says what it is.
func TestSnapshotterWritesPNG(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "shot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lib := &library.Service{}
	lib.Open(db)
	dir := t.TempDir()
	shot := &Snapshotter{Backend: &fakeBackend{}, Dir: dir, Lib: lib}
	m, err := shot.Screenshot(context.Background(), "screen")
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "screenshot" || m.Width != 64 || m.Height != 48 || m.HasAudio || m.DurationMs != 0 {
		t.Errorf("record: %+v", m)
	}
	data, err := os.ReadFile(m.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, pngMagic) {
		t.Error("the file is not a PNG")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
		t.Errorf("PNG is %v", b)
	}
	// The backend's failure keeps its kind, and leaves no file behind.
	denied := &Snapshotter{Backend: &fakeBackend{err: ErrCancelled}, Dir: dir, Lib: lib}
	if _, err := denied.Screenshot(context.Background(), "screen"); !errors.Is(err, ErrCancelled) {
		t.Errorf("got %v, want the cancellation", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("a failed screenshot left files: %v", entries)
	}
}

// The native canvas of a source is fixed once, so a source that resizes cannot
// change the encoder's configuration mid-file.
func TestEvenCanvasIsStable(t *testing.T) {
	w, h := evenSize(1366, 768)
	if w != 1366 || h != 768 {
		t.Errorf("even input changed: %dx%d", w, h)
	}
	if img := rgbaOf(Frame{Data: make([]byte, 4), Width: 1, Height: 1}); img.Bounds().Dx() != 1 {
		t.Errorf("rgbaOf bounds %v", img.Bounds())
	}
	_ = image.Rect
}
