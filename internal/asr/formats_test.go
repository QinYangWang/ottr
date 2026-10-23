package asr

import (
	"strings"
	"testing"
)

func TestFormatSRT(t *testing.T) {
	segs := []Segment{
		{Idx: 1, StartMs: 1200, EndMs: 4500, Text: "Hello world"},
		{Idx: 2, StartMs: 5000, EndMs: 8250, Text: "Second line"},
	}
	srt := FormatSRT(segs)
	if !strings.Contains(srt, "00:00:01,200 --> 00:00:04,500") {
		t.Fatalf("unexpected srt: %s", srt)
	}
	if !strings.Contains(srt, "Hello world") || !strings.Contains(srt, "Second line") {
		t.Fatalf("missing text in srt: %s", srt)
	}
}

func TestFormatVTT(t *testing.T) {
	segs := []Segment{
		{Idx: 1, StartMs: 1200, EndMs: 4500, Text: "Hello world"},
	}
	vtt := FormatVTT(segs)
	if !strings.HasPrefix(vtt, "WEBVTT") {
		t.Fatalf("vtt must start with WEBVTT: %s", vtt)
	}
	if !strings.Contains(vtt, "00:00:01.200 --> 00:00:04.500") {
		t.Fatalf("unexpected vtt: %s", vtt)
	}
}

func TestFormatTXT(t *testing.T) {
	segs := []Segment{
		{Idx: 1, StartMs: 0, EndMs: 1000, Text: "Line one"},
		{Idx: 2, StartMs: 1000, EndMs: 2000, Text: "Line two"},
	}
	txt := FormatTXT(segs)
	lines := strings.Split(strings.TrimSpace(txt), "\n")
	if len(lines) != 2 || lines[0] != "Line one" || lines[1] != "Line two" {
		t.Fatalf("unexpected txt: %s", txt)
	}
}
