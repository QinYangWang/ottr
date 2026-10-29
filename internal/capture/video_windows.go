//go:build windows

package capture

import (
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
)

// recordFPS is the rate a recording is written at. Native capture delivers a
// frame only when the screen changes, so the sink repeats the newest frame to
// keep the encoder's input fixed-rate: a still screen still records, and audio
// stays on the same clock.
const recordFPS = 30

var videoSeq atomic.Int64

// videoInput is a named pipe carrying raw BGRA frames of one fixed size into
// the encoder. The size is fixed because the encoder's input cannot change
// mid-file: a source that resizes is fitted into it.
//
// shortcut: frames are paced by a ticker, not by their native timestamps; the
// sample-accurate path is the native muxer of docs/native-capture.md.
type videoInput struct {
	name string

	mu     sync.Mutex
	latest []byte
	fresh  bool
	conn   net.Conn
	closed bool

	stop func()
}

// newVideoInput starts a raw BGRA pipe of width×height at recordFPS and returns
// its ffmpeg input arguments.
func newVideoInput(width, height int) (*videoInput, error) {
	name := fmt.Sprintf(`\\.\pipe\ottr-video-%d-%d`, os.Getpid(), videoSeq.Add(1))
	ln, err := winio.ListenPipe(name, nil)
	if err != nil {
		return nil, err
	}
	v := &videoInput{name: name}
	var once sync.Once
	v.stop = func() {
		once.Do(func() {
			ln.Close()
			v.mu.Lock()
			v.closed = true
			if v.conn != nil {
				v.conn.Close() // ends the pacer, and the encoder's input
			}
			v.mu.Unlock()
		})
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		v.mu.Lock()
		if v.closed {
			v.mu.Unlock()
			return
		}
		v.conn = conn
		v.mu.Unlock()

		ticker := time.NewTicker(time.Second / recordFPS)
		defer ticker.Stop()
		var last []byte
		for range ticker.C {
			v.mu.Lock()
			if v.closed {
				v.mu.Unlock()
				return
			}
			frame, fresh := v.latest, v.fresh
			v.fresh = false
			v.mu.Unlock()
			if fresh {
				last = frame
			}
			if last == nil {
				continue // nothing captured yet: the clock starts with frame one
			}
			if _, err := conn.Write(last); err != nil {
				return // the encoder went away
			}
		}
	}()
	return v, nil
}

// args is the encoder input the pipe is read from.
func (v *videoInput) args(width, height int) []string {
	return []string{
		"-thread_queue_size", "8",
		"-f", "rawvideo", "-pixel_format", "bgra",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-framerate", fmt.Sprint(recordFPS),
		"-i", v.name,
	}
}

// offer hands the newest frame to the pacer, which drops what it replaced.
func (v *videoInput) offer(frame []byte) {
	v.mu.Lock()
	v.latest, v.fresh = frame, true
	v.mu.Unlock()
}

func (v *videoInput) close() { v.stop() }
