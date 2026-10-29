package native

import (
	"testing"

	"ottr/internal/asr"
)

// A partial revision of the same utterance replaces the line it belongs to; a
// final one freezes it and the next utterance starts a new line. The pane keeps
// the newest six finalized lines plus the one being spoken.
func TestLiveRevisionsUpdateInPlace(t *testing.T) {
	a := &App{}
	a.reviseLive(asr.LiveSubtitle{Utterance: 1, StartMs: 0, Text: "你"})
	a.reviseLive(asr.LiveSubtitle{Utterance: 1, StartMs: 0, Text: "你好"})
	a.reviseLive(asr.LiveSubtitle{Utterance: 1, StartMs: 0, Text: "你好，世界", Final: true})
	if len(a.late) != 1 {
		t.Fatalf("partials added lines: %+v", a.late)
	}
	if a.late[0].text != "你好，世界" || !a.late[0].final {
		t.Fatalf("line is %+v, want the final revision in place", a.late[0])
	}

	a.reviseLive(asr.LiveSubtitle{Utterance: 2, StartMs: 2000, Text: "下"})
	if len(a.late) != 2 || a.late[1].text != "下" || a.late[1].at != 2000 {
		t.Fatalf("a new utterance did not start a line: %+v", a.late)
	}
	if a.late[1].final {
		t.Error("a partial revision is marked final")
	}
}

func TestLivePaneKeepsSixFinalLinesAndTheLiveOne(t *testing.T) {
	a := &App{}
	for i := int64(1); i <= 9; i++ {
		a.reviseLive(asr.LiveSubtitle{Utterance: i, StartMs: i * 1000, Text: "line", Final: true})
	}
	a.reviseLive(asr.LiveSubtitle{Utterance: 10, StartMs: 10000, Text: "live"})
	if len(a.late) != 7 {
		t.Fatalf("want six final lines plus the live one, got %d: %+v", len(a.late), a.late)
	}
	if a.late[len(a.late)-1].text != "live" {
		t.Errorf("the live line was dropped: %+v", a.late)
	}
	if a.late[0].utterance != 4 {
		t.Errorf("want the oldest kept line to be utterance 4, got %d", a.late[0].utterance)
	}
}

// A warning revision carries no text and must not become a line.
func TestLiveWarningIsNotALine(t *testing.T) {
	a := &App{}
	a.reviseLive(asr.LiveSubtitle{Warn: "behind"})
	if len(a.late) != 0 {
		t.Fatalf("a warning became a line: %+v", a.late)
	}
	if a.liveWarn != "behind" {
		t.Errorf("the warning is not kept for the pane: %q", a.liveWarn)
	}
	a.reviseLive(asr.LiveSubtitle{Utterance: 1, Text: "ok"})
	if a.liveWarn != "" {
		t.Errorf("text did not clear the warning: %q", a.liveWarn)
	}
}
