// Ottr: offline screen recording, transcription and summaries.
package main

import (
	"log"
	"path/filepath"

	"github.com/egoist/mygo"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/settings"
	"ottr/internal/storage"
	"ottr/internal/summary"
	"ottr/internal/tools"
	"ottr/internal/translate"
	"ottr/ui/native"
)

func main() {
	mygo.App.SetName("Ottr")

	lib := &library.Service{}
	prefs := &settings.Service{}
	mm := models.NewManager("", "")
	js := &jobs.Service{}
	asrSvc := asr.NewService(lib, mm, js)
	trSvc := translate.NewService(lib, asrSvc, js)
	sumSvc := summary.NewService(lib, asrSvc, js)
	// One capture backend serves the three products: recording, the idle
	// preview and screenshots.
	backend := capture.NewBackend()
	rec := &capture.Recorder{Backend: backend, Lib: lib, ASR: asrSvc}
	rec.Live = func(rate, channels int) (capture.LiveWriter, error) {
		return asrSvc.StartLive(rate, channels, func(sub asr.LiveSubtitle) {
			// The window polls the recorder, so State carries the same
			// revision the event does.
			line := &capture.LiveLine{Utterance: sub.Utterance, StartMs: sub.StartMs, Text: sub.Text, Final: sub.Final}
			if sub.Warn != "" {
				line.Warn = sub.Warn
			}
			if sub.Error != "" {
				line.Text, line.Warn = "", sub.Error
			}
			rec.SetLiveText(line)
			asr.BroadcastLive(sub)
		})
	}

	app := &native.App{
		Lib: lib, Rec: rec, ASR: asrSvc, Trans: trSvc, Sum: sumSvc, Jobs: js, MM: mm,
		Preview: capture.NewPreviewer(backend), // the picture of a source, before recording
		Shot: &capture.Snapshotter{Backend: backend, Lib: lib},
		Prefs:   prefs,
	}

	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatalf("user data dir: %v", err)
		}
		mm.SetDirs(filepath.Join(dir, "models"), filepath.Join(dir, "bin"))
		tools.SetBinDir(filepath.Join(dir, "bin"))
		db, err := storage.Open(filepath.Join(dir, "ottr.db"))
		if err != nil {
			log.Fatalf("open database: %v", err)
		}
		lib.Open(db)
		asrSvc.Open(db)
		trSvc.Open(db)
		sumSvc.Open(db)
		js.Open(db)
		mm.Open(db)
		prefs.Open(db)
		rec.Dir = filepath.Join(dir, "recordings")
		app.Shot.Dir = filepath.Join(dir, "recordings")

		// The encoder is FFmpeg until internal/media replaces it; the path
		// setting outlives the FFmpeg screens it came from.
		tools.SetChosen(prefs.Get(settings.FFmpegPath))

		// The look chosen last, before the window exists, so that it starts in
		// it.
		mygo.Theme.SetSource(mygo.ThemeSource(prefs.Get(settings.Theme)))
		background := "#101014"
		if !mygo.Theme.IsDark() {
			background = "#f6f6f8"
		}

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Ottr",
			// The view draws its own title bar in the strip the window
			// controls sit in (ui/native.titleBar).
			TitleBarStyle:   mygo.TitleBarHidden,
			TitleBarHeight:  46,
			Width:           native.CompactWidth,
			Height:          native.CompactHeight,
			MinWidth:        380,
			MinHeight:       560,
			BackgroundColor: background,
			Content:         native.View(app),
		})
		app.Begin(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
