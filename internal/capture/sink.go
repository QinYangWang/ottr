package capture

import (
	"io"
	"sync"
	"sync/atomic"
)

// LiveWriter receives the recorded audio as it comes, interleaved f32le, at
// the rate and channel count it was opened for.
type LiveWriter interface {
	io.WriteCloser
}

// LiveStarter opens the live sink of a recording, or returns nil when there
// is none: no model installed, or live text turned off.
type LiveStarter func(rate, channels int) (LiveWriter, error)

// liveFeed hands the capture's PCM to the live sink without blocking the
// capture goroutine, opening the sink with the first packet's format: one
// buffer is offered at a time, and buffers written while the sink still
// holds the last one are dropped.
//
// shortcut: only the first audio stream is transcribed, so a recording of
// system audio and the microphone gets live text from one of them; open one
// sink per stream, and mix them, when that matters.
type liveFeed struct {
	starter LiveStarter

	mu   sync.Mutex
	w    LiveWriter
	rate int
	ch   int
	stop bool

	busy atomic.Bool
	once sync.Once
}

func newLiveFeed(starter LiveStarter) *liveFeed {
	return &liveFeed{starter: starter}
}

// write offers one buffer of interleaved f32le samples.
func (f *liveFeed) write(pcm []byte, rate, channels int) {
	if f == nil || f.starter == nil || len(pcm) == 0 {
		return
	}
	f.mu.Lock()
	if f.stop {
		f.mu.Unlock()
		return
	}
	if f.w == nil {
		w, err := f.starter(rate, channels)
		if err != nil || w == nil {
			f.stop = true
			f.mu.Unlock()
			return
		}
		f.w, f.rate, f.ch = w, rate, channels
	}
	if rate != f.rate || channels != f.ch {
		f.mu.Unlock()
		return // another stream's format: not transcribed
	}
	w := f.w
	f.mu.Unlock()
	if !f.busy.CompareAndSwap(false, true) {
		return // the sink is behind: drop this buffer
	}
	buf := append([]byte(nil), pcm...)
	go func() {
		defer f.busy.Store(false)
		w.Write(buf)
	}()
}

// close closes the sink once, so that it decodes what it has left.
func (f *liveFeed) close() {
	if f == nil {
		return
	}
	f.mu.Lock()
	w := f.w
	f.stop = true
	f.mu.Unlock()
	if w != nil {
		f.once.Do(func() { w.Close() })
	}
}
