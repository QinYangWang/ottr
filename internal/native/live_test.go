package native

import (
	"context"
	"sync"
	"testing"
	"time"

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

func TestLivePaneKeepsEveryLine(t *testing.T) {
	a := &App{}
	for i := int64(1); i <= 40; i++ {
		a.reviseLive(asr.LiveSubtitle{Utterance: i, StartMs: i * 1000, Text: "line", Final: true})
	}
	a.reviseLive(asr.LiveSubtitle{Utterance: 41, StartMs: 41000, Text: "live"})
	if len(a.late) != 41 {
		t.Fatalf("the pane dropped history: got %d lines", len(a.late))
	}
	if a.late[0].utterance != 1 || a.late[len(a.late)-1].text != "live" {
		t.Errorf("the pane keeps %d..%d, want 1..41", a.late[0].utterance, a.late[len(a.late)-1].utterance)
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

// Live translation is opt-in, costs one request per finalized line, and drops
// the line when the target language changes so it is never shown stale.
func TestLiveTranslationFollowsFinalLines(t *testing.T) {
	a := &App{}
	var mu sync.Mutex
	calls := 0
	a.liveTranslator = func(_ context.Context, texts []string, source, target string) ([]string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return []string{"[" + target + "] " + texts[0]}, nil
	}
	a.trLang = "en"
	a.liveTranslate = true

	a.reviseLive(asr.LiveSubtitle{Utterance: 1, Text: "你好"})
	a.drain()
	if got := callCount(&mu, &calls); got != 0 {
		t.Fatalf("a line still being spoken was translated: %d calls", got)
	}

	a.reviseLive(asr.LiveSubtitle{Utterance: 1, Text: "你好，世界", Final: true})
	waitLive(t, a, func() bool { return a.liveTrans[1].translated != "" })
	if got := a.liveTrans[1].translated; got != "[en] 你好，世界" {
		t.Fatalf("translated line is %q", got)
	}

	// The same final revision again must not ask a second time.
	a.reviseLive(asr.LiveSubtitle{Utterance: 1, Text: "你好，世界", Final: true})
	a.drain()
	if got := callCount(&mu, &calls); got != 1 {
		t.Fatalf("the same line was translated twice: %d calls", got)
	}

	a.trLang = "zh"
	a.restartLiveTranslation()
	waitLive(t, a, func() bool { return a.liveTrans[1].translated == "[zh] 你好，世界" })
	if got := callCount(&mu, &calls); got != 2 {
		t.Fatalf("changing the language did not translate the line again: %d calls", got)
	}
}

func callCount(mu *sync.Mutex, n *int) int {
	mu.Lock()
	defer mu.Unlock()
	return *n
}

func waitLive(t *testing.T, a *App, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.drain()
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the live translation did not arrive")
}
