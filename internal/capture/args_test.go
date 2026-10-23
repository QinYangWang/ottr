package capture

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildArgsNoAudio(t *testing.T) {
	args := buildArgs([]string{"-f", "gdigrab", "-i", "desktop"}, nil, "127.0.0.1:9", "out.mp4")
	joined := slices.Concat(args)
	if !slices.Contains(joined, "-map") {
		t.Fatal("no map")
	}
	// video-only: no audio codec
	if slices.Contains(joined, "-c:a") {
		t.Error("audio codec without audio input")
	}
	// preview output present
	if !slices.Contains(joined, "tcp://127.0.0.1:9") {
		t.Error("no preview output")
	}
}

func TestBuildArgsOneAudio(t *testing.T) {
	args := buildArgs([]string{"-f", "gdigrab", "-i", "desktop"},
		[]audioSpec{{rate: 48000, channels: 2, pipe: `\\.\pipe\p0`}}, "", "out.mp4")
	if !slices.Contains(args, "f32le") {
		t.Fatal("no pcm input")
	}
	if slices.Contains(args, "-filter_complex") {
		t.Error("single stream needs no mixer")
	}
}

func TestBuildArgsTwoAudioMixes(t *testing.T) {
	args := buildArgs([]string{"-f", "gdigrab", "-i", "desktop"},
		[]audioSpec{{rate: 48000, channels: 2, pipe: "p0"}, {rate: 44100, channels: 1, pipe: "p1"}},
		"", "out.mp4")
	i := slices.Index(args, "-filter_complex")
	if i < 0 || !strings.Contains(args[i+1], "amix") {
		t.Fatal("two streams must be resampled and mixed")
	}
}
