//go:build !cgo

// Package native stands in for the cgo wrapper in a build without cgo: there
// is no native ASR, and every call says so rather than pretending.
package native

import "errors"

// Available reports whether this build can use native ASR.
var Available = false

// ErrUnavailable is what every call below returns in this build.
var ErrUnavailable = errors.New("native ASR is not available in this build")

// ErrNoRecognizer never happens here; it exists so callers compile.
var ErrNoRecognizer = errors.New("sherpa-onnx refused this model configuration")

// Load reports that this build has no native ASR.
func Load(path string) (string, error) { return "", ErrUnavailable }

// Version is always empty in this build.
func Version() string { return "" }

// Recognizer is a stand-in that cannot be created.
type Recognizer struct{}

// NewRecognizer reports that this build has no native ASR.
func NewRecognizer(tokens, encoder, decoder, joiner string, threads int, endpoint bool, silence1, maxUtterance float32) (*Recognizer, error) {
	return nil, ErrUnavailable
}

// Stream cannot be opened here.
func (r *Recognizer) Stream() *Stream { return nil }

// Destroy does nothing.
func (r *Recognizer) Destroy() {}

// Stream is a stand-in with no native state.
type Stream struct{}

// Accept does nothing in this build.
func (s *Stream) Accept(rate int, samples []float32) {}

// Decode does nothing in this build.
func (s *Stream) Decode() {}

// Endpoint is always false in this build.
func (s *Stream) Endpoint() bool { return false }

// Reset does nothing in this build.
func (s *Stream) Reset() {}

// Finish does nothing in this build.
func (s *Stream) Finish() {}

// Result is always empty in this build.
func (s *Stream) Result() (string, []string, []float64) { return "", nil, nil }

// Destroy does nothing.
func (s *Stream) Destroy() {}
