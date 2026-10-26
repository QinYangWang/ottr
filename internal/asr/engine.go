package asr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"ottr/internal/models"
	"ottr/internal/tools"
)

type sherpaResult struct {
	Text       string    `json:"text"`
	Tokens     []string  `json:"tokens"`
	Timestamps []float64 `json:"timestamps"`
}

// ExtractWav converts any media file's audio stream to 16kHz mono 16-bit WAV PCM.
func ExtractWav(ctx context.Context, mediaPath, wavPath string) error {
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-y", "-i", mediaPath,
		"-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", wavPath)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("audio extraction failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RunSherpa executes sherpa-onnx on a 16kHz WAV file and returns grouped subtitle segments.
func RunSherpa(ctx context.Context, binPath string, model *models.ASRModel, wavPath string) ([]Segment, error) {
	cmd := exec.CommandContext(ctx, binPath,
		"--tokens="+model.Tokens,
		"--encoder="+model.Encoder,
		"--decoder="+model.Decoder,
		"--joiner="+model.Joiner,
		"--provider=cpu",
		"--num-threads=2",
		wavPath)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sherpa-onnx failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	var allSegments []Segment
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var res sherpaResult
		if err := json.Unmarshal(line, &res); err != nil {
			continue
		}
		if len(res.Tokens) > 0 {
			segs := GroupTokens(res.Tokens, res.Timestamps)
			for _, s := range segs {
				s.Idx = len(allSegments) + 1
				allSegments = append(allSegments, s)
			}
		}
	}
	return allSegments, nil
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
