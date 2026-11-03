package asr

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"ottr/internal/asr/native"
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
	if !mdl.Streaming {
		return recognizeOffline(ctx, mdl, samples, rate)
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

// recognizeOffline uses the non-streaming API required by offline transducer
// models such as ReazonSpeech. It never sends those models to the online API.
func recognizeOffline(ctx context.Context, mdl models.ASRModel, samples []float32, rate int) ([]Segment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rec, err := native.NewOfflineRecognizer(mdl.Tokens, mdl.Encoder, mdl.Decoder, mdl.Joiner, 2)
	if err != nil {
		return nil, fmt.Errorf("sherpa-onnx could not load the offline model in %s: %w", mdl.Dir, err)
	}
	defer rec.Destroy()
	const chunkSamples = 16000 * 10 // bound offline attention memory on long files
	var segments []Segment
	for start := 0; start < len(samples); start += chunkSamples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+chunkSamples, len(samples))
		stream := rec.Stream()
		if stream == nil {
			return nil, fmt.Errorf("sherpa-onnx could not open an offline recognition stream")
		}
		stream.Accept(rate, samples[start:end])
		stream.Decode()
		text, tokens, timestamps := stream.Result()
		stream.Destroy()
		offset := float64(start) / float64(rate)
		if len(tokens) == 0 {
			text = strings.TrimSpace(text)
			if text != "" {
				segments = append(segments, Segment{
					StartMs: int64(offset * 1000), EndMs: int64(float64(end) / float64(rate) * 1000), Text: text,
				})
			}
			continue
		}
		for i := range timestamps {
			timestamps[i] += offset
		}
		segments = append(segments, GroupTokens(tokens, timestamps)...)
	}
	segments = mergeShortSegments(segments, 12000) // only artifacts at chunk edges
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
	return groupTokens(tokens, timestamps, 2000)
}

func groupTokens(tokens []string, timestamps []float64, shortGapMs int64) []Segment {
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
	return mergeShortSegments(segments, shortGapMs)
}

// mergeShortSegments keeps isolated characters and other tiny fragments out
// of their own rows. Sentence punctuation and genuinely long pauses stay hard
// boundaries; the larger offline gap joins artifacts at chunk edges.
func mergeShortSegments(segments []Segment, maxGapMs int64) []Segment {
	if len(segments) < 2 {
		return segments
	}
	out := make([]Segment, 0, len(segments))
	for _, seg := range segments {
		if len(out) > 0 {
			prev := &out[len(out)-1]
			gap := seg.StartMs - prev.EndMs
			short := utf8.RuneCountInString(prev.Text) < 4 || utf8.RuneCountInString(seg.Text) < 4
			if gap >= 0 && gap <= maxGapMs && short && !strings.ContainsAny(prev.Text, "。！？!?") {
				prev.Text += seg.Text
				prev.EndMs = seg.EndMs
				continue
			}
		}
		out = append(out, seg)
	}
	return out
}
