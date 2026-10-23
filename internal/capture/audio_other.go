//go:build !windows

package capture

import "errors"

type audioStream struct {
	loopback bool
	rate     int
	channels int
	pipe     string
	stop     func()
}

func startAudioStream(loopback bool, sink *liveFeed) (*audioStream, error) {
	return nil, errors.New("audio capture is not implemented on this platform yet (Phase 6)")
}
