//go:build !windows

package capture

import (
	"context"
	"fmt"
	"runtime"
)

// newBackend is the platform's capture backend. Only Windows has one so far;
// macOS and Linux arrive in the phases of docs/native-capture.md.
func newBackend() Backend { return unsupportedBackend{} }

type unsupportedBackend struct{}

func (unsupportedBackend) Sources(context.Context) ([]Source, error) {
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

func (unsupportedBackend) Open(context.Context, StreamConfig) (Stream, error) {
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

func (unsupportedBackend) Snapshot(context.Context, SnapshotConfig) (Frame, error) {
	return Frame{}, fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

func (unsupportedBackend) Close() error { return nil }
