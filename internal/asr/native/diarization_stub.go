//go:build !cgo

package native

type SpeakerSegment struct {
	StartMs int64
	EndMs   int64
	Speaker int
}

func Diarize(segmentation, embedding string, samples []float32) ([]SpeakerSegment, error) {
	return nil, ErrUnavailable
}
