//go:build !cgo

package native

func SpeakerEmbedding(model string, samples []float32) ([]float32, error) {
	return nil, ErrUnavailable
}
