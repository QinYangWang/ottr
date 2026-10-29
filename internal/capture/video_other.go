//go:build !windows

package capture

import (
	"errors"
	"fmt"
)

// videoInput is a native frame path into the encoder. Only Windows has one so
// far; macOS and Linux arrive with their backends.
type videoInput struct{ width, height int }

func newVideoInput(width, height int) (*videoInput, error) {
	return nil, errors.New("native video capture is not implemented on this platform yet")
}

func (v *videoInput) args(width, height int) []string { return nil }

func (v *videoInput) offer(frame []byte) {}

func (v *videoInput) close() {}

var _ = fmt.Sprint
