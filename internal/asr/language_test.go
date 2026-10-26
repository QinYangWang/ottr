package asr

import (
	"testing"
	"time"
)

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		name string
		text []string
		want string
	}{
		{"chinese", []string{"今天我们讨论一下发布计划", "下周一之前完成"}, "zh"},
		{"japanese", []string{"今日はリリースについて話します"}, "ja"},
		{"korean", []string{"오늘 회의를 시작하겠습니다"}, "ko"},
		{"english", []string{"Let us talk about the release plan."}, "en"},
		{"chinese with an english word", []string{"我们用 Docker 部署这个服务"}, "zh"},
		{"nothing to judge by", []string{"", "12345"}, DefaultLanguage},
		{"no segments", nil, DefaultLanguage},
	}
	for _, tc := range cases {
		var segs []Segment
		for i, text := range tc.text {
			segs = append(segs, Segment{Idx: i + 1, Text: text})
		}
		if got := DetectLanguage(segs); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTranscriptLanguageIsDetectedAndCanBeSet(t *testing.T) {
	svc, _ := setupTestASR(t)
	db, _ := svc.store()

	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio)
		VALUES ('english', 'import', '/tmp/english.mp4', ?, 1)`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := res.LastInsertId()

	if err := svc.saveTranscript(mediaID, []Segment{{Idx: 1, StartMs: 0, EndMs: 1000, Text: "hello there"}}); err != nil {
		t.Fatal(err)
	}
	tr, err := svc.Get(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Language != "en" {
		t.Errorf("saved language is %q, want the detected en", tr.Language)
	}

	// The user can say otherwise, and that is what translation reads.
	if err := svc.SetLanguage(mediaID, "JA"); err != nil {
		t.Fatal(err)
	}
	if tr, err = svc.Get(mediaID); err != nil {
		t.Fatal(err)
	}
	if tr.Language != "ja" {
		t.Errorf("language after SetLanguage is %q, want ja", tr.Language)
	}

	if err := svc.SetLanguage(mediaID+1, "en"); err == nil {
		t.Error("setting the language of a media file without a transcript reported no error")
	}
}
