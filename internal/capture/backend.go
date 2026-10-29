package capture

import (
	"context"
	"errors"
	"image"
	"io"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

// Source is a recordable screen, display or window. ID is the platform's own
// identity — a monitor or window handle — never a title or an index, so that
// the source a user chose is the one captured.
type Source struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // screen | display | window
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"` // size, when the platform knows it
}

// Frame is one captured image: tightly packed BGRA8888, physical pixels, and
// the platform's monotonic timestamp.
type Frame struct {
	Data          []byte
	Width, Height int
	PTS           time.Duration
}

// StreamConfig asks for a live stream of one source.
type StreamConfig struct {
	SourceID string
	FPS      int  // the rate the caller wants; the platform may deliver on change
	Cursor   bool // draw the pointer into the frame
}

// SnapshotConfig asks for one frame of one source.
type SnapshotConfig struct {
	SourceID string
	Cursor   bool
}

// Stream is a live capture. Next returns io.EOF on a normal end.
type Stream interface {
	Next(context.Context) (Frame, error)
	Close() error // idempotent; no callback runs once it returns
}

// Backend is the platform's capture API: what can be captured, live streams,
// and one-shot frames. One instance serves every product of the process.
type Backend interface {
	Sources(context.Context) ([]Source, error)
	Open(context.Context, StreamConfig) (Stream, error)
	Snapshot(context.Context, SnapshotConfig) (Frame, error)
	Close() error
}

// The ways capture fails, for the window to explain. Errors keep their cause
// and wrap one of these.
var (
	ErrPermission  = errors.New("screen recording is not permitted")
	ErrCancelled   = errors.New("the source selection was cancelled")
	ErrSourceGone  = errors.New("the source is gone")
	ErrUnsupported = errors.New("this system cannot be captured")
	ErrEncoder     = errors.New("no encoder is available")
	ErrNoFrame     = errors.New("the source sent no frame")
)

// queueFrames is what one stream hands its consumer: at most two frames, the
// oldest dropped when the consumer is behind, so a slow reader never grows
// memory and never blocks the native callback thread.
const queueFrames = 2

// frameQueue is the bounded handoff between the platform's callback thread and
// the consumer. The newest frames win.
type frameQueue struct {
	mu     sync.Mutex
	frames []Frame
	err    error
	notify chan struct{} // one wake-up, coalesced
	done   chan struct{}
	once   sync.Once
}

func newFrameQueue() *frameQueue {
	return &frameQueue{notify: make(chan struct{}, 1), done: make(chan struct{})}
}

// push offers a frame and never waits.
func (q *frameQueue) push(f Frame) {
	q.mu.Lock()
	if q.err != nil {
		q.mu.Unlock()
		return
	}
	if len(q.frames) == queueFrames {
		q.frames = q.frames[1:]
	}
	q.frames = append(q.frames, f)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Next returns the oldest queued frame, waiting for one while none is there.
func (q *frameQueue) Next(ctx context.Context) (Frame, error) {
	for {
		q.mu.Lock()
		if len(q.frames) > 0 {
			f := q.frames[0]
			q.frames = q.frames[1:]
			q.mu.Unlock()
			return f, nil
		}
		err := q.err
		q.mu.Unlock()
		if err != nil {
			return Frame{}, err
		}
		select {
		case <-ctx.Done():
			return Frame{}, ctx.Err()
		case <-q.notify:
		case <-q.done:
		}
	}
}

// close ends the queue with err, io.EOF for a normal end.
func (q *frameQueue) close(err error) {
	if err == nil {
		err = io.EOF
	}
	q.once.Do(func() {
		q.mu.Lock()
		q.err = err
		q.mu.Unlock()
		close(q.done)
	})
}

// rgbaOf turns a tightly packed BGRA frame into the RGBA image Go draws and
// encodes. Alpha stays as the source delivered it.
func rgbaOf(f Frame) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		src := f.Data[y*f.Width*4:]
		dst := img.Pix[y*img.Stride:]
		for x := 0; x < f.Width; x++ {
			dst[4*x+0] = src[4*x+2]
			dst[4*x+1] = src[4*x+1]
			dst[4*x+2] = src[4*x+0]
			dst[4*x+3] = src[4*x+3]
		}
	}
	return img
}

// tightCopy removes the row padding a platform's stride may add, so that every
// Frame is tightly packed whatever the source delivered.
func tightCopy(bgra []byte, width, height, stride int) []byte {
	row := width * 4
	if stride == row {
		out := make([]byte, row*height)
		copy(out, bgra[:row*height])
		return out
	}
	out := make([]byte, row*height)
	for y := 0; y < height; y++ {
		copy(out[y*row:(y+1)*row], bgra[y*stride:y*stride+row])
	}
	return out
}

// evenSize is width and height as H.264 accepts them: the canvas the encoder
// gets never has an odd side, and a resized source is fitted into it rather
// than changing the codec's configuration mid-file.
func evenSize(width, height int) (int, int) {
	if width < 2 || height < 2 {
		return 2, 2
	}
	return width &^ 1, height &^ 1
}

// fitFrame is the frame in a canvas of width×height: copied when it already
// matches, else scaled to fit and letterboxed, so that its aspect never
// changes.
func fitFrame(f Frame, width, height int) []byte {
	if f.Width == width && f.Height == height {
		return f.Data
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	src := rgbaOf(f)
	w, h := width, height
	if f.Width*height > width*f.Height {
		h = f.Height * width / f.Width
	} else {
		w = f.Width * height / f.Height
	}
	dst := image.Rect((width-w)/2, (height-h)/2, (width+w)/2, (height+h)/2)
	draw.ApproxBiLinear.Scale(canvas, dst, src, src.Bounds(), draw.Src, nil)

	out := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		row := canvas.Pix[y*canvas.Stride:]
		for x := 0; x < width; x++ {
			out[4*(y*width+x)+0] = row[4*x+2]
			out[4*(y*width+x)+1] = row[4*x+1]
			out[4*(y*width+x)+2] = row[4*x+0]
			out[4*(y*width+x)+3] = 0xff
		}
	}
	return out
}

// NewBackend returns the backend of the platform this binary runs on.
func NewBackend() Backend { return newBackend() }
