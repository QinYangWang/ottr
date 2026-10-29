package capture

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

// previewFPS is how often the preview refreshes its picture, and how wide it
// is: five frames a second of a 960-pixel-wide JPEG is all a window shows.
const (
	previewFPS     = 5
	previewWidth   = 960
	previewQuality = 78
)

// preview keeps the newest raw frame of a capture and encodes a JPEG from it at
// a steady rate, so that capture, the window and the encoder never wait for
// each other: a source that changes faster than the preview only makes the
// preview skip frames.
type preview struct {
	mu     sync.Mutex
	raw    Frame // the newest frame, nil before the first
	jpeg   []byte
	err    error
	closed bool
	stop   chan struct{}
	fresh  chan struct{}      // one wake-up for the encoder, coalesced
	cancel context.CancelFunc // ends the stream feeding it, when there is one
}

func newPreview() *preview {
	p := &preview{stop: make(chan struct{}), fresh: make(chan struct{}, 1)}
	go p.encodeLoop()
	return p
}

// offer takes a frame and wakes the encoder; a frame it replaces is dropped.
func (p *preview) offer(f Frame) {
	p.mu.Lock()
	if !p.closed {
		p.raw = f
	}
	p.mu.Unlock()
	select {
	case p.fresh <- struct{}{}:
	default:
	}
}

// encodeLoop is the only encoder of a preview. The first frame is encoded at
// once and the rest at most previewFPS times a second, so a window opens on a
// picture instead of waiting for the next tick and a busy source costs what a
// quiet one does.
func (p *preview) encodeLoop() {
	interval := time.Second / previewFPS
	var last time.Time
	for {
		select {
		case <-p.stop:
			return
		case <-p.fresh:
		}
		if wait := interval - time.Since(last); wait > 0 {
			select {
			case <-p.stop:
				return
			case <-time.After(wait):
			}
		}
		p.mu.Lock()
		f := p.raw
		p.raw = Frame{}
		p.mu.Unlock()
		if f.Data == nil {
			continue
		}
		last = time.Now()
		encoded, err := encodePreview(f)
		if err != nil {
			continue
		}
		p.mu.Lock()
		if !p.closed {
			p.jpeg = encoded
		}
		p.mu.Unlock()
	}
}

// Frame returns the newest JPEG, or nil before the first one.
func (p *preview) Frame() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.jpeg
}

func (p *preview) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel() // closing the stream is what lets its goroutine return
	}
	close(p.stop)
}

// own makes the preview cancel ctx and its stream when it closes.
func (p *preview) own(cancel context.CancelFunc) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		cancel()
		return
	}
	p.cancel = cancel
	p.mu.Unlock()
}

// previewer holds the stream behind a preview, and swaps it when the source
// changes. It is the idle picture, shown before a recording starts.
type Previewer struct {
	backend Backend

	mu      sync.Mutex
	source  string
	cancel  context.CancelFunc
	preview *preview
	err     error
}

// NewPreviewer creates a previewer over one capture backend. Nothing runs
// until Show.
func NewPreviewer(backend Backend) *Previewer {
	return &Previewer{backend: backend}
}

// Show draws the source given from now on, in the background: the caller gets
// the picture from Frame once a frame arrives.
func (p *Previewer) Show(sourceID string) {
	p.mu.Lock()
	if p.source == sourceID && p.cancel != nil {
		p.mu.Unlock()
		return
	}
	p.stopLocked()
	p.source = sourceID
	ctx, cancel := context.WithCancel(context.Background())
	pv := newPreview()
	p.cancel, p.preview, p.err = cancel, pv, nil
	p.mu.Unlock()

	go p.run(ctx, sourceID, pv)
}

func (p *Previewer) run(ctx context.Context, sourceID string, pv *preview) {
	defer pv.close()
	stream, err := p.backend.Open(ctx, StreamConfig{SourceID: sourceID, FPS: previewFPS, Cursor: false})
	if err != nil {
		p.fail(sourceID, err)
		return
	}
	defer stream.Close()
	for {
		f, err := stream.Next(ctx)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
				p.fail(sourceID, err)
			}
			return
		}
		pv.offer(f)
	}
}

func (p *Previewer) fail(sourceID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// A newer Show may have won while this one failed: keep only the newest.
	if p.source != sourceID {
		return
	}
	p.err = err
}

// Frame returns the latest JPEG frame, or nil before the first one.
func (p *Previewer) Frame() []byte {
	p.mu.Lock()
	pv := p.preview
	p.mu.Unlock()
	if pv == nil {
		return nil
	}
	return pv.Frame()
}

// Pending reports whether the preview is still starting: the window asks for
// frames until the first one lands.
func (p *Previewer) Pending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancel != nil && p.err == nil
}

// Err reports why no picture comes, when it is not simply "not yet".
func (p *Previewer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Stop ends the preview.
func (p *Previewer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Previewer) stopLocked() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.preview, p.source, p.err = nil, nil, "", nil
}

// encodePreview scales a frame to the preview's width and writes it as JPEG. A
// preview is a picture of the whole desktop, so it is scaled before encoding,
// not after.
func encodePreview(f Frame) ([]byte, error) {
	img := scaleFrame(f, previewWidth)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: previewQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// scaleFrame converts BGRA to RGBA and scales it to at most maxWidth, averaging
// the pixels each destination pixel covers so that text stays readable. It
// never enlarges: a small source is copied as it is.
func scaleFrame(f Frame, maxWidth int) image.Image {
	src := rgbaOf(f)
	if f.Width <= maxWidth {
		return src
	}
	height := f.Height * maxWidth / f.Width
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
