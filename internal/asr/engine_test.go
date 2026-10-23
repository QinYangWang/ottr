package asr

import (
	"testing"
)

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

func TestGroupTokensEmpty(t *testing.T) {
	segs := GroupTokens(nil, nil)
	if len(segs) != 0 {
		t.Fatalf("want empty, got %d", len(segs))
	}
}
