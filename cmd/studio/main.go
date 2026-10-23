// Ottr: offline screen recording, transcription and summaries.
package main

import (
	"log"
	"net/http"
	"path/filepath"

	"github.com/egoist/mygo"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
	"ottr/internal/translate"
	"ottr/ui/web"
)

func main() {
	mygo.App.SetName("Ottr")
	mygo.SetFrontend(web.FS)

	svc := &library.Service{}
	mm := models.NewManager("", "")
	asrSvc := asr.NewService(svc, mm)
	trSvc := translate.NewService(svc, asrSvc)
	rec := &capture.Recorder{Lib: svc, ASR: asrSvc}
	mygo.BindAs("Library", svc)
	mygo.BindAs("Recorder", rec)
	mygo.BindAs("Transcription", asrSvc)
	mygo.BindAs("Translation", trSvc)
	mygo.BindAs("Models", mm)

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
		svc.Open(db)
		asrSvc.Open(db)
		trSvc.Open(db)
		rec.Dir = filepath.Join(dir, "recordings")
		libH := library.MediaHandler(svc)
		prevH := capture.PreviewHandler(rec)
		err = mygo.Protocol.HandleFunc("media", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/preview" {
				prevH(w, r)
				return
			}
			libH(w, r)
		})
		if err != nil {
			log.Fatalf("media protocol: %v", err)
		}
		mygo.NewWindow(mygo.WindowOptions{
			Title:           "Ottr",
			URL:             "/",
			Width:           1200,
			Height:          780,
			MinWidth:        880,
			MinHeight:       560,
			BackgroundColor: "#101014",
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
