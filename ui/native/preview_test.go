package native

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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

// TestPreview draws the window to PNG files for looking at by hand. It is off
// unless OTTR_PREVIEW names the folder to write them to.
func TestPreview(t *testing.T) {
	dir := os.Getenv("OTTR_PREVIEW")
	if dir == "" {
		t.Skip("set OTTR_PREVIEW=<folder> to write screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := storage.Open(filepath.Join(t.TempDir(), "preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lib := &library.Service{}
	lib.Open(db)
	modelsDir := filepath.Join(t.TempDir(), "models")
	mm := models.NewManager(modelsDir, filepath.Join(t.TempDir(), "bin"))
	mm.Open(db)
	js := &jobs.Service{}
	js.Open(db)
	asrSvc := asr.NewService(lib, mm, js)
	trSvc, sumSvc := translate.NewService(lib, asrSvc, js), summary.NewService(lib, asrSvc, js)
	asrSvc.Open(db)
	trSvc.Open(db)
	sumSvc.Open(db)

	// Two models on disk, one of them broken.
	for _, name := range []string{"sherpa-onnx-streaming-zipformer-zh-14M", "zipformer-en-20M"} {
		modelDir := filepath.Join(modelsDir, name)
		if err := os.MkdirAll(modelDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"tokens.txt", "encoder.int8.onnx", "decoder.int8.onnx", "joiner.int8.onnx"} {
			if err := os.WriteFile(filepath.Join(modelDir, f), make([]byte, 5<<20), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(modelsDir, "half-downloaded"), 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	add := func(name, status, summaryStatus string, minutes int) int64 {
		res, err := db.Exec(`INSERT INTO media (name, source, path, duration_ms, width, height, has_audio, has_video, created_at, transcript_status, summary_status)
			VALUES (?, 'recording', '/tmp/x.mp4', ?, 1920, 1080, 1, 1, ?, ?, ?)`,
			name, minutes*60000, now.Add(-time.Duration(minutes)*time.Minute).Format(time.RFC3339), status, summaryStatus)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	add("Weekly product sync", "done", "done", 42)
	add("Design review — onboarding", "done", "none", 28)
	add("Bug triage", "none", "none", 17)
	add("Interview: backend engineer", "error", "none", 96)
	id := add("Recorded screen 2026-10-24", "done", "none", 3)

	tr, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'en', ?)`, id, now.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	trID, _ := tr.LastInsertId()
	lines := []string{
		"Alright, let's start with the release plan.",
		"We ship the editor on Thursday, and the recorder follows next week.",
		"The one thing blocking us is the export dialog.",
		"I'll take that, and pair with Ana tomorrow morning.",
		"Then we cut 0.6.1 on Friday.",
	}
	for i, text := range lines {
		if _, err := db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text)
			VALUES (?, ?, ?, ?, ?)`, trID, i+1, i*4200, i*4200+3900, text); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{Lib: lib, Rec: &capture.Recorder{Lib: lib, Dir: t.TempDir()}, ASR: asrSvc, Trans: trSvc, Sum: sumSvc, Jobs: js, MM: mm}
	a.sources = []capture.Source{
		{ID: "screen", Kind: "screen", Name: "Full screen"},
		{ID: "display:0", Kind: "display", Name: "Display 1", Detail: "1920x1080"},
		{ID: "display:1", Kind: "display", Name: "Display 2", Detail: "2560x1440"},
		{ID: "window:π - ottr", Kind: "window", Name: "π - ottr", Detail: "1560x992"},
		{ID: "window:主页 / X", Kind: "window", Name: "主页 / X", Detail: "1560x992"},
		{ID: "window:设置", Kind: "window", Name: "设置", Detail: "980x700"},
	}
	a.pickSource()
	a.speakers = []capture.AudioDevice{{ID: "speakers", Name: "Speakers (Realtek Audio)"}}
	a.microphones = []capture.AudioDevice{{ID: "microphone", Name: "Microphone (USB Audio)"}}
	a.speaker, a.microphone = a.speakers[0], a.microphones[0]
	a.reload()
	a.selectMedia(id)
	a.trLang = "en"

	tt := ui.NewTester(a.view, WideWidth, WideHeight)
	// The room Windows' minimize, maximize and close take, at the right.
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	shot := func(name string) {
		for i := 0; i < 12; i++ {
			tt.Advance(80 * time.Millisecond)
			tt.Frame()
			time.Sleep(4 * time.Millisecond)
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}

	// The window as it opens: the recording form alone, drawing the desk.
	tt.SetSize(CompactWidth, CompactHeight)
	a.preview = ui.NewBitmap(deskPicture())
	a.previewW, a.previewH = 640, 360
	shot("00-compact")
	a.preview = nil
	a.previewErr = "ffmpeg not found: put ffmpeg beside sherpa-onnx in C:/Users/oukinyou/AppData/Roaming/Ottr/bin, or on the PATH"
	shot("11-preview-error")
	a.previewErr = ""

	mainTT := tt
	tt = ui.NewTester(a.libraryView, WideWidth, WideHeight)
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	a.preview = ui.NewBitmap(deskPicture())
	a.previewW, a.previewH = 640, 360

	// The grid, then one media's page in place of it.
	a.closeDetail()
	shot("12-library-grid")
	a.openDetail(id)
	shot("01-session")

	// The recording form in place of the grid.
	a.chooseRecording()
	shot("13-library-new-recording")
	a.stopChoosing()

	tt = ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	a.settingsTab = 0
	shot("02-settings-appearance")
	a.settingsTab = 1
	shot("03-settings-transcription")
	a.settingsTab = 2
	a.loadedTrans = true
	shot("04-settings-translation")
	tt = mainTT
	a.tab = 0
	shot("06-sidebar")

	// A window chosen, the microphone off, and the window narrower.
	a.chooseSource("window:π - ottr")
	a.microphone = capture.AudioDevice{}
	a.tab = 0
	tt.SetSize(900, 620)
	shot("07-narrow-window")

	tt.SetSize(WideWidth, WideHeight)
	tt.SetDark(true)
	shot("08-dark")

	// The media's page in the dark look.
	a.openDetail(id)
	lt := ui.NewTester(a.libraryView, WideWidth, WideHeight)
	lt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	lt.SetDark(true)
	for i := 0; i < 12; i++ {
		lt.Advance(80 * time.Millisecond)
		lt.Frame()
	}
	f, err := os.Create(filepath.Join(dir, "14-session-dark.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, lt.Image()); err != nil {
		t.Fatal(err)
	}

	tt.SetDark(false)
	tt = ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	a.settingsTab = 3
	a.loadedSum = true
	shot("09-settings-summary")
}

// deskPicture is a stand-in for the frame the previewer draws: something with
// edges, so that the panel's framing shows.
func deskPicture() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			shade := uint8(28 + (x+y)/14)
			img.Set(x, y, color.RGBA{R: shade / 3, G: shade / 2, B: shade, A: 255})
		}
	}
	// A lighter "window" in the middle, and a bright title bar.
	for y := 60; y < 300; y++ {
		for x := 120; x < 520; x++ {
			img.Set(x, y, color.RGBA{R: 44, G: 48, B: 58, A: 255})
		}
	}
	for y := 60; y < 82; y++ {
		for x := 120; x < 520; x++ {
			img.Set(x, y, color.RGBA{R: 72, G: 78, B: 92, A: 255})
		}
	}
	return img
}

// The recording page and the page shown before anything is chosen, for looking
// at while working on them.
func TestPreviewRecording(t *testing.T) {
	dir := os.Getenv("OTTR_PREVIEW")
	if dir == "" {
		t.Skip("set OTTR_PREVIEW=<folder> to write screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a, _ := newTestApp(t)
	a.sources = []capture.Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}
	a.pickSource()
	a.forget() // nothing chosen: the empty page

	tt := ui.NewTester(a.libraryView, 1180, 760)
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	shot := func(name string) {
		for i := 0; i < 6; i++ {
			tt.Advance(80 * time.Millisecond)
			tt.Frame()
			time.Sleep(4 * time.Millisecond)
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	shot("10-empty")
}

// The title bar as it looks while a recording runs, which the window shows
// only when the recorder says it is recording.
func TestPreviewTitleBar(t *testing.T) {
	dir := os.Getenv("OTTR_PREVIEW")
	if dir == "" {
		t.Skip("set OTTR_PREVIEW=<folder> to write screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a, _ := newTestApp(t)
	a.recording, a.startedAt = true, time.Now().Add(-3*time.Minute-9*time.Second)

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { a.titleBar(c, c.Theme()) })
	}, 1180, 120)
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	for i := 0; i < 6; i++ {
		tt.Advance(80 * time.Millisecond)
		tt.Frame()
	}
	f, err := os.Create(filepath.Join(dir, "10-titlebar-recording.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
