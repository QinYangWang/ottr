package capture

import (
	"bytes"
	"net"
	"sync"
)

// previewHub reads the MJPEG stream ffmpeg writes over TCP and keeps only the
// latest JPEG frame, which the window shows: a consumer slower than the
// recording never grows memory.
type previewHub struct {
	ln net.Listener

	mu     sync.Mutex
	frame  []byte
	closed bool
}

func newPreviewHub() (*previewHub, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	h := &previewHub{ln: ln}
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
}

func (h *previewHub) Close() {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	h.ln.Close()
}
