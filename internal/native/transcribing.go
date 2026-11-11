package native

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/native/kit"
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
// the pipeline card — transcription with its steps and progress, then what
// the provider can do after it — Cancel, and the live lines of the recording
// until the full pass replaces them.
func (a *App) transcribingPanel(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	job := a.job
	now := time.Now()
	a.speech.observe(job, now)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(16, 20, 20, 20).Gap(14).Children(func() {
			ui.Column(c).FillWidth().Padding(14, 16).Gap(12).Radius(kit.RadiusPanel).Border(1, p.Line).
				Background(p.Sheet).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, a.tr("Processing on this computer")).FontSize(13.5).FontWeight(600).TextColor(p.Ink)
						ui.Text(c, a.tr("Transcribing your recording")).FontSize(11.5).TextColor(p.Ink3)
					})
					if job.Running() {
						if kit.Button(c, t, kit.Secondary, nil, a.tr("Cancel"), false).Label(a.tr("Stop this job")).Clicked() {
							a.cancelJob()
						}
					}
				})
				model := a.modelInUseName()
				stage1 := a.tr("Transcribing")
				if model != "" {
					stage1 = a.trf("Transcribing with %s", shortModel(model))
				}
				a.pipelineStage(c, t, 1, stage1, a.trf("%d%%", int(job.Progress*100)), true, func() {
					value := job.Progress
					if !job.Running() || value <= 0 {
						value = -1
					}
					progressBar(c, t, value, 0)
					ui.Row(c).FillWidth().Gap(6).Children(func() {
						line := "CPU · " + a.processedText(job, m)
						if left, ok := a.speech.left(job, now); ok {
							line += " · " + a.trf("about %s left", fmtLeft(left))
						}
						mono(ui.Text(c, line)).FontSize(11).TextColor(p.Ink3).SingleLine()
					})
					ui.Column(c).FillWidth().Padding(4, 0, 0, 0).Children(func() {
						for i, step := range transcribeSteps(job.Progress) {
							a.stepRow(c, t, i, step, job, m)
						}
					})
				})
				a.pipelineStage(c, t, 2, a.trf("Translate into %s", langName(a.trLang)),
					a.tr("On request · online provider"), false, nil)
				a.pipelineStage(c, t, 3, a.tr("Summarize"), a.tr("On request"), false, nil)
			})
			ui.Text(c, a.tr("The transcript opens here when it is done.")).FontSize(11.5).TextColor(p.Ink3)
			if a.provisionalID == m.ID && len(a.provisional) > 0 {
				a.liveNote(c, t, a.tr("Live transcript shown until the full pass replaces it."))
				a.provisionalLines(c, t, true)
			}
		})
	})
}

// pipelineStage is one numbered stage of the pipeline card; the active one has
// an accent number and its details under it.
func (a *App) pipelineStage(c *ui.Context, t *ui.Theme, n int, label, note string, active bool, details func()) {
	p := pal(t)
	ui.Row(c.Key(n)).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
		num := ui.Column(c).Size(22, 22).Radius(kit.RadiusControl).Center().Shrink(0)
		ink := p.Ink3
		if active {
			num.Background(p.Accent)
			ink = p.BtnInk
		} else {
			num.Background(p.Hover)
		}
		num.Children(func() { mono(ui.Text(c, strconv.Itoa(n))).FontSize(11.5).FontWeight(600).TextColor(ink) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Padding(2, 0, 0, 0).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				text := ui.Text(c, label).FontSize(13).SingleLine().Grow(1).MinWidth(0)
				if active {
					text.TextColor(p.Ink)
				} else {
					text.TextColor(p.Ink3)
				}
				noteText := ui.Text(c, note).FontSize(11.5).SingleLine().Shrink(0)
				if active {
					mono(noteText).TextColor(p.Ink)
				} else {
					noteText.TextColor(p.Ink3)
				}
			})
			if details != nil {
				details()
			}
		})
	})
}

// stepRow is one step of the transcription: its state as a mark, its name,
// and a detail.
func (a *App) stepRow(c *ui.Context, t *ui.Theme, i int, step transcribeStep, job jobs.Job, m library.Media) {
	p := pal(t)
	ui.Row(c.Key(i)).FillWidth().Gap(8).Padding(3, 0).AlignItems(ui.Center).Children(func() {
		mark := ui.Column(c).Size(16, 16).Radius(8).Center().Shrink(0)
		mark.Children(func() {
			switch step.state {
			case stepDone:
				mark.Background(p.GreenSoft)
				ui.Icon(c, iconCheck).FontSize(11).TextColor(p.Green)
			case stepActive:
				mark.Background(p.AccentSoft)
				ui.Box(c).Size(6, 6).Radius(3).Background(p.Accent)
			default:
				mark.Background(p.Hover)
			}
		})
		label := ui.Text(c, a.tr(step.label)).FontSize(12).Grow(1).SingleLine()
		if step.state == stepQueued {
			label.TextColor(p.Ink3)
		} else {
			label.TextColor(p.Ink2)
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
			mono(ui.Text(c, detail)).FontSize(11).TextColor(p.Ink3)
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

// liveNote is the dashed band that says where the lines under it came from.
func (a *App) liveNote(c *ui.Context, t *ui.Theme, text string) {
	p := pal(t)
	ui.Row(c).FillWidth().Padding(8, 12).Gap(8).AlignItems(ui.Center).Radius(kit.RadiusCard).
		BorderStyle(ui.BorderDashed).Border(1, p.Line2).Background(p.Sunk).Children(func() {
		ui.Icon(c, iconCaptions).FontSize(14).TextColor(p.Ink3)
		ui.Text(c, text).FontSize(12).TextColor(p.Ink2)
	})
}

// provisionalLines are the lines the live pass heard while the recording was
// written. They carry no speaker until the full pass runs.
func (a *App) provisionalLines(c *ui.Context, t *ui.Theme, dimmed bool) {
	p := pal(t)
	for _, line := range a.provisional {
		if !line.final {
			continue
		}
		row := ui.Row(c.Key(line.utterance)).FillWidth().Gap(12).AlignItems(ui.Start)
		if dimmed {
			row.Opacity(0.75)
		}
		row.Children(func() {
			kit.UnknownAvatar(c, t, 24)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, a.tr("Unknown speaker")).FontSize(12).FontWeight(600).TextColor(p.Ink3)
					mono(ui.Text(c, fmtDuration(line.at))).FontSize(11.5).TextColor(p.Ink3)
				})
				ui.Text(c, line.text).FontSize(13).TextColor(p.Ink2)
			})
		})
	}
}

// liveTranscriptTab is the transcript a recording already has when it stops:
// the lines heard live, with the full pass one click away.
func (a *App) liveTranscriptTab(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(12, 20, 16, 20).Gap(12).Children(func() {
				a.liveNote(c, t, a.tr("Transcribed live while recording. The full pass adds speakers and fixes mistakes."))
				a.provisionalLines(c, t, false)
			})
		})
		ui.Row(c).FillWidth().Shrink(0).Padding(12, 20).Gap(8).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
			ui.Text(c, a.tr("No speakers yet.")).FontSize(11.5).TextColor(p.Ink3).Grow(1)
			if kit.Button(c, t, kit.Secondary, iconRefresh, a.tr("Transcribe again"), false).Clicked() {
				a.startTranscribe(m.ID)
			}
		})
	})
}
