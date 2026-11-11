package native

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
	"ottr/internal/summary"
	"ottr/internal/translate"
)

// newTestApp builds an app over a real database, with one transcribed media
// file in it.
func newTestApp(t *testing.T) (*App, int64) {
	t.Helper()
	a, id, _ := newTestAppDB(t)
	return a, id
}

// newTestAppDB is newTestApp with the database, for tests that set up a state
// no service call reaches.
func newTestAppDB(t *testing.T) (*App, int64, *sql.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	lib := &library.Service{}
	lib.Open(db)
	js := &jobs.Service{}
	js.Open(db)
	mm := models.NewManager(t.TempDir(), t.TempDir())
	mm.Open(db)
	asrSvc := asr.NewService(lib, mm, js)
	trSvc, sumSvc := translate.NewService(lib, asrSvc, js), summary.NewService(lib, asrSvc, js)
	asrSvc.Open(db)
	trSvc.Open(db)
	sumSvc.Open(db)
	a := &App{
		Lib: lib,
		Rec: &capture.Recorder{Lib: lib, Dir: t.TempDir()},
		ASR: asrSvc, Trans: trSvc, Sum: sumSvc, Jobs: js, MM: mm,
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO media (name, source, path, duration_ms, has_audio, has_video, created_at, transcript_status)
		VALUES ('standup', 'recording', '/tmp/standup.mp4', 61000, 1, 1, ?, 'done')`, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	tr, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'zh', ?)`, id, now)
	if err != nil {
		t.Fatal(err)
	}
	trID, _ := tr.LastInsertId()
	if _, err := db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text)
		VALUES (?, 1, 0, 2000, 'first line'), (?, 2, 2000, 4000, 'second line')`, trID, trID); err != nil {
		t.Fatal(err)
	}
	a.reload()
	return a, id, db
}

// The library lists what it recorded as cards, and opening one shows its
// transcript, which its tabs switch away from and back to.
func TestWindowShowsTranscript(t *testing.T) {
	a, id := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	// The list shows the file; its row opens the media's page.
	if !tt.HasText("standup") {
		t.Fatalf("the library does not list the recording: %q", tt.Texts())
	}
	tt.Frame()
	a.openDetail(id)
	tt.Frame()
	for i := 0; i < 5 && !tt.HasText("first line"); i++ {
		tt.Frame()
	}
	if !tt.HasText("first line") || !tt.HasText("second line") {
		t.Fatalf("the transcript did not show: %q", tt.Texts())
	}
	// Search sits in the tab bar; the spoken language is on the row under it.
	search, searchOK := tt.Find("Search the subtitles")
	language, languageOK := tt.Find("Spoken language")
	if !searchOK || !languageOK || search.Y+search.H > language.Y {
		t.Errorf("the search button is not above the language selector: %+v %+v", search, language)
	}
	if err := tt.Click("Search the subtitles"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.trans.Segments = []translate.TranslationSegment{{SegmentID: a.segs[0].ID, TranslatedText: "translated first line"}}
	a.transLang = a.trLang
	if err := tt.Click("Subtitle settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Translation"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("translated first line") {
		t.Fatalf("the translation switch did not merge translated rows: %q", tt.Texts())
	}
	// The other tabs ask for what they need rather than showing nothing.
	if err := tt.Click("Summary"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5 && !tt.HasText("No summary yet"); i++ {
		tt.Frame()
	}
	if !tt.HasText("No summary yet") {
		t.Errorf("the Summary tab says %q", tt.Texts())
	}
	a.subtitleSettingsOpen = true // the title bar has a separate Settings button
	tt.Frame()
	if !tt.HasText("Transcription model") {
		t.Fatalf("the playback Settings menu has no model settings: %q", tt.Texts())
	}
	if err := tt.Click("Transcript"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("second line") {
		t.Errorf("back on Transcript, the window says %q", tt.Texts())
	}
}

func TestPlaybackSliderSeeksOnRelease(t *testing.T) {
	video := testVideo(t)
	a, id := newTestApp(t)
	m, _ := a.byID(id)
	m.Path, m.Width, m.Height, m.DurationMs = video, 640, 360, 10000
	a.media[0] = m
	a.openDetail(id)
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	slider, ok := tt.Find("Playback position")
	if !ok {
		t.Fatalf("no playback slider: %q", tt.Texts())
	}
	y := slider.Y + slider.H/2
	tt.Press(slider.X+slider.W*0.2, y)
	tt.Move(slider.X+slider.W*0.7, y)
	tt.Release(slider.X+slider.W*0.7, y)
	if a.playback.seekSeconds < 6.8 || a.playback.seekSeconds > 7.2 {
		t.Fatalf("slider value after drag = %v", a.playback.seekSeconds)
	}
	if delta := a.playback.offset - 7*time.Second; delta < -100*time.Millisecond || delta > 100*time.Millisecond {
		t.Fatalf("release did not seek: %v", a.playback.offset)
	}
	a.closePlayback()
}

func TestRenameMedia(t *testing.T) {
	a, id := newTestApp(t)
	a.openDetail(id)
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	if err := tt.Click("Rename"); err != nil {
		t.Fatal(err)
	}
	a.renameText = "renamed recording"
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && a.media[0].Name != "renamed recording"; i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if a.media[0].Name != "renamed recording" {
		t.Fatalf("media was not renamed: %+v", a.media[0])
	}
	if a.media[0].Path != "/tmp/standup.mp4" {
		t.Fatalf("rename moved the media file: %q", a.media[0].Path)
	}
}

// Searching the library keeps the file listed while it matches, and says so
// when nothing does.
func TestSearchFilters(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	a.query = "nothing-like-this"
	a.reload()
	tt.Frame()
	if !tt.HasText("No results for") {
		t.Errorf("search: %q", tt.Texts())
	}
	a.query = "stand"
	a.reload()
	tt.Frame()
	if !tt.HasText("standup") {
		t.Errorf("search kept: %q", tt.Texts())
	}
}

// Deleting a record leaves the library empty, and the window says so.
func TestDeleteMedia(t *testing.T) {
	a, id := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	a.removeMedia(id, false)
	for i := 0; i < 20 && len(a.media) > 0; i++ {
		time.Sleep(10 * time.Millisecond)
		tt.Frame()
	}
	if len(a.media) != 0 {
		t.Fatalf("still %d records", len(a.media))
	}
	for i := 0; i < 5 && !tt.HasText("Record your first screen"); i++ {
		tt.Frame()
	}
	if !tt.HasText("Record your first screen") {
		t.Errorf("the empty library says %q", tt.Texts())
	}
}

// A summary being generated is a job the window can watch and stop, and
// stopping it leaves the summary as it was rather than as an error.
func TestCancelStopsTheSummaryJob(t *testing.T) {
	a, id := newTestApp(t)
	started := make(chan struct{}, 1)
	gone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the body so that the server notices the client hanging up.
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done(): // answer only once the caller gives up
		case <-gone:
		}
	}))
	defer server.Close()
	defer close(gone)
	if err := a.Sum.SaveSettings(summary.Settings{BaseURL: server.URL, APIKey: "test-key", Model: "mock"}); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	a.openDetail(id)
	tt.Frame()
	if err := tt.Click("Summary"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && !tt.HasText("Generate"); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if err := tt.Click("Generate"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("the summary request never went out: %q", tt.Texts())
	}

	for i := 0; i < 40 && !tt.HasText("Cancel"); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if !tt.HasText("Cancel") {
		t.Fatalf("the window offers no way to stop the job: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		job, ok := a.Jobs.Latest(id)
		if ok && !job.Running() {
			if job.State != jobs.StateCancelled {
				t.Errorf("the job ended as %q: %+v", job.State, job)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the job did not stop: %+v", job)
		}
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}

	for i := 0; i < 20 && tt.HasText("Cancel"); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if tt.HasText("Cancel") {
		t.Errorf("the window still offers to cancel: %q", tt.Texts())
	}
}

// Importing a file runs the offline transcription as a job of its own, so that
// the window shows it, can stop it, and hears why it failed.
func TestImportChainRecordsWhatItDid(t *testing.T) {
	a, id := newTestApp(t)
	a.startImport(id, false, "en")

	deadline := time.Now().Add(5 * time.Second)
	for {
		job, ok := a.Jobs.Latest(id)
		if ok && !job.Running() {
			if job.Kind != jobs.KindImport {
				t.Errorf("the import job is of kind %q", job.Kind)
			}
			// The test app has no native ASR runtime, so this is the honest
			// end of the chain rather than a fabricated one.
			if job.State != jobs.StateFailed || !strings.Contains(job.Error, "sherpa-onnx") {
				t.Errorf("the import job ended as %q: %+v", job.State, job)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import job did not end: %+v", job)
		}
		time.Sleep(5 * time.Millisecond)
	}

	m, err := a.Lib.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if m.TranscriptStatus == "pending" {
		t.Errorf("the media is left pending after a failed import: %+v", m)
	}
}

// Recordings the library does not hold are reported rather than silently left
// on disk, and deleting them asks first.
func TestOrphanNoticeAndDelete(t *testing.T) {
	a, _ := newTestApp(t)
	orphan := filepath.Join(a.Rec.Dir, "left-behind.mp4")
	if err := os.WriteFile(orphan, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.reload()

	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	if !tt.HasText("1 recording file is not in the library.") {
		t.Fatalf("the window says %q", tt.Texts())
	}

	if err := tt.Click("Delete files…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Delete 1 recording files?") {
		t.Fatalf("the window does not ask first: %q", tt.Texts())
	}
	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if _, err := os.Stat(orphan); os.IsNotExist(err) {
			break
		}
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("the orphan file is still there: %v", err)
	}
	for i := 0; i < 20 && tt.HasText("1 recording file is not in the library."); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if tt.HasText("1 recording file is not in the library.") {
		t.Errorf("the notice outlived the files: %q", tt.Texts())
	}
}

// Settings lists what is installed, shows what cannot be used, and choosing a
// model is what transcription uses from then on.
func TestSettingsChooseAModel(t *testing.T) {
	a, _ := newTestApp(t)
	base := a.MM.ModelsDir()
	for _, name := range []string{"aaa-model", "zzz-model"} {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"tokens.txt", "model.int8.onnx"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("dummy"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(base, "not-a-model"), 0o755); err != nil {
		t.Fatal(err)
	}

	tt := settingsTester(a, 0, WideWidth, WideHeight)
	tt.Frame()
	if err := tt.Click("Transcription"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && !tt.HasText("SenseVoice"); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if !tt.HasText("SenseVoice") {
		t.Fatalf("the installed models are not listed: %q", tt.Texts())
	}
	// A folder that is not a supported model is not offered at all; the three
	// families are, whether or not they are on disk.
	if tt.HasText("not-a-model") {
		t.Errorf("an unsupported folder is listed: %q", tt.Texts())
	}
	if !tt.HasText("FunASR Nano") || !tt.HasText("Qwen3 ASR 0.6B") || !tt.HasText("Download") {
		t.Errorf("the models that could be downloaded are missing: %q", tt.Texts())
	}
	// Without a choice the first valid model is the one in use; the other is
	// offered.
	if a.modelInUse != filepath.Join(base, "aaa-model") {
		t.Fatalf("in use: %q", a.modelInUse)
	}
	if err := tt.Click("Use this model"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.MM.SelectedDir() != filepath.Join(base, "zzz-model") {
		if time.Now().After(deadline) {
			t.Fatalf("the choice was not recorded: %q", a.MM.SelectedDir())
		}
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	for i := 0; i < 40 && a.modelInUse != filepath.Join(base, "zzz-model"); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if a.modelInUse != filepath.Join(base, "zzz-model") {
		t.Errorf("transcription still uses %q", a.modelInUse)
	}
}

// What can be recorded is laid out as cards in the sidebar, and windows, which
// are too many for it, are chosen on a page of their own after a search.
func TestSourceCardsAndPicker(t *testing.T) {
	a, _ := newTestApp(t)
	a.sources = []capture.Source{
		{ID: "screen", Kind: "screen", Name: "Full screen"},
		{ID: "display:0", Kind: "display", Name: "Display 1"},
		{ID: "display:1", Kind: "display", Name: "Display 2"},
		{ID: "window:Alpha — Editor", Kind: "window", Name: "Alpha — Editor", Detail: "1560x992"},
		{ID: "window:Beta", Kind: "window", Name: "Beta", Detail: "800x600"},
	}
	a.pickSource()
	a.speakers = []capture.AudioDevice{{ID: "speaker-1", Name: "Speakers 1"}, {ID: "speaker-2", Name: "Speakers 2"}}
	a.speaker = a.speakers[0]
	a.microphones = []capture.AudioDevice{{ID: "mic-1", Name: "Microphone 1"}}
	if a.sourceID != "display:0" {
		t.Fatalf("the first display is not chosen by default: %q", a.sourceID)
	}

	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	if !tt.HasText("New capture") || !tt.HasText("System audio") {
		t.Fatalf("the capture panel says %q", tt.Texts())
	}
	if !tt.HasText("Full screen") || !tt.HasText("Window") || !tt.HasText("Microphone") {
		t.Fatalf("the cards are not there: %q", tt.Texts())
	}
	// Area and Camera are on the canvas but not built: they are shown as
	// disabled cards marked SOON, and choosing them records nothing else.
	if !tt.HasText("SOON") || tt.HasText("Camera only") || tt.HasText("Not yet") {
		t.Errorf("unsupported source cards are not marked as coming: %q", tt.Texts())
	}
	before := a.sourceID
	_ = tt.Click("Area")
	tt.Frame()
	if a.sourceID != before {
		t.Errorf("the Area card changed the source to %q", a.sourceID)
	}
	if err := tt.Click("System audio device"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("Speakers 2"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.speaker.ID != "speaker-2" {
		t.Errorf("the dropdown chose %q", a.speaker.ID)
	}

	// The Window card switches the dropdown to window selection.
	if err := tt.Click("Window"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Choose a window"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("Alpha — Editor") || !tt.HasText("Beta") {
		t.Fatalf("the windows are not listed: %q", tt.Texts())
	}
	if err := tt.Click("Beta"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.sourceID != "window:Beta" {
		t.Errorf("the dropdown chose source %q", a.sourceID)
	}

	// Back on the form, the dropdown says what is recorded.
	tt.Frame()
	if !tt.HasText("Beta") {
		t.Errorf("the dropdown does not say what is chosen: %q", tt.Texts())
	}
	if err := tt.Click("Full screen"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Choose a display"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("Display 2"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.sourceID != "display:1" {
		t.Errorf("the display dropdown chose source %q", a.sourceID)
	}
}

// A recording takes the whole window over, in the item page's shell: the stage
// with the live picture, the clock and the two controls, the facts, and the live
// transcript beside it. The library comes back when the recording stops.
func TestRecordingTakesTheWindow(t *testing.T) {
	a, _ := newTestApp(t)
	a.recording = true
	a.recName = "Recording 2026-11-11 13.27.04.mp4"
	a.startedAt = time.Now().Add(-23 * time.Second)
	tt := ui.NewTester(func(c *ui.Context) { a.frame(c, a.applyTheme(c)) }, WideWidth, WideHeight)
	tt.Frame()
	if _, ok := tt.Find("Recording preview"); !ok {
		t.Fatalf("the recording page has no live picture: %q", tt.Texts())
	}
	if tt.HasText("All media") {
		t.Error("the library is still drawn beside the recording")
	}
	for _, want := range []string{a.recName, "Recording", "Pause", "Stop", "LIVE TRANSCRIPT", "System audio"} {
		if !tt.HasText(want) {
			t.Errorf("the recording page has no %q: %q", want, tt.Texts())
		}
	}
}

func TestCompactRecordingShowsTimerOnce(t *testing.T) {
	a, _ := newTestApp(t)
	a.recording = true
	a.startedAt = time.Now().Add(-23 * time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		t := a.applyTheme(c)
		ui.Column(c).Fill().Children(func() {
			a.titleBar(c, t)
			a.recorderPage(c, t)
		})
	}, CompactWidth, CompactHeight)
	tt.Frame()
	// The clock is shown once, on the recording page. Nothing else repeats it:
	// the title bar no longer carries a REC pill.
	count := 0
	for _, text := range tt.Texts() {
		if strings.HasSuffix(text, "0:23") {
			count++
			if text != "00:00:23" {
				t.Errorf("the clock reads %q, not 00:00:23", text)
			}
		}
	}
	if count != 1 {
		t.Fatalf("recording timer appears %d times: %q", count, tt.Texts())
	}
	preview, ok := tt.Find("Recording preview")
	if !ok || preview.H < 150 || preview.H > 260 {
		t.Fatalf("compact preview did not keep a useful aspect-sized height: %+v", preview)
	}
}

// The window draws its own title bar, in the strip the window controls sit in,
// and keeps its name clear of the room they take at the right.
func TestTitleBarKeepsClearOfTheWindowControls(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.view, 1100, 720)
	tt.SetTitleBar(ui.TitleBar{Height: TitleBarHeight, Right: 138})
	tt.Frame()

	name, ok := tt.Find("Ottr")
	if !ok {
		t.Fatalf("the title bar has no name: %q", tt.Texts())
	}
	if name.Y+name.H > TitleBarHeight {
		t.Errorf("the name is at y=%.0f, below the title bar", name.Y)
	}
	if right := name.X + name.W; right > 1100-138 {
		t.Errorf("the name ends at x=%.0f, under the window controls", right)
	}
}

func TestSecondaryWindowsUseImmersiveTitleBars(t *testing.T) {
	a, _ := newTestApp(t)
	a.settingsOpen = true
	for name, view := range map[string]func(*ui.Context){"Settings": a.view} {
		tt := ui.NewTester(view, WideWidth, WideHeight)
		tt.SetTitleBar(ui.TitleBar{Height: TitleBarHeight, Right: 138})
		tt.Frame()
		title, ok := tt.Find("Ottr")
		if !ok || title.Y+title.H > TitleBarHeight {
			t.Errorf("%s has no immersive title: %+v", name, title)
		}
		if title.X+title.W > WideWidth-138 {
			t.Errorf("%s title runs under the window controls", name)
		}
	}
}

// The wide window shows the sidebar, the library and the capture panel
// together; narrower windows show one page, and the library button switches
// between the recorder and the library. A row opens its page in the same
// window, and back returns to the library.
func TestMainWindowLayouts(t *testing.T) {
	a, _ := newTestApp(t)
	a.sources = []capture.Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}
	a.pickSource()

	tt := ui.NewTester(a.view, WideWidth, WideHeight)
	tt.Frame()
	for _, want := range []string{"All media", "New capture", "standup", "Start recording", "Appearance"} {
		if !tt.HasText(want) {
			t.Fatalf("the wide window lacks %q: %q", want, tt.Texts())
		}
	}

	tt.SetSize(CompactWidth, CompactHeight)
	tt.Frame()
	if !tt.HasText("New capture") || tt.HasText("standup") {
		t.Fatalf("the narrow window should show the recorder alone: %q", tt.Texts())
	}
	if err := tt.Click("Library"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("standup") || tt.HasText("New capture") {
		t.Fatalf("the library button did not show the library: %q", tt.Texts())
	}

	tt.SetSize(900, 620)
	a.libraryOpen = false
	tt.Frame()
	// The recorder at 900 lists the last recording under it, but not the
	// library itself.
	if !tt.HasText("New capture") || !tt.HasText("Last recording") && !tt.HasText("LAST RECORDING") || tt.HasText("All media") {
		t.Fatalf("the 900px window should show the recorder: %q", tt.Texts())
	}
	if err := tt.Click("Library"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("standup") || !tt.HasText("All media") {
		t.Fatalf("the rail's library button did not show the library: %q", tt.Texts())
	}

	tt.SetSize(WideWidth, WideHeight)
	id := a.media[0].ID
	a.openDetail(id)
	// The library row already quotes the first line, so wait for the page's
	// own way back rather than for the text.
	for i := 0; i < 20; i++ {
		if _, ok := tt.Find("Back to library"); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if !tt.HasText("Transcript") || !tt.HasText("first line") {
		t.Fatalf("opening a row did not show its page: %q", tt.Texts())
	}
	if err := tt.Click("Back to library"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, onPage := tt.Find("Back to library"); onPage || !tt.HasText("All media") || !tt.HasText("standup") {
		t.Errorf("back did not return to the library: %q", tt.Texts())
	}
}

func TestRecordingSettingsTab(t *testing.T) {
	a, _ := newTestApp(t)
	a.recording = true
	a.startedAt = time.Now()
	tt := ui.NewTester(func(c *ui.Context) {
		a.recorderPage(c, a.applyTheme(c))
	}, CompactWidth, CompactHeight)
	tt.Frame()
	// The live transcript's settings are two selects on the page itself: what it
	// listens for and what it is translated into, without a menu between.
	if !tt.HasText("Spoken language") || !tt.HasText("Translate into") {
		t.Fatalf("the recording page has no live transcript settings: %q", tt.Texts())
	}
	if err := tt.Click("Spoken language"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("English") {
		t.Fatalf("the spoken language select offers %q", tt.Texts())
	}
	if err := tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if a.defaultLanguage != "en" {
		t.Errorf("choosing English left the spoken language %q", a.defaultLanguage)
	}
	if err := tt.Click("Translate into"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("Español") {
		t.Errorf("the translation targets stop at %q", tt.Texts())
	}
}

func TestDefaultFont(t *testing.T) {
	a := &App{}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, a.applyTheme(c).Font)
	}, 200, 60)
	tt.Frame()
	if !tt.HasText("Geist") {
		t.Fatalf("default font = %q", tt.Texts())
	}
}

func TestInterfaceLanguageAndFont(t *testing.T) {
	a, _ := newTestApp(t)
	a.setInterfaceLanguage("zh")
	a.font = "Microsoft YaHei"
	tt := settingsTester(a, 0, WideWidth, WideHeight)
	tt.Frame()
	for _, text := range []string{"界面语言", "显示字体", "跟随系统", "浅色", "深色"} {
		if !tt.HasText(text) {
			t.Fatalf("Chinese appearance settings do not show %q: %q", text, tt.Texts())
		}
	}
	if got := a.tr("Library"); got != "媒体库" {
		t.Fatalf("Library translation = %q", got)
	}
	a.setInterfaceLanguage("en")
	if got := a.tr("Library"); got != "Library" {
		t.Fatalf("English translation = %q", got)
	}
}

// The look can be fixed to light or dark, or left to the desktop.
func TestAppearanceChangesTheLook(t *testing.T) {
	a, _ := newTestApp(t)
	tt := settingsTester(a, 0, WideWidth, WideHeight)
	tt.Frame()
	for _, look := range []string{"System", "Light", "Dark"} {
		if !tt.HasText(look) {
			t.Fatalf("the looks to pick are not there: %q", tt.Texts())
		}
	}

	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	if a.theme != "dark" {
		t.Errorf("the app kept the look %q", a.theme)
	}
}

// The two switches the recording page shows read in the interface's own words:
// the spoken language falls back to "自动" in Chinese, and a translation that is
// off names the switch, not a language nobody is seeing.
func TestRecordingSwitchLabelsInChinese(t *testing.T) {
	a := &App{language: "zh"}
	if got := a.tr("Auto"); got != "自动" {
		t.Errorf("the spoken language fallback reads %q, want 自动", got)
	}
	if got := a.trf("%s off", a.tr("Translate")); got != "翻译已关闭" {
		t.Errorf("a translation turned off reads %q, want 翻译已关闭", got)
	}
	if got := a.trf("%s on", langName("zh")); got != "中文已开启" {
		t.Errorf("a translation turned on reads %q, want 中文已开启", got)
	}
}

// A launch without a model asks for one, in a page of its own: the three
// models with their download, and Later, which records without a transcript
// rather than asking again every frame.
func TestOnboardingAsksForAModelOnce(t *testing.T) {
	a, _ := newTestApp(t)
	a.onboarding, a.modelLoaded = true, true
	a.modelList = []models.Offer{{
		ID: "sensevoice", Name: "SenseVoice", Detail: "Fast, four languages", DownloadSize: 240 << 20,
	}}
	tt := ui.NewTester(func(c *ui.Context) { a.frame(c, a.applyTheme(c)) }, WideWidth, WideHeight)
	tt.Frame()
	for _, want := range []string{"Set up transcription", "SenseVoice", "Download", "Later"} {
		if !tt.HasText(want) {
			t.Fatalf("the first-run page has no %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("All media") {
		t.Error("the library is drawn behind the first-run page")
	}
	if err := tt.Click("Later"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.onboarding || !a.onboardLater {
		t.Errorf("Later left onboarding=%v later=%v", a.onboarding, a.onboardLater)
	}
	if tt.HasText("Set up transcription") {
		t.Errorf("the first-run page is still drawn: %q", tt.Texts())
	}
}
