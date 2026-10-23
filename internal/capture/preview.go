package capture

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// previewHub relays the MJPEG stream ffmpeg writes over TCP to HTTP
// consumers, keeping only the latest JPEG frame so a slow consumer never
// grows memory.
type previewHub struct {
	ln net.Listener

	mu     sync.Mutex
	frame  []byte
	ch     chan struct{} // closed when a new frame arrives, then replaced
	closed bool
}

func newPreviewHub() (*previewHub, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	h := &previewHub{ln: ln, ch: make(chan struct{})}
	go h.run()
	return h, nil
}

func (h *previewHub) addr() string { return h.ln.Addr().String() }

func (h *previewHub) run() {
	conn, err := h.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	var buf []byte
	tmp := make([]byte, 64<<10)
	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf = h.extract(append(buf, tmp[:n]...))
		}
		if err != nil {
			return
		}
	}
}

// extract pulls complete JPEG frames (FFD8..FFD9) out of the MJPEG stream
// and returns the unconsumed remainder.
func (h *previewHub) extract(buf []byte) []byte {
	for {
		start := bytes.Index(buf, []byte{0xFF, 0xD8})
		if start < 0 {
			if len(buf) > 0 && buf[len(buf)-1] == 0xFF {
				return buf[len(buf)-1:]
			}
			return buf[:0]
		}
		end := bytes.Index(buf[start+2:], []byte{0xFF, 0xD9})
		if end < 0 {
			if len(buf) > 1<<20 { // no SOI..EOI within 1 MiB: garbage
				return buf[:0]
			}
			return buf[start:]
		}
		frame := buf[start : start+2+end+2]
		h.publish(append([]byte(nil), frame...))
		buf = buf[start+2+end+2:]
	}
}

func (h *previewHub) publish(f []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.frame = f
	close(h.ch)
	h.ch = make(chan struct{})
}

// ServeHTTP streams frames as multipart/x-mixed-replace until the request
// or the hub ends.
func (h *previewHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	fl, _ := w.(http.Flusher)
	for {
		h.mu.Lock()
		frame, ch, closed := h.frame, h.ch, h.closed
		h.mu.Unlock()
		if closed {
			return
		}
		if frame != nil {
			fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
			w.Write(frame)
			w.Write([]byte("\r\n"))
			if fl != nil {
				fl.Flush()
			}
		}
		select {
		case <-ch:
		case <-r.Context().Done():
			return
		}
	}
}

func (h *previewHub) Close() {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		close(h.ch)
	}
	h.mu.Unlock()
	h.ln.Close()
}
