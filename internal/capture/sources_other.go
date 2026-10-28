//go:build !windows

package capture

import (
	"errors"
	"runtime"
)

var errUnsupported = errors.New("screen recording is not implemented on " + runtime.GOOS + " yet (Phase 6)")

func listSources() ([]Source, error)            { return nil, errUnsupported }
func resolveSource(id string) ([]string, error) { return nil, errUnsupported }
func resolveSourceFPS(id string, fps int) ([]string, error) {
	return nil, errUnsupported
}
