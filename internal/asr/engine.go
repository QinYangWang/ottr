package asr

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"ottr/internal/models"
)

// batchSamples is how much audio one feed takes. Feeding in pieces lets a
// cancelled job stop at the next one instead of waiting out the whole file.
const batchSamples = 16000 // 1 s

// recognizeWav runs a 16 kHz mono 16-bit WAV file through the streaming
// recognizer and returns its subtitle segments on the file's timeline.
//
// The whole file is one utterance chain: an endpoint inside it ends an
// utterance and starts the next, exactly as when the audio was live.
func recognizeWav(ctx context.Context, mdl models.ASRModel, wavPath string) ([]Segment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	samples, rate, err := readWav(wavPath)
	if err != nil {
		return nil, err
	}
	if rate <= 0 {
		return nil, fmt.Errorf("read %s: no sample rate", wavPath)
	}
	st, err := newStreamer(mdl)
	if err != nil {
		return nil, err
	}
	defer st.close()

	conv := NewConverter(rate, 1)
	var segments []Segment
	flush := func() {
		segments = append(segments, st.result().segments...)
	}
	for i := 0; i < len(samples); i += batchSamples {
		if err := ctx.Err(); err != nil {
			return nil, err // nothing is freed under an active native call
		}
		end := min(i+batchSamples, len(samples))
		st.feed(conv.Feed(samples[i:end]))
		if st.endpoint() {
			flush()
			st.reset()
		}
	}
	st.feed(conv.Flush())
	st.finish()
	flush()
	for i := range segments {
		segments[i].Idx = i + 1
	}
	return segments, nil
}

// readWav reads a 16 kHz mono 16-bit WAV file into float samples, and says what
// rate they are at in case the file is not 16 kHz.
func readWav(path string) ([]float32, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%s is not a WAV file", path)
	}
	var rate, channels, bits int
	var body []byte
	for off := 12; off+8 <= len(data); {
		id, size := string(data[off:off+4]), int(binary.LittleEndian.Uint32(data[off+4:]))
		start := off + 8
		end := min(start+size, len(data))
		switch id {
		case "fmt ":
			if end-start < 16 {
				return nil, 0, fmt.Errorf("%s has a truncated header", path)
			}
			channels = int(binary.LittleEndian.Uint16(data[start+2:]))
			rate = int(binary.LittleEndian.Uint32(data[start+4:]))
			bits = int(binary.LittleEndian.Uint16(data[start+14:]))
		case "data":
			body = data[start:end]
		}
		off = start + size + size&1 // chunks are word aligned
	}
	if rate == 0 || channels == 0 {
		return nil, 0, fmt.Errorf("%s has no audio format chunk", path)
	}
	if bits != 16 {
		return nil, 0, fmt.Errorf("%s is %d-bit; only 16-bit PCM is read", path, bits)
	}
	frames := len(body) / 2 / channels
	out := make([]float32, frames)
	for i := 0; i < frames; i++ {
		var sum int32
		for ch := 0; ch < channels; ch++ {
			at := (i*channels + ch) * 2
			sum += int32(int16(binary.LittleEndian.Uint16(body[at:])))
		}
		out[i] = float32(sum) / float32(channels) / 32768
	}
	return out, rate, nil
}

// GroupTokens splits token sequences with timestamps into clean subtitle segments.
func GroupTokens(tokens []string, timestamps []float64) []Segment {
	if len(tokens) == 0 {
		return nil
	}
	if len(timestamps) != len(tokens) {
		// Fallback if timestamps are missing: synthesize estimated spacing
		timestamps = make([]float64, len(tokens))
		for i := range timestamps {
			timestamps[i] = float64(i) * 0.2
		}
	}

	var segments []Segment
	var curTokens []string
	curStart := timestamps[0]
	prevTime := timestamps[0]

	flush := func(endTime float64) {
		if len(curTokens) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(curTokens, ""))
		if text != "" {
			if endTime <= curStart {
				endTime = curStart + 0.3
			}
			segments = append(segments, Segment{
				Idx:     len(segments) + 1,
				StartMs: int64(curStart * 1000),
				EndMs:   int64(endTime * 1000),
				Text:    text,
			})
		}
		curTokens = nil
	}

	for i, tok := range tokens {
		t := timestamps[i]
		if len(curTokens) > 0 && (t-prevTime) > 0.75 {
			flush(prevTime + 0.25)
			curStart = t
		}
		curTokens = append(curTokens, tok)
		prevTime = t
		if strings.ContainsAny(tok, "。！？!?") || len(curTokens) >= 25 {
			flush(t + 0.2)
			if i+1 < len(timestamps) {
				curStart = timestamps[i+1]
			}
		}
	}
	flush(prevTime + 0.3)
	return segments
}
