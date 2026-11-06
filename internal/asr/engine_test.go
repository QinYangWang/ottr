package asr

import (
	"strings"
	"testing"

	"ottr/internal/asr/native"
	"ottr/internal/models"
)

func TestSpeakerRangesUseTheLargestOverlap(t *testing.T) {
	segments := []Segment{{StartMs: 0, EndMs: 1000}, {StartMs: 1000, EndMs: 2000}}
	ranges := []native.SpeakerSegment{{StartMs: 0, EndMs: 400, Speaker: 2}, {StartMs: 400, EndMs: 1800, Speaker: 4}}
	segments = speakerRanges(segments, ranges)
	if segments[0].Speaker != 0 || segments[1].Speaker != 0 {
		t.Fatalf("speaker assignment = %+v", segments)
	}
}

func TestGroupTokens(t *testing.T) {
	tokens := []string{"你", "好", "世", "界", "，", "天", "气", "真", "好", "。"}
	timestamps := []float64{0.1, 0.3, 0.5, 0.7, 0.9, 1.8, 2.0, 2.2, 2.4, 2.6}

	segs := GroupTokens(tokens, timestamps)
	if len(segs) != 2 {
		t.Fatalf("want 2 segments due to pause (>0.75s), got %d: %+v", len(segs), segs)
	}
	if segs[0].Text != "你好世界，" {
		t.Errorf("seg 0 text: %q", segs[0].Text)
	}
	if segs[1].Text != "天气真好。" {
		t.Errorf("seg 1 text: %q", segs[1].Text)
	}
	if segs[0].StartMs != 100 {
		t.Errorf("seg 0 start: %d", segs[0].StartMs)
	}
	if segs[1].StartMs != 1800 {
		t.Errorf("seg 1 start: %d", segs[1].StartMs)
	}
}

func TestOfflineModelCanBeUsedForLiveRecognition(t *testing.T) {
	st, err := newStreamer(models.ASRModel{Dir: "offline-model"})
	if st != nil {
		st.destroy()
	}
	if err != nil && strings.Contains(err.Error(), "choose a streaming model") {
		t.Fatalf("offline model was rejected for live recognition: %v", err)
	}
}

func TestGroupTokensMergesSingleCharacterFragments(t *testing.T) {
	tokens := []string{"課", "長", "は", "い", "は", "い"}
	timestamps := []float64{0, 1.5, 10, 19, 20, 29}
	segments := groupTokens(tokens, timestamps, 12000)
	if len(segments) != 1 || segments[0].Text != "課長はいはい" {
		t.Fatalf("short fragments were not merged: %+v", segments)
	}
}

func TestGroupTokensKeepsSentenceBoundaries(t *testing.T) {
	tokens := []string{"好", "。", "好", "。"}
	timestamps := []float64{0, 0.2, 1, 1.2}
	segments := GroupTokens(tokens, timestamps)
	if len(segments) != 2 || segments[0].Text != "好。" || segments[1].Text != "好。" {
		t.Fatalf("sentence boundaries changed: %+v", segments)
	}
}

func TestGroupTokensKeepsSegmentsOrdered(t *testing.T) {
	tokens := []string{"あ", "。", "い", "。"}
	timestamps := []float64{4.3, 4.319, 4.32, 4.329}
	segments := GroupTokens(tokens, timestamps)
	for i, seg := range segments {
		if i > 0 && seg.StartMs < segments[i-1].EndMs {
			t.Fatalf("segments overlap: %+v", segments)
		}
		if seg.EndMs <= seg.StartMs {
			t.Fatalf("invalid segment duration: %+v", seg)
		}
	}
}

func TestGroupTokensEmpty(t *testing.T) {
	segs := GroupTokens(nil, nil)
	if len(segs) != 0 {
		t.Fatalf("want empty, got %d", len(segs))
	}
}
