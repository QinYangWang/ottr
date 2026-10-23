// Ottr: offline screen recording, transcription and summaries.
package main

import (
	"log"
	"path/filepath"

	"github.com/egoist/mygo"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
	"ottr/internal/summary"
	"ottr/internal/translate"
	"ottr/ui/native"
)

func main() {
	mygo.App.SetName("Ottr")

	lib := &library.Service{}
	mm := models.NewManager("", "")
	asrSvc := asr.NewService(lib, mm)
	trSvc := translate.NewService(lib, asrSvc)
	sumSvc := summary.NewService(lib, asrSvc)
	rec := &capture.Recorder{Lib: lib, ASR: asrSvc}
	rec.Live = func(rate, channels int) (capture.LiveWriter, error) {
		return asrSvc.StartLive(rate, channels, func(sub asr.LiveSubtitle) {
			rec.SetLiveText(sub.Text)
			asr.BroadcastLive(sub)
		})
	}

	app := &native.App{Lib: lib, Rec: rec, ASR: asrSvc, Trans: trSvc, Sum: sumSvc}

	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatalf("user data dir: %v", err)
		}
		mm.SetDirs(filepath.Join(dir, "models"), filepath.Join(dir, "bin"))
		db, err := storage.Open(filepath.Join(dir, "ottr.db"))
		if err != nil {
			log.Fatalf("open database: %v", err)
		}
		lib.Open(db)
		asrSvc.Open(db)
		trSvc.Open(db)
		sumSvc.Open(db)
		rec.Dir = filepath.Join(dir, "recordings")

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Ottr",
			Width:           1180,
			Height:          760,
			MinWidth:        860,
			MinHeight:       560,
			BackgroundColor: "#101014",
			Content:         native.View(app),
		})
		app.Begin(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
