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
	if !slices.Contains(args, "fps=5,scale=960:-2") {
		t.Error("preview is not high resolution")
	}
}

// A recording is written as fragmented mp4, so that one which never stops
// cleanly is still playable: a plain mp4 of a killed recording holds nothing.
func TestBuildArgsWritesAFragmentedFile(t *testing.T) {
	args := buildArgs([]string{"-f", "gdigrab", "-i", "desktop"}, nil, "", "out.mp4")
	flags := ""
	if i := slices.Index(args, "-movflags"); i >= 0 && i+1 < len(args) {
		flags = args[i+1]
	}
	for _, want := range []string{"frag_keyframe", "empty_moov", "default_base_moof"} {
		if !strings.Contains(flags, want) {
			t.Errorf("movflags %q lacks %s", flags, want)
		}
	}
	// Fragments are cut at keyframes, so the encoder has to emit them.
	if i := slices.Index(args, "-g"); i < 0 || args[i+1] == "" {
		t.Errorf("no keyframe interval: %v", args)
	}
}

// Whatever encoder was probed, the arguments carry its name and its rate
// control, and the file still asks for a pixel format every decoder reads.
func TestBuildArgsUseTheProbedEncoder(t *testing.T) {
	enc := videoEncoder()
	args := buildArgs([]string{"-f", "gdigrab", "-i", "desktop"}, nil, "", "out.mp4")
	i := slices.Index(args, "-c:v")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("no video codec: %v", args)
	}
	if args[i+1] != enc.name {
		t.Errorf("codec %q, probed %q", args[i+1], enc.name)
	}
	if !slices.Contains(args, "yuv420p") {
		t.Error("no pixel format")
	}
	if len(enc.opts) > 0 && !slices.Contains(args, enc.opts[0]) {
		t.Errorf("encoder options %v missing: %v", enc.opts, args)
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
