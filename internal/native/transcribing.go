package native

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/library"
)

// stepState is where one step of a transcription stands.
type stepState int

const (
	stepQueued stepState = iota
	stepActive
	stepDone
)

// transcribeStep is one line of the steps card: what happens, and where it
// stands.
type transcribeStep struct {
	label string
	state stepState
}

// transcribeSteps reads the phases of a transcription out of the progress its
// job reported (asr.Phase*). The recording is on disk before the job starts.
func transcribeSteps(p float64) []transcribeStep {
	steps := []transcribeStep{
		{"Saved to disk", stepDone},
		{"Audio extracted", stepQueued},
		{"Speech to text", stepQueued},
		{"Matching speakers", stepQueued},
		{"Saving the transcript", stepQueued},
	}
	bounds := []float64{asr.PhaseRecognizing, asr.PhaseSpeakers, asr.PhaseSaving, 1}
	active := false
	for i, bound := range bounds {
		switch {
		case p >= bound:
			steps[i+1].state = stepDone
		case !active:
			steps[i+1].state = stepActive
			active = true
		}
	}
	return steps
}

// speechClock is when recognition was first seen moving, so that the time
// left can be estimated from how fast it moves since.
type speechClock struct {
	job int64
	at  time.Time
	p   float64
}

// observe notes the progress of a running job the first time it is inside
// recognition.
func (s *speechClock) observe(job jobs.Job, now time.Time) {
	if s.job == job.ID || job.Progress < asr.PhaseRecognizing || job.Progress >= asr.PhaseSpeakers {
		return
	}
	*s = speechClock{job: job.ID, at: now, p: job.Progress}
}

// left estimates how long recognition still needs, from the rate it has
// moved at since it was first seen. ok is false until there is a rate.
func (s speechClock) left(job jobs.Job, now time.Time) (time.Duration, bool) {
	if s.job != job.ID || job.Progress >= asr.PhaseSpeakers {
		return 0, false
	}
	moved, spent := job.Progress-s.p, now.Sub(s.at)
	if moved < 0.01 || spent < time.Second {
		return 0, false
	}
	rest := (asr.PhaseSpeakers - job.Progress) / moved
	return time.Duration(rest * float64(spent)), true
}

// transcribingPanel is the transcript tab while a recording is transcribed:
// how far it has come, the steps it goes through, and the way to stop it.
func (a *App) transcribingPanel(c *ui.Context, t *ui.Theme, m library.Media) {
	job := a.job
	now := time.Now()
	a.speech.observe(job, now)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(t.Space(3)).Children(func() {
			card := ui.Column(c).FillWidth().Padding(t.Space(3.5)).Gap(t.Space(1.5)).
				Radius(t.Radius+4).Border(1, t.Border).Background(t.Background)
			card.Children(func() {
				ui.Text(c, a.tr("Transcribing your recording")).Bold()
				model := a.modelInUseName()
				detail := a.tr("Runs on this computer. You can keep using Ottr while it works.")
				if model != "" {
					detail = a.trf("Runs on this computer with %s. You can keep using Ottr while it works.", model)
				}
				ui.Text(c, detail).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
				value := job.Progress
				if !job.Running() || value <= 0 {
					value = -1
				}
				ui.Progress(c, value).FillWidth()
				ui.Row(c).Gap(t.Space(2)).Children(func() {
					ui.Text(c, a.processedText(job, m)).FontSize(t.FontSize - 3).
						TextColor(t.TextMuted).FontFeatures("tnum").Grow(1)
					if left, ok := a.speech.left(job, now); ok {
						ui.Text(c, a.trf("about %s left", fmtLeft(left))).FontSize(t.FontSize - 3).
							TextColor(t.TextMuted).FontFeatures("tnum")
					}
				})
			})
			steps := ui.Column(c).FillWidth().Padding(t.Space(2), t.Space(3.5)).
				Radius(t.Radius+4).Border(1, t.Border).Background(t.Background)
			steps.Children(func() {
				for i, step := range transcribeSteps(job.Progress) {
					a.stepRow(c, t, i, step, job, m)
				}
			})
			ui.Text(c, a.tr("The transcript opens here when it is done.")).
				FontSize(t.FontSize - 3).TextColor(t.TextMuted)
			if job.Running() {
				if ui.Button(c, a.tr("Cancel")).Label(a.tr("Stop this job")).Clicked() {
					a.cancelJob()
				}
			}
		})
	})
}

// stepRow is one step: its state as an icon, its name, and a detail.
func (a *App) stepRow(c *ui.Context, t *ui.Theme, i int, step transcribeStep, job jobs.Job, m library.Media) {
	ui.Row(c.Key(i)).Gap(t.Space(2.5)).Padding(t.Space(2), 0).AlignItems(ui.Center).Children(func() {
		mark := ui.Column(c).Size(22, 22).Radius(11).Center().Shrink(0)
		mark.Children(func() {
			switch step.state {
			case stepDone:
				mark.Background(t.Success.Alpha(0.16))
				ui.Icon(c, iconCheck).FontSize(t.FontSize - 1).TextColor(t.Success)
			case stepActive:
				mark.Background(t.Accent.Alpha(0.14))
				ui.Spinner(c).Label(a.tr(step.label)).Size(14, 14)
			default:
				mark.Background(t.Surface)
				ui.Icon(c, iconClock).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			}
		})
		label := ui.Text(c, a.tr(step.label)).FontSize(t.FontSize - 1).Bold().Grow(1)
		if step.state == stepQueued {
			label.TextColor(t.TextMuted)
		}
		detail := ""
		switch {
		case i == 0:
			detail = durationOrDash(m)
			if m.Size > 0 {
				detail += " · " + fmtSize(m.Size)
			}
		case step.state == stepQueued:
			detail = a.tr("queued")
		case step.label == "Speech to text" && step.state == stepActive:
			detail = a.trf("%d%%", int(asr.RecognizedShare(job.Progress)*100))
		}
		if detail != "" {
			ui.Text(c, detail).FontSize(t.FontSize - 3).TextColor(t.TextMuted).FontFeatures("tnum")
		}
	})
}

// processedText says how much of the recording has been read: in recording
// time once recognition moves, else the job's own share.
func (a *App) processedText(job jobs.Job, m library.Media) string {
	share := asr.RecognizedShare(job.Progress)
	if share > 0 && m.DurationMs > 0 {
		done := int64(share * float64(m.DurationMs))
		return a.trf("%s of %s processed", fmtDuration(done), fmtDuration(m.DurationMs))
	}
	return a.trf("%d%% done", int(job.Progress*100))
}

// stoppedAt is where in the recording a failed transcription stopped, when it
// stopped inside recognition.
func stoppedAt(job jobs.Job, m library.Media) (int64, bool) {
	if job.Kind != jobs.KindTranscribe || job.State != jobs.StateFailed || m.DurationMs <= 0 {
		return 0, false
	}
	share := asr.RecognizedShare(job.Progress)
	if share <= 0 || share >= 1 {
		return 0, false
	}
	return int64(share * float64(m.DurationMs)), true
}

// fmtLeft is a short estimate: seconds under a minute, else minutes.
func fmtLeft(d time.Duration) string {
	if d < time.Minute {
		return fmtSeconds(int(max(d.Round(5*time.Second).Seconds(), 5)))
	}
	return fmtMinutes(int(d.Round(time.Minute).Minutes()))
}

func fmtSeconds(s int) string { return strconv.Itoa(s) + " s" }
func fmtMinutes(m int) string { return strconv.Itoa(m) + " min" }

// modelInUseName is the transcription model's display name, read once and
// kept for the window: finding it walks the models folder.
func (a *App) modelInUseName() string {
	if !a.details.loaded {
		a.loadDetails()
	}
	return a.details.model
}

// autoTranscribe starts the offline transcription of a recording that just
// stopped, so the window can move from recording to its transcript. What
// keeps it from starting is said in the transcript tab, not as an error.
func (a *App) autoTranscribe(m library.Media) {
	a.trErr = ""
	if a.ASR == nil || !m.HasAudio {
		return
	}
	if _, err := a.ASR.Transcribe(m.ID); err != nil {
		a.trErr = err.Error()
	}
}
