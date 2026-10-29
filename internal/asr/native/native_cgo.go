//go:build cgo

// Package native is the cgo wrapper around the sherpa-onnx C API. The library
// is loaded at run time by native/sherpa_shim.c, so Ottr starts without it;
// nothing above internal/asr sees a C type, and every native object is created
// and destroyed here.
//
// Both batch and live audio go through a streaming recognizer: a streaming
// zipformer model is exactly that, and one code path means the batch result and
// the live text cannot disagree about models or configuration.
package native

/*
#cgo CFLAGS: -O2
#include <stdlib.h>
#include "sherpa_shim.h"
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Available reports whether this build can use native ASR: false in a
// CGO_ENABLED=0 build, where everything below returns ErrUnavailable.
var Available = true

// ErrUnavailable is what a build without cgo answers to any call below.
var ErrUnavailable = errors.New("native ASR is not available in this build")

// ErrNoRecognizer is what a create call returns when sherpa-onnx refused the
// configuration, which in practice means the model files are unusable.
var ErrNoRecognizer = errors.New("sherpa-onnx refused this model configuration")

var (
	loadMu   sync.Mutex
	loaded   bool
	loadErr  error
	loadPath string
	version  string
)

// Load loads the sherpa-onnx library at path once per process and returns its
// version. A failure is remembered for that path: Go cannot unload a library,
// so retrying the same path could only repeat the answer.
func Load(path string) (string, error) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loaded {
		return version, nil
	}
	if loadErr != nil && path == loadPath {
		return "", loadErr
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	if C.sherpa_api_load(cPath) == nil {
		loadErr = errors.New(C.GoString(C.sherpa_api_error()))
		loadPath = path
		return "", loadErr
	}
	loaded, loadErr = true, nil
	version = C.GoString(C.sherpa_api_version())
	return version, nil
}

// Version returns the version of the loaded library, or "" when none is.
func Version() string {
	loadMu.Lock()
	defer loadMu.Unlock()
	return version
}

// withCString runs fn with a C copy of s, freed afterwards.
func withCString(s string, fn func(*C.char)) {
	p := C.CString(s)
	defer C.free(unsafe.Pointer(p))
	fn(p)
}

// Recognizer owns one streaming recognizer. It and its streams are owned by a
// single goroutine: nothing here is safe for concurrent use.
type Recognizer struct {
	h unsafe.Pointer
}

// NewRecognizer builds a streaming transducer recognizer. With endpoint on,
// sherpa-onnx ends an utterance after silence1 seconds of trailing silence, or
// after maxUtterance seconds of speech without a pause.
func NewRecognizer(tokens, encoder, decoder, joiner string, threads int, endpoint bool, silence1, maxUtterance float32) (*Recognizer, error) {
	if !loaded {
		return nil, ErrUnavailable
	}
	var h unsafe.Pointer
	// The C side reads these strings only while it builds the recognizer.
	withCString(tokens, func(ct *C.char) {
		withCString(encoder, func(ce *C.char) {
			withCString(decoder, func(cd *C.char) {
				withCString(joiner, func(cj *C.char) {
					on := C.int32_t(0)
					if endpoint {
						on = 1
					}
					// The second rule (silence with no speech yet) is half the first.
					h = C.sherpa_online_create(ct, ce, cd, cj, C.int32_t(threads), on,
						C.float(silence1), C.float(silence1/2), C.float(maxUtterance))
				})
			})
		})
	})
	if h == nil {
		return nil, ErrNoRecognizer
	}
	return &Recognizer{h: h}, nil
}

// Stream opens one decoding stream on the recognizer.
func (r *Recognizer) Stream() *Stream {
	return &Stream{r: r, h: C.sherpa_online_stream_create(r.h)}
}

// Destroy frees the recognizer. Its streams must be destroyed first.
func (r *Recognizer) Destroy() {
	if r != nil && r.h != nil {
		C.sherpa_online_destroy(r.h)
		r.h = nil
	}
}

// Stream is one decoding stream inside a recognizer.
type Stream struct {
	r *Recognizer
	h unsafe.Pointer
}

// Accept hands mono float samples at rate to the stream. sherpa-onnx copies
// what it keeps, so the slice need not outlive the call.
func (s *Stream) Accept(rate int, samples []float32) {
	if s == nil || s.h == nil || len(samples) == 0 {
		return
	}
	C.sherpa_online_stream_accept(s.h, C.int32_t(rate),
		(*C.float)(unsafe.Pointer(&samples[0])), C.int32_t(len(samples)))
}

// Decode runs every decode step the stream has frames for.
func (s *Stream) Decode() {
	for C.sherpa_online_stream_ready(s.r.h, s.h) != 0 {
		C.sherpa_online_stream_decode(s.r.h, s.h)
	}
}

// Endpoint reports whether the recognizer found the end of an utterance.
func (s *Stream) Endpoint() bool { return C.sherpa_online_stream_endpoint(s.r.h, s.h) != 0 }

// Reset clears the decoder state after an endpoint.
func (s *Stream) Reset() { C.sherpa_online_stream_reset(s.r.h, s.h) }

// Finish says that no more samples will come.
func (s *Stream) Finish() { C.sherpa_online_stream_finish(s.h) }

// Result copies the stream's current result into Go: its text, and its tokens
// with timestamps in seconds. Timestamps are nil when the model gives none.
func (s *Stream) Result() (text string, tokens []string, timestamps []float64) {
	buf := make([]byte, 256)
	for {
		var arr **C.char
		var stamps *C.float
		var count C.int32_t
		need := C.sherpa_online_result(s.r.h, s.h, (*C.char)(unsafe.Pointer(&buf[0])),
			C.int32_t(len(buf)), &arr, &stamps, &count)
		if int(need) >= len(buf) {
			buf = make([]byte, need+1) // the text outgrew the buffer
			continue
		}
		text = string(buf[:need])
		if arr != nil && count > 0 {
			defer C.sherpa_free(unsafe.Pointer(arr))
			tokens = make([]string, count)
			timestamps = make([]float64, count)
			strs := unsafe.Slice(arr, int(count))
			for i := range tokens {
				tokens[i] = C.GoString(strs[i])
			}
			if stamps != nil {
				defer C.sherpa_free(unsafe.Pointer(stamps))
				ts := unsafe.Slice(stamps, int(count))
				for i := range timestamps {
					timestamps[i] = float64(ts[i])
				}
			}
		}
		return text, tokens, timestamps
	}
}

// Destroy frees the stream.
func (s *Stream) Destroy() {
	if s != nil && s.h != nil {
		C.sherpa_online_stream_destroy(s.h)
		s.h = nil
	}
}
