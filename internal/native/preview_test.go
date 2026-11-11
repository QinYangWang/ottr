package native

import (
	"context"
	"database/sql"
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

// TestPreview draws the window to PNG files named as docs/design/screens is,
// so that each can be laid beside the Doop frame it mirrors. It is off unless
// OTTR_PREVIEW names the folder to write them to.
func TestPreview(t *testing.T) {
	dir := os.Getenv("OTTR_PREVIEW")
	if dir == "" {
		t.Skip("set OTTR_PREVIEW=<folder> to write screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A machine without CJK fonts (a Linux container) draws 中文 as nothing;
	// OTTR_PREVIEW_CJK names one to register, as Windows has YaHei.
	if cjk := os.Getenv("OTTR_PREVIEW_CJK"); cjk != "" {
		data, err := os.ReadFile(cjk)
		if err != nil {
			t.Fatal(err)
		}
		if err := ui.RegisterFont(data, ""); err != nil {
			t.Fatal(err)
		}
	}
	a, ids, db := previewApp(t)
	sync := ids["Weekly product sync"]

	shoot := func(name string, view func(*ui.Context), w, h int, dark bool) {
		t.Helper()
		tt := ui.NewTester(view, w, h)
		tt.SetTitleBar(ui.TitleBar{Height: TitleBarHeight, Right: 138})
		tt.SetDark(dark)
		for i := 0; i < 14; i++ {
			tt.Advance(80 * time.Millisecond)
			tt.Frame()
			time.Sleep(3 * time.Millisecond)
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

	// The recorder and its library.
	a.closeDetail()
	shoot("01-recorder", a.view, WideWidth, WideHeight, false)
	shoot("02-recorder-dark", a.view, WideWidth, WideHeight, true)

	// A recording under way, with live text and a translated final line.
	a.recording, a.startedAt = true, time.Now().Add(-12*time.Minute-48*time.Second)
	a.recName = "Recording 2026-11-11 13.27.04.mp4"
	a.late = []liveLine{
		{at: 731000, utterance: 1, text: "So the export dialog is the last thing blocking 0.6.1.", final: true},
		{at: 735000, utterance: 2, text: "I'll take it and pair with you tomorrow morning.", final: true},
		{at: 744000, utterance: 3, text: "Keep SRT and plain text, and drop the", final: false},
	}
	a.liveTranslate, a.trLang = true, "zh"
	a.liveTrans = map[int64]liveTranslation{
		1: {text: a.late[0].text, translated: "所以导出对话框是 0.6.1 最后的阻碍。"},
		2: {text: a.late[1].text, translated: "我来做，明天上午和你结对。"},
	}
	// The recorder is not running, so the view must not mirror it.
	still := func(c *ui.Context) { a.frame(c, a.applyTheme(c)) }
	shoot("03-recording", still, WideWidth, WideHeight, false)
	shoot("04-recording-dark", still, WideWidth, WideHeight, true)
	shoot("19b-recording-900", still, 900, 620, false)
	shoot("20b-recording-420", still, CompactWidth, CompactHeight, false)
	a.recording, a.late, a.liveTrans = false, nil, nil
	a.recName = ""

	// Transcribing after Stop: a job in recognition, the live lines kept.
	pending := ids["Bug triage"]
	if _, err := db.Exec(`UPDATE media SET transcript_status = 'pending' WHERE id = ?`, pending); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	if _, err := a.Jobs.Run(pending, jobs.KindTranscribe, func(ctx context.Context, progress func(float64)) error {
		progress(asr.PhaseRecognizing + 0.64*(asr.PhaseSpeakers-asr.PhaseRecognizing))
		<-release
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	a.provisional = []liveLine{
		{at: 731000, utterance: 1, text: "So the export dialog is the last thing blocking 0.6.1.", final: true},
		{at: 735000, utterance: 2, text: "I'll take it and pair with you tomorrow morning.", final: true},
	}
	a.provisionalID = pending
	a.reload()
	a.openDetail(pending)
	shoot("05-transcribing", a.view, WideWidth, WideHeight, false)
	close(release)
	a.closeDetail()

	// Playback of the sync: the line at the playhead, the chapters, 中文.
	open := func() {
		a.closeDetail()
		a.reload()
		a.openDetail(sync)
		a.trLang = "zh"
		a.loadTranscript()
		a.loadSummary()
		for i := 0; i < 200 && (!a.trLoaded || !a.sumLoaded); i++ {
			time.Sleep(3 * time.Millisecond)
			a.drain()
		}
		a.trans = translate.Translation{}
		for _, seg := range a.segs {
			if zh, ok := zhLines[seg.Text]; ok {
				a.trans.Segments = append(a.trans.Segments, translate.TranslationSegment{SegmentID: seg.ID, TranslatedText: zh})
			}
		}
		a.transLang, a.showTranslation = "zh", true
		a.playback = playback{id: sync, paused: true, offset: 12*time.Minute + 11*time.Second, seekSeconds: 731}
	}
	open()
	shoot("06-playback", a.view, WideWidth, WideHeight, false)
	shoot("07-playback-dark", a.view, WideWidth, WideHeight, true)
	a.tab = tabSummary
	shoot("08-summary", a.view, WideWidth, WideHeight, false)
	a.tab = tabTranscript
	a.segSearchOpen, a.segQuery, a.segHit = true, "export", 1
	shoot("09-search", a.view, WideWidth, WideHeight, false)
	a.segSearchOpen, a.segQuery = false, ""
	shoot("21-playback-900", a.view, 900, 620, false)
	shoot("22-playback-420", a.view, CompactWidth, CompactHeight, false)

	// The dialogs, over the page they open from.
	a.askDelete(sync)
	shoot("16-dialog-delete", a.view, WideWidth, WideHeight, false)
	a.confirmDel = false
	m, _ := a.current()
	a.openExport(m)
	shoot("17-dialog-export", a.view, WideWidth, WideHeight, false)
	a.exportOpen = false
	a.failedJob = jobs.Job{Kind: jobs.KindTranscribe, State: jobs.StateFailed, MediaID: sync,
		Progress: asr.PhaseRecognizing + 0.62*(asr.PhaseSpeakers-asr.PhaseRecognizing),
		Error:    "sherpa-onnx: failed to allocate 412 MB for the encoder"}
	a.failedOpen = true
	shoot("18-dialog-failed", a.view, WideWidth, WideHeight, false)
	a.failedOpen = false

	// The library as a grid, then the responsive recorder.
	a.closeDetail()
	a.libraryGridView = true
	shoot("10-library-grid", a.view, WideWidth, WideHeight, false)
	a.libraryGridView = false
	shoot("19-recorder-900", a.view, 900, 620, false)
	shoot("20-recorder-420", a.view, CompactWidth, CompactHeight, false)

	// Settings, in the main window.
	a.loadedProvider = true
	a.provider = providerSettings{
		BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", APIKey: "sk-preview",
		TranslatePrompt: translate.DefaultPrompt, SummaryPrompt: summary.DefaultPrompt,
	}
	for i, name := range []string{"12-settings-appearance", "13-settings-transcription", "14-settings-provider", "15-settings-speakers"} {
		a.settingsTab = i
		a.settingsOpen = true
		shoot(name, a.view, WideWidth, WideHeight, false)
	}
	a.settingsOpen = false

	// The first run: nothing in the library.
	empty, _ := newTestApp(t)
	for _, m := range empty.media {
		if err := empty.Lib.Remove(m.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	empty.reload()
	empty.sources = a.sources
	empty.pickSource()
	empty.speakers, empty.speaker = a.speakers, a.speaker
	empty.preview, empty.previewW, empty.previewH = a.preview, a.previewW, a.previewH
	shoot("11-library-empty", empty.view, WideWidth, WideHeight, false)
}

// zhLines are the 中文 lines of the preview's sync.
var zhLines = map[string]string{
	"Fixed on Tuesday. It shows once now, under the grid, with Add back.":    "周二修好了。现在只在网格下方显示一次，带“添加回来”。",
	"So the export dialog is the last thing blocking 0.6.1.":                 "所以导出对话框是 0.6.1 最后的阻碍。",
	"I'll take it and pair with you tomorrow morning.":                       "我来做，明天上午和你结对。",
	"Keep SRT and plain text, and drop the Word option for now.":             "保留 SRT 和纯文本，Word 选项先去掉。",
	"Then the translation toggle goes into the same dialog?":                 "那翻译开关也放进同一个对话框？",
	"Yes, one switch: include the 中文 lines under each subtitle.":             "对，一个开关：在每条字幕下附上中文。",
	"I'll update the export screenshots in docs/design once it's in.":        "合进去后我来更新 docs/design 里的导出截图。",
	"Then we cut 0.6.1 on Friday, after the smoke test.":                     "然后周五冒烟测试后发布 0.6.1。",
	"The recorder work is done; Area capture stays disabled with a tooltip.": "录制部分完成了；区域录制保持禁用并带提示。",
}

// previewApp is an app over a library like the canvas's: a sync with named
// speakers, a summary and a translation, and the other rows of the list.
func previewApp(t *testing.T) (*App, map[string]int64, *sql.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
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

	for _, name := range []string{"sherpa-onnx-sense-voice-zh-en-ja-ko-yue", "funasr-nano-int8", "half-downloaded"} {
		modelDir := filepath.Join(modelsDir, name)
		if err := os.MkdirAll(modelDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if name == "half-downloaded" {
			continue
		}
		for _, f := range []string{"tokens.txt", "model.int8.onnx"} {
			if err := os.WriteFile(filepath.Join(modelDir, f), make([]byte, 5<<20), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	now := time.Now().UTC()
	ids := map[string]int64{}
	add := func(name, source, status, summaryStatus string, minutes, ago int) int64 {
		res, err := db.Exec(`INSERT INTO media (name, source, path, duration_ms, width, height, size, has_audio, has_video, created_at, transcript_status, summary_status)
			VALUES (?, ?, ?, ?, 1920, 1080, ?, 1, 1, ?, ?, ?)`,
			name, source, filepath.Join(t.TempDir(), "Videos", "Ottr", name+".mp4"), minutes*60000, int64(minutes)*10<<20,
			now.Add(-time.Duration(ago)*time.Minute).Format(time.RFC3339), status, summaryStatus)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		ids[name] = id
		return id
	}
	add("Interview: backend engineer", "recording", "error", "none", 58, 60*26)
	add("GopherCon keynote", "import", "done", "done", 47, 60*25)
	add("Bug triage", "recording", "none", "none", 17, 90)
	add("Lumen Studio — customer call", "recording", "done", "none", 31, 60)
	sync := add("Weekly product sync", "recording", "done", "done", 42, 30)

	if _, err := db.Exec(`INSERT INTO job (media_id, kind, state, progress, error, created_at, updated_at)
		VALUES (?, 'transcribe', 'failed', ?, 'out of memory', ?, ?)`, ids["Interview: backend engineer"],
		asr.PhaseRecognizing+0.62*(asr.PhaseSpeakers-asr.PhaseRecognizing), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Logf("job fixture: %v", err)
	}

	speakers := []string{"Ana Ruiz", "Chen Ligan", "Speaker 3", "Jonas Keller"}
	for _, name := range speakers {
		if _, err := db.Exec(`INSERT INTO speaker (name, embedding, created_at) VALUES (?, x'00000000', ?)`,
			name, now.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	lines := []struct {
		speaker int
		at      int
		text    string
	}{
		{2, 722, "Fixed on Tuesday. It shows once now, under the grid, with Add back."},
		{1, 729, "So the export dialog is the last thing blocking 0.6.1."},
		{0, 735, "I'll take it and pair with you tomorrow morning."},
		{1, 744, "Keep SRT and plain text, and drop the Word option for now."},
		{0, 751, "Then the translation toggle goes into the same dialog?"},
		{1, 760, "Yes, one switch: include the 中文 lines under each subtitle."},
		{2, 772, "I'll update the export screenshots in docs/design once it's in."},
		{3, 790, "Then we cut 0.6.1 on Friday, after the smoke test."},
	}
	for _, id := range []int64{sync, ids["Lumen Studio — customer call"], ids["GopherCon keynote"]} {
		tr, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'en', ?)`, id, now.Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
		trID, _ := tr.LastInsertId()
		for i, l := range lines {
			if _, err := db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text, speaker)
				VALUES (?, ?, ?, ?, ?, ?)`, trID, i+1, l.at*1000, l.at*1000+6000, l.text, l.speaker+1); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`INSERT INTO summary (media_id, overview, content_markdown, model, created_at, key_points_json, action_items_json, chapters_json, highlights_json)
		VALUES (?, ?, '# Weekly product sync', 'deepseek-chat', ?, ?, ?, ?, ?)`, sync,
		"The team went through what is left for Ottr 0.6.1. Recorder and library work is done, and the export dialog is the last blocker. Chen owns it, pairing with Ana. The release is cut on Friday after the smoke test.",
		now.Format(time.RFC3339),
		`["Recorder work is done; Area capture stays disabled with a tooltip.","The orphan-files banner now shows once, under the grid.","Export ships with SRT and plain text. Word waits until after 0.6.1.","中文 lines become one switch in the same export dialog."]`,
		`["Build the export dialog","Pair on the export review","Update export screenshots in docs/design","Smoke test and cut 0.6.1"]`,
		`[{"startMs":0,"title":"Release status"},{"startMs":691000,"title":"What is left for 0.6.1"},{"startMs":1450000,"title":"Export dialog scope"},{"startMs":2028000,"title":"Docs and release day"}]`,
		`[{"startMs":729000,"note":"So the export dialog is the last thing blocking 0.6.1."}]`,
	); err != nil {
		t.Logf("summary fixture: %v", err)
	}

	a := &App{Lib: lib, Rec: &capture.Recorder{Lib: lib, Dir: filepath.Join(t.TempDir(), "Videos", "Ottr")},
		ASR: asrSvc, Trans: trSvc, Sum: sumSvc, Jobs: js, MM: mm}
	a.sources = []capture.Source{
		{ID: "display:0", Kind: "display", Name: "Display 1", Detail: "2560x1440"},
		{ID: "display:1", Kind: "display", Name: "Display 2", Detail: "1920x1080"},
		{ID: "window:Ottr — Design", Kind: "window", Name: "Ottr — Design", Detail: "1560x992"},
	}
	a.pickSource()
	a.speakers = []capture.AudioDevice{{ID: "speakers", Name: "Speakers (Realtek Audio)"}}
	a.microphones = []capture.AudioDevice{{ID: "microphone", Name: "Microphone (USB Audio)"}}
	a.speaker = a.speakers[0]
	a.preview = ui.NewBitmap(deskPicture())
	a.previewW, a.previewH = 1280, 720
	// Posters of their own: the files are not there to decode.
	a.thumbs = map[int64]*ui.Bitmap{}
	for name, id := range ids {
		if name == "Weekly product sync" || name == "Lumen Studio — customer call" {
			a.thumbs[id] = ui.NewBitmap(callPicture())
		} else {
			a.thumbs[id] = ui.NewBitmap(deskPicture())
		}
	}
	a.reload()
	a.trLang = "zh"
	return a, ids, db
}

// deskPicture is a stand-in for a recorded screen: a dark desk with a light
// document window and a title bar.
func deskPicture() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	fill := func(x0, y0, x1, y1 int, c color.RGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.Set(x, y, c)
			}
		}
	}
	fill(0, 0, 640, 360, color.RGBA{0x1B, 0x1E, 0x22, 255})
	fill(60, 40, 580, 320, color.RGBA{0xFF, 0xFF, 0xFF, 255})
	fill(60, 40, 580, 64, color.RGBA{0xE8, 0xEB, 0xEF, 255})
	fill(60, 64, 170, 320, color.RGBA{0xF3, 0xF5, 0xF7, 255})
	for i, w := range []int{180, 300, 260, 280, 200} {
		c := color.RGBA{0xD5, 0xDA, 0xE0, 255}
		if i == 0 {
			c = color.RGBA{0x9A, 0xA3, 0xAE, 255}
		}
		fill(196, 92+i*28, 196+w, 102+i*28, c)
	}
	return img
}

// callPicture is a stand-in for a video call: four tiles.
func callPicture() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	tiles := []color.RGBA{{0x2A, 0x33, 0x40, 255}, {0x28, 0x33, 0x2F, 255}, {0x36, 0x29, 0x3A, 255}, {0x2C, 0x2A, 0x3A, 255}}
	faces := []color.RGBA{{0x3E, 0x63, 0xDD, 255}, {0x12, 0xA5, 0x94, 255}, {0xD6, 0x40, 0x9F, 255}, {0x8E, 0x4E, 0xC6, 255}}
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{0x1B, 0x1E, 0x22, 255})
		}
	}
	for i := 0; i < 4; i++ {
		x0, y0 := 28+(i%2)*300, 20+(i/2)*166
		for y := y0; y < y0+154; y++ {
			for x := x0; x < x0+284; x++ {
				img.Set(x, y, tiles[i])
				dx, dy := x-(x0+142), y-(y0+72)
				if dx*dx+dy*dy < 34*34 {
					img.Set(x, y, faces[i])
				}
			}
		}
	}
	return img
}
