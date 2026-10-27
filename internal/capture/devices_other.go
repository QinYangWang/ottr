//go:build !windows

package capture

import "errors"

func audioDevices() ([]AudioDevice, []AudioDevice, error) {
	return nil, nil, errors.New("audio capture is not implemented on this platform yet (Phase 6)")
}
