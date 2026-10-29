//go:build windows

package capture

/*
#cgo LDFLAGS: -ld3d11 -ldxgi -lruntimeobject -lole32 -luuid -ldwmapi
#include <stdlib.h>
#include "wgc.h"

extern void goWGCFrame(uintptr_t handle, void *data, int width, int height, int stride, long long pts);
extern void goWGCList(uintptr_t handle, char *id, char *kind, char *name, int width, int height);

*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

// windowsBackend captures through Windows Graphics Capture: displays as whole
// monitors and windows as themselves, so a covered or resized window is still
// the window, not a crop of whatever happened to cover it.
type windowsBackend struct {
	once sync.Once

	mu     sync.Mutex
	opened bool
}

func newBackend() Backend { return &windowsBackend{} }

func (b *windowsBackend) Sources(ctx context.Context) ([]Source, error) {
	var sources []Source
	handle := cgo.NewHandle(func(s Source) { sources = append(sources, s) })
	defer handle.Delete()

	errBuf := (*C.char)(C.malloc(256))
	defer C.free(unsafe.Pointer(errBuf))
	// The Go exports are declared above, in this same preamble, so their
	// addresses are the callbacks native code gets.
	if rc := C.ottr_wgc_list((C.ottr_source_cb)(C.goWGCList), C.uintptr_t(handle),
		errBuf, 256); rc != 0 {
		return nil, fmt.Errorf("%w: %s", ErrSourceGone, C.GoString(errBuf))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("%w: no display is attached", ErrSourceGone)
	}
	return sources, nil
}

func (b *windowsBackend) Open(ctx context.Context, cfg StreamConfig) (Stream, error) {
	id := C.CString(cfg.SourceID)
	defer C.free(unsafe.Pointer(id))

	st := &wgcStream{queue: newFrameQueue(), sourceID: cfg.SourceID}
	st.handle = cgo.NewHandle(st)

	// The bridge reports the size it opened and why it failed, if it did.
	var width, height, code C.int
	errBuf := (*C.char)(C.malloc(256))
	defer C.free(unsafe.Pointer(errBuf))
	stream := C.ottr_wgc_open(id, boolInt(cfg.Cursor), (C.ottr_frame_cb)(C.goWGCFrame),
		C.uintptr_t(st.handle), &width, &height, &code, errBuf, 256)
	if stream == nil {
		st.handle.Delete()
		return nil, classifyWithCode(C.GoString(errBuf), int(code))
	}
	st.stream = stream
	st.startWatch()
	return st, nil
}

func (b *windowsBackend) Snapshot(ctx context.Context, cfg SnapshotConfig) (Frame, error) {
	stream, err := b.Open(ctx, StreamConfig{SourceID: cfg.SourceID, Cursor: cfg.Cursor})
	if err != nil {
		return Frame{}, err
	}
	defer stream.Close()

	// The first frame of a fresh stream is the current desktop: WGC has no
	// one-shot read, and a stream that never delivers is a failed capture.
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	frame, err := stream.Next(cctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return Frame{}, fmt.Errorf("%w: %s", ErrNoFrame, cfg.SourceID)
	}
	return frame, err
}

func (b *windowsBackend) Close() error { return nil }

// wgcStream is one open WGC stream. The native callback only enqueues.
type wgcStream struct {
	queue    *frameQueue
	sourceID string
	handle   cgo.Handle

	mu     sync.Mutex
	stream unsafe.Pointer
	width  int
	height int
	closed bool

	stopWatch chan struct{}
}

func (s *wgcStream) Next(ctx context.Context) (Frame, error) { return s.queue.Next(ctx) }

func (s *wgcStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	stream := s.stream
	s.mu.Unlock()

	close(s.stopWatch)
	// Closing the native stream waits for a callback already in flight, so
	// the handle can go only after it returns.
	C.ottr_wgc_close(stream)
	s.handle.Delete()
	s.queue.close(nil)
	return nil
}

// startWatch ends the stream when its source disappears: a closed window never
// signals that on its own, and the recorder must not hang on a dead source.
func (s *wgcStream) startWatch() {
	s.stopWatch = make(chan struct{})
	source := C.CString(s.sourceID)
	go func() {
		defer C.free(unsafe.Pointer(source))
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopWatch:
				return
			case <-ticker.C:
				if C.ottr_wgc_alive(source) == 0 {
					s.queue.close(fmt.Errorf("%w: %s", ErrSourceGone, s.sourceID))
					return
				}
			}
		}
	}()
}

//export goWGCFrame
func goWGCFrame(handle C.uintptr_t, data unsafe.Pointer, width, height, stride C.int, pts C.longlong) {
	st, ok := cgo.Handle(handle).Value().(*wgcStream)
	if !ok {
		return
	}
	w, h, row := int(width), int(height), int(stride)
	if w <= 0 || h <= 0 {
		return
	}
	bgra := C.GoBytes(data, C.int(row*h))
	st.queue.push(Frame{
		Data:   tightCopy(bgra, w, h, row),
		Width:  w,
		Height: h,
		PTS:    time.Duration(pts),
	})
}

//export goWGCList
func goWGCList(handle C.uintptr_t, id, kind, name *C.char, width, height C.int) {
	fn, ok := cgo.Handle(handle).Value().(func(Source))
	if !ok {
		return
	}
	src := Source{ID: C.GoString(id), Kind: C.GoString(kind), Name: C.GoString(name)}
	if width > 0 && height > 0 {
		src.Detail = fmt.Sprintf("%dx%d", int(width), int(height))
	}
	fn(src)
}

// classifyWithCode turns the bridge's error code into the shared error kind.
func classifyWithCode(msg string, code int) error {
	err := errors.New(msg)
	switch code {
	case 1:
		return fmt.Errorf("%w: %s", ErrPermission, msg)
	case 2:
		return fmt.Errorf("%w: %s", ErrSourceGone, msg)
	default:
		return err
	}
}

func boolInt(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
