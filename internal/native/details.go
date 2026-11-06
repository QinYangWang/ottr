package native

import (
	"path/filepath"
	"strings"
	"time"

	"ottr/internal/jobs"
	"ottr/internal/library"
)

// recorderDetails is what the details card under the recorder says: where
// recordings go, how much room is left there, and which model transcribes.
// Each is read off the main thread: the volume and the models folder are
// disk work.
type recorderDetails struct {
	loaded  bool
	free    uint64
	freeErr string
	model   string
	ready   bool   // native ASR can run
	why     string // why it cannot, when it cannot
}

// loadDetails reads the details card's facts in the background.
func (a *App) loadDetails() {
	a.details.loaded = true
	rec, asrSvc, mm := a.Rec, a.ASR, a.MM
	go func() {
		var d recorderDetails
		d.loaded = true
		if rec != nil && rec.Dir != "" {
			if free, err := rec.FreeSpace(); err != nil {
				d.freeErr = err.Error()
			} else {
				d.free = free
			}
		}
		if mm != nil {
			d.model = mm.CurrentName()
		}
		if asrSvc != nil {
			rt := asrSvc.RuntimeStatus()
			d.ready, d.why = rt.OK(), rt.Err
		}
		a.update(func() { a.details = d })
	}()
}

func (a *App) recordingsDir() string {
	if a.Rec == nil {
		return ""
	}
	return a.Rec.Dir
}

// shortDir names a folder by its last two parts, the way the design writes
// it: "Ottr › recordings".
func shortDir(dir string) string {
	if dir == "" {
		return "—"
	}
	dir = filepath.Clean(dir)
	parts := strings.FieldsFunc(dir, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, " › ")
}

// dayGroup is the heading a recording is listed under: Today, Yesterday, or
// its date.
func (a *App) dayGroup(now, at time.Time) string {
	now, at = now.Local(), at.Local()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := at.Date()
	today := time.Date(y1, m1, d1, 0, 0, 0, 0, now.Location())
	day := time.Date(y2, m2, d2, 0, 0, 0, 0, now.Location())
	switch {
	case day.Equal(today):
		return a.tr("Today")
	case day.Equal(today.AddDate(0, 0, -1)):
		return a.tr("Yesterday")
	}
	return a.date(at)
}

// groupLabel is a list heading as it is shown: capitals in English.
func (a *App) groupLabel(group string) string {
	if a.language == "zh" {
		return group
	}
	return strings.ToUpper(group)
}

// failedNote is the red part of a failed recording's line: where its
// transcript stopped, when that is known.
func (a *App) failedNote(m library.Media) string {
	if at, ok := stoppedAt(a.lastJobs[m.ID], m); ok {
		return a.trf("transcript stopped at %s", fmtDuration(at))
	}
	return a.tr("transcription failed")
}

// loadLastJobs reads what became of the last attempt on each failed media,
// for the list's red notes.
func (a *App) loadLastJobs() {
	a.lastJobs = nil
	if a.Jobs == nil {
		return
	}
	for _, m := range a.media {
		if m.TranscriptStatus != "error" {
			continue
		}
		if job, ok := a.Jobs.Latest(m.ID); ok {
			if a.lastJobs == nil {
				a.lastJobs = map[int64]jobs.Job{}
			}
			a.lastJobs[m.ID] = job
		}
	}
}
