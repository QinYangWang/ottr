package native

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/summary"
	"ottr/internal/translate"
)

func TestTranscribeSteps(t *testing.T) {
	states := func(p float64) []stepState {
		var out []stepState
		for _, s := range transcribeSteps(p) {
			out = append(out, s.state)
		}
		return out
	}
	tests := []struct {
		p    float64
		want []stepState
	}{
		{0, []stepState{stepDone, stepActive, stepQueued, stepQueued, stepQueued}},
		{asr.PhaseRecognizing, []stepState{stepDone, stepDone, stepActive, stepQueued, stepQueued}},
		{0.5, []stepState{stepDone, stepDone, stepActive, stepQueued, stepQueued}},
		{asr.PhaseSpeakers, []stepState{stepDone, stepDone, stepDone, stepActive, stepQueued}},
		{asr.PhaseSaving, []stepState{stepDone, stepDone, stepDone, stepDone, stepActive}},
		{1, []stepState{stepDone, stepDone, stepDone, stepDone, stepDone}},
	}
	for _, tc := range tests {
		got := states(tc.p)
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("p=%v: steps %v, want %v", tc.p, got, tc.want)
				break
			}
		}
	}
}

// The time left is extrapolated from how fast recognition moved since it was
// first seen, and only once it has moved.
func TestSpeechClock(t *testing.T) {
	var clock speechClock
	start := time.Unix(1000, 0)
	job := jobs.Job{ID: 7, State: jobs.StateRunning, Progress: 0.2}
	clock.observe(job, start)
	if _, ok := clock.left(job, start.Add(10*time.Second)); ok {
		t.Fatal("an estimate before recognition moved")
	}
	job.Progress = 0.3 // 0.1 in 10 s, 0.4 to go
	left, ok := clock.left(job, start.Add(10*time.Second))
	if !ok || left != 40*time.Second {
		t.Fatalf("left = %v, %v; want 40s", left, ok)
	}
	clock.observe(job, start.Add(10*time.Second)) // the same job is not re-anchored
	if clock.at != start {
		t.Error("the clock moved its anchor")
	}
	if _, ok := clock.left(jobs.Job{ID: 8, Progress: 0.3}, start); ok {
		t.Error("an estimate for another job")
	}
}

func TestDayGroupAndShortDir(t *testing.T) {
	a := &App{}
	now := time.Date(2026, 11, 5, 13, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{now.Add(-time.Hour), "Today"},
		{time.Date(2026, 11, 4, 23, 59, 0, 0, time.Local), "Yesterday"},
		{time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local), "Nov 3"},
	} {
		if got := a.dayGroup(now, tc.at); got != tc.want {
			t.Errorf("dayGroup(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
	if got := a.groupLabel("Today"); got != "TODAY" {
		t.Errorf("groupLabel = %q", got)
	}
	if got := shortDir(filepath.Join("a", "Videos", "Ottr")); got != "Videos › Ottr" {
		t.Errorf("shortDir = %q", got)
	}
}

// waitFor draws frames until the window says text, or fails.
func waitFor(t *testing.T, tt *ui.Tester, text string) {
	t.Helper()
	for i := 0; i < 200 && !tt.HasText(text); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if !tt.HasText(text) {
		t.Fatalf("the window never said %q: %q", text, tt.Texts())
	}
}

// A transcription in progress shows how much of the recording it has read and
// the step it is on, and Cancel stops it.
func TestTranscribingPanel(t *testing.T) {
	a, id, db := newTestAppDB(t)
	if _, err := db.Exec(`UPDATE media SET transcript_status = 'pending' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	half := asr.PhaseRecognizing + 0.5*(asr.PhaseSpeakers-asr.PhaseRecognizing)
	if _, err := a.Jobs.Run(id, jobs.KindTranscribe, func(ctx context.Context, progress func(float64)) error {
		progress(half)
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.openDetail(id)
	tt := ui.NewTester(a.view, 1280, 800)
	waitFor(t, tt, "Transcribing your recording")
	waitFor(t, tt, "0:30 of 1:01 processed")
	for _, want := range []string{"Saved to disk", "Speech to text", "50%", "queued"} {
		if !tt.HasText(want) {
			t.Errorf("the panel lacks %q: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Stop this job"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if job, ok := a.Jobs.Latest(id); ok && !job.Running() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the job did not stop")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A transcription that fails on the media shown opens the failure dialog,
// which says where it stopped and is dismissed by Later; the list marks it.
func TestFailedTranscriptionDialog(t *testing.T) {
	a, id, db := newTestAppDB(t)
	if _, err := db.Exec(`UPDATE media SET transcript_status = 'pending' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	quarter := asr.PhaseRecognizing + 0.25*(asr.PhaseSpeakers-asr.PhaseRecognizing)
	if _, err := a.Jobs.Run(id, jobs.KindTranscribe, func(ctx context.Context, progress func(float64)) error {
		progress(quarter)
		<-release
		db.Exec(`UPDATE media SET transcript_status = 'error' WHERE id = ?`, id)
		return errors.New("the model ran out of memory")
	}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.openDetail(id)
	tt := ui.NewTester(a.view, 1280, 800)
	waitFor(t, tt, "Transcribing your recording")
	close(release)
	waitFor(t, tt, "Couldn’t finish the transcript")
	if !tt.HasText("the model ran out of memory") {
		t.Errorf("the dialog does not say why: %q", tt.Texts())
	}
	waitFor(t, tt, "“standup” stopped at 0:15 of 1:01. The video is safe — only the text is incomplete.")
	if err := tt.Click("Later"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.failedOpen || tt.HasText("Couldn’t finish the transcript") {
		t.Fatalf("Later left the dialog open: %q", tt.Texts())
	}
	a.closeDetail()
	a.reload()
	waitFor(t, tt, "Failed")
	waitFor(t, tt, "· transcript stopped at 0:15")
}

// Deleting a recording can keep its transcript as text beside where it was.
func TestDeleteKeepsTranscript(t *testing.T) {
	a, id, db := newTestAppDB(t)
	dir := t.TempDir()
	video := filepath.Join(dir, "standup.mp4")
	if err := os.WriteFile(video, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE media SET path = ? WHERE id = ?`, video, id); err != nil {
		t.Fatal(err)
	}
	a.reload()
	tt := ui.NewTester(a.view, 1280, 800)
	a.askDelete(id)
	waitFor(t, tt, "Delete this recording?")
	if err := tt.Click("Keep the transcript as a .txt file"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.keepText {
		t.Fatal("the checkbox did not keep its state")
	}
	if err := tt.Click("Delete recording"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200 && len(a.media) > 0; i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if len(a.media) != 0 {
		t.Fatal("the recording is still listed")
	}
	data, err := os.ReadFile(filepath.Join(dir, "standup.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[0:00] Speaker 1: first line\n[0:02] Speaker 1: second line\n" {
		t.Errorf("the kept transcript says %q", data)
	}
	if _, err := os.Stat(video); !os.IsNotExist(err) {
		t.Errorf("the video is still on disk: %v", err)
	}
}

// The export dialog picks a format and options, and hands them to the
// transcript service as it takes them.
func TestExportDialog(t *testing.T) {
	a, id := newTestApp(t)
	a.openDetail(id)
	tt := ui.NewTester(a.view, 1280, 800)
	waitFor(t, tt, "first line")
	if err := tt.Click("Export the transcript"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, tt, "Export transcript")
	waitFor(t, tt, "standup · 2 segments · 1 speakers")
	if !tt.HasText("Export .srt") {
		t.Fatalf("SRT is not the default: %q", tt.Texts())
	}
	if err := tt.Click("Plain text"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, tt, "Export .txt")
	m, _ := a.current()
	a.trans.Segments = []translate.TranslationSegment{{SegmentID: a.segs[0].ID, TranslatedText: "第一行"}}
	a.exportTranslation = true
	opts := a.exportOptions(m)
	if !opts.Speakers || !opts.Timestamps || opts.Translation[a.segs[0].ID] != "第一行" || opts.Preamble != "" {
		t.Errorf("options = %+v", opts)
	}
	a.exportFormat = "md"
	a.sum.ContentMarkdown = "# standup\n"
	if got := a.exportOptions(m).Preamble; got != "# standup\n" {
		t.Errorf("the Markdown file does not start with the summary: %q", got)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.exportOpen {
		t.Error("Cancel left the dialog open")
	}
}

// The summary tab shows what the service stored, chapters and highlights with
// their times.
func TestSummaryTab(t *testing.T) {
	a, id := newTestApp(t)
	a.openDetail(id)
	a.tab = 1
	a.sumLoaded = true
	a.sum = summary.Summary{
		Overview: "The team planned the release.", Model: "deepseek-chat",
		ContentMarkdown: "# standup\n", ActionItems: []string{"Fix the dialog"},
		Chapters:   []summary.Chapter{{StartMs: 8000, Title: "Export dialog blocker"}},
		Highlights: []summary.Highlight{{StartMs: 8000, Note: "The one thing blocking us."}},
	}
	tt := ui.NewTester(a.view, 1280, 800)
	waitFor(t, tt, "The team planned the release.")
	for _, want := range []string{"deepseek-chat", "Chapters", "0:08", "Export dialog blocker",
		"Action items", "Fix the dialog", "The one thing blocking us."} {
		if !tt.HasText(want) {
			t.Errorf("the summary lacks %q: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("Generate again"); !ok {
		t.Error("no way to generate the summary again")
	}
}

// An empty library offers both ways to fill it.
func TestEmptyLibraryOffersRecordAndImport(t *testing.T) {
	a, id := newTestApp(t)
	a.removeMedia(id, false)
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 200 && len(a.media) > 0; i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	waitFor(t, tt, "No recordings yet")
	if !tt.HasText("Import a video") {
		t.Errorf("no import in the empty library: %q", tt.Texts())
	}
}

// Settings ends its menu with the way back to the recorder.
func TestSettingsBackToRecorder(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	if !tt.HasText("Back to recorder") {
		t.Fatalf("no way back: %q", tt.Texts())
	}
	if err := tt.Click("Back to recorder"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}
