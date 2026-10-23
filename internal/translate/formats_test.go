package translate

import (
	"strings"
	"testing"
)

func TestFormatBilingualSRT(t *testing.T) {
	segs := []TranslationSegment{
		{
			Idx:            1,
			StartMs:        1000,
			EndMs:          3000,
			OriginalText:   "你好世界",
			TranslatedText: "Hello world",
		},
	}
	res := FormatBilingualSRT(segs)
	if !strings.Contains(res, "00:00:01,000 --> 00:00:03,000") {
		t.Fatalf("bad timestamp: %s", res)
	}
	if !strings.Contains(res, "Hello world\n你好世界") {
		t.Fatalf("bilingual order mismatch: %s", res)
	}
}

func TestFormatSRT(t *testing.T) {
	segs := []TranslationSegment{
		{
			Idx:            1,
			StartMs:        500,
			EndMs:          1500,
			OriginalText:   "你好",
			TranslatedText: "Hello",
		},
	}
	res := FormatSRT(segs)
	if !strings.Contains(res, "Hello") || strings.Contains(res, "你好") {
		t.Fatalf("single language SRT should contain only translated text: %s", res)
	}
}

func TestFormatVTT(t *testing.T) {
	segs := []TranslationSegment{
		{
			Idx:            1,
			StartMs:        500,
			EndMs:          1500,
			TranslatedText: "Hello",
		},
	}
	res := FormatVTT(segs)
	if !strings.HasPrefix(res, "WEBVTT") || !strings.Contains(res, "00:00:00.500 --> 00:00:01.500") {
		t.Fatalf("bad vtt: %s", res)
	}
}
