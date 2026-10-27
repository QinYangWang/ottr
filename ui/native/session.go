package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/translate"
)

var langs = []struct{ code, name string }{
	{"en", "English"},
	{"zh", "Chinese"},
	{"ja", "Japanese"},
}

func langName(code string) string {
	for _, l := range langs {
		if l.code == code {
			return l.name
		}
	}
	return code
}

// langChoices are the spoken languages the user can set, in the order the
// select shows them: what was detected first, then the ones this app knows.
func langChoices() (names, codes []string) {
	names = append(names, "Detected")
	codes = append(codes, asr.DefaultLanguage)
	for _, l := range langs {
		names = append(names, l.name)
		codes = append(codes, l.code)
	}
	return names, codes
}

func langLabel(code string) string {
	if code == "" || code == asr.DefaultLanguage {
		return "Detected"
	}
	return langName(code)
}

// sessionPage is what the app made of the media chosen: its transcript, its
// translation, and its summary, beside the media itself.
func (a *App) sessionPage(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).Fill().Children(func() {
		a.sessionHead(c, t, m)
		a.tabs(c, t, m)
	})
}

func (a *App) sessionHead(c *ui.Context, t *ui.Theme, m library.Media) {
	detail := []string{m.Source, fmtDuration(m.DurationMs), m.CreatedAt.Local().Format("Jan 2 15:04")}
	if m.Width > 0 {
		detail = append(detail, fmt.Sprintf("%dx%d", m.Width, m.Height))
	}
	ui.Row(c).Padding(t.Space(4), t.Space(5)).Gap(t.Space(3)).AlignItems(ui.Center).
		Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, m.Name).FontSize(t.FontSize + 4).Bold().SingleLine()
			ui.Text(c, strings.Join(detail, "  ·  ")).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		})
		if ui.Button(c, "Play").Label("Play the media").Tooltip("Open the media in its player").
			Clicked() {
			mygo.Shell.OpenPath(m.Path)
		}
		if ui.Button(c, "Show file").Clicked() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		if ui.Button(c, "Delete…").Clicked() {
			a.delID, a.confirmDel = m.ID, true
		}
		if ui.Button(c, "Close").Tooltip("Close media details").Clicked() {
			a.forget()
		}
	})
}

func (a *App) tabs(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		ui.Row(c).Padding(t.Space(4), t.Space(5), 0, t.Space(5)).Children(func() {
			ui.Tabs(c, &a.tab, "Transcript", "Translate", "Summary")
		})
		ui.Column(c).Grow(1).MinWidth(0).Padding(t.Space(4), t.Space(5)).Children(func() {
			switch a.tab {
			case 1:
				a.translateTab(c, t, m)
			case 2:
				a.summaryTab(c, t, m)
			default:
				a.transcriptTab(c, t, m)
			}
		})
	})
}

// ------------------------------------------------------------- transcript

func (a *App) transcriptTab(c *ui.Context, t *ui.Theme, m library.Media) {
	if m.TranscriptStatus == "pending" {
		a.working(c, t, "Transcribing", "Recognizing the speech in this file on this computer.")
		return
	}
	if m.TranscriptStatus != "done" {
		detail := "Offline speech recognition turns the audio into timestamped subtitles."
		if a.job.Kind == jobs.KindTranscribe && a.job.State == jobs.StateFailed && a.job.Error != "" {
			detail = a.job.Error
		}
		a.emptyTab(c, t, "No transcript yet", detail,
			[]action{{"Transcribe", m.HasAudio, func() { a.startTranscribe(m.ID) }}})
		return
	}
	if !a.trLoaded {
		a.loadTranscript()
		a.working(c, t, "Loading", "Reading the transcript.")
		return
	}
	if len(a.segs) == 0 {
		a.emptyTab(c, t, "The transcript is empty",
			"No speech was recognized in this file.",
			[]action{{"Transcribe again", true, func() { a.startTranscribe(m.ID) }}})
		return
	}
	ui.Column(c).Fill().Gap(t.Space(2)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.SearchField(c, &a.segQuery).Label("Search the subtitles").Grow(1)
			if ui.Button(c, "SRT").Label("Export as SRT").Clicked() {
				a.exportTranscript(m.ID, "srt")
			}
			if ui.Button(c, "VTT").Label("Export as WebVTT").Clicked() {
				a.exportTranscript(m.ID, "vtt")
			}
			if ui.Button(c, "TXT").Label("Export as text").Clicked() {
				a.exportTranscript(m.ID, "txt")
			}
			if ui.Button(c, "Transcribe again").Clicked() {
				a.startTranscribe(m.ID)
			}
		})
		a.spokenLanguageRow(c, t, m)
		rows := a.filteredSegments()
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				if len(rows) == 0 {
					ui.Text(c, "Nothing matches that search.").TextColor(t.TextMuted)
				}
				for _, seg := range rows {
					line := ui.Row(c.Key(seg.ID)).Gap(t.Space(3)).Padding(t.Space(2), t.Space(3)).
						Radius(t.Radius)
					if line.Hovered() {
						line.Background(t.SurfaceHover)
					}
					line.Children(func() {
						ui.Text(c, fmtDuration(seg.StartMs)).FontSize(t.FontSize - 2).
							TextColor(t.TextMuted).FontFeatures("tnum").Width(56)
						ui.Text(c, seg.Text).Grow(1).Selectable()
					})
				}
			})
		})
	})
}

// spokenLanguageRow says what language the subtitles are in, which
// translation takes as its source, and lets the user overrule the guess.
func (a *App) spokenLanguageRow(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Spoken language").FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		names, codes := langChoices()
		lang := langLabel(a.lang)
		ui.Select(c, &lang, names).Label("Spoken language").Width(130).Changed()
		for i, name := range names {
			if name == lang && codes[i] != a.lang {
				a.setLanguage(m.ID, codes[i])
			}
		}
		ui.Spacer(c)
	})
}

func (a *App) filteredSegments() []asr.Segment {
	q := strings.TrimSpace(strings.ToLower(a.segQuery))
	if q == "" {
		return a.segs
	}
	out := make([]asr.Segment, 0, len(a.segs))
	for _, s := range a.segs {
		if strings.Contains(strings.ToLower(s.Text), q) {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) startTranscribe(id int64) {
	if _, err := a.ASR.Transcribe(id); err != nil {
		a.setError(err)
		return
	}
	a.reload()
}

// setLanguage says what language a transcript is in, which translation takes
// as its source.
func (a *App) setLanguage(id int64, code string) {
	a.lang = code
	go func() {
		if err := a.ASR.SetLanguage(id, code); err != nil {
			a.update(func() { a.setError(err) })
		}
	}()
}

func (a *App) exportTranscript(id int64, format string) {
	go func() {
		_, err := a.ASR.Export(context.Background(), id, format)
		if err != nil {
			a.update(func() { a.setError(err) })
		}
	}()
}

// -------------------------------------------------------------- translate

func (a *App) translateTab(c *ui.Context, t *ui.Theme, m library.Media) {
	if m.TranscriptStatus != "done" {
		a.emptyTab(c, t, "A transcript comes first",
			"Translate the subtitles of this file once it has been transcribed.",
			[]action{{"Go to Transcript", true, func() { a.tab = 0 }}})
		return
	}
	if a.transLang != a.trLang {
		a.transLang = a.trLang
		a.loadTranslation()
	}
	if a.jobRunning(jobs.KindTranslate) {
		a.working(c, t, "Translating", "Sending the subtitles to the provider.")
		return
	}
	has := len(a.trans.Segments) > 0
	ui.Column(c).Fill().Gap(t.Space(2)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			name := langName(a.trLang)
			ui.Select(c, &name, langNames()).Label("Translate into").Width(160).Changed()
			for _, l := range langs {
				if l.name == name {
					if l.code != a.trLang {
						a.trLang = l.code
						a.trans = translate.Translation{}
						a.loadTranslation()
					}
				}
			}
			ui.Spacer(c)
			if has {
				if ui.Button(c, "SRT").Label("Export as SRT").Clicked() {
					a.exportTranslation(m.ID, "srt")
				}
				if ui.Button(c, "Bilingual").Label("Export as bilingual SRT").Clicked() {
					a.exportTranslation(m.ID, "bilingual_srt")
				}
				if ui.Button(c, "VTT").Label("Export as WebVTT").Clicked() {
					a.exportTranslation(m.ID, "vtt")
				}
				if ui.Button(c, "TXT").Label("Export as text").Clicked() {
					a.exportTranslation(m.ID, "txt")
				}
			}
		})
		if !has {
			a.emptyTab(c, t, "Not translated yet",
				"Translate the subtitles into "+langName(a.trLang)+" with the online provider.",
				[]action{{"Translate", true, func() { a.startTranslate(m.ID) }}})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				for _, seg := range a.trans.Segments {
					ui.Row(c.Key(seg.ID)).Gap(t.Space(3)).Padding(t.Space(2), t.Space(3)).Children(func() {
						ui.Text(c, fmtDuration(seg.StartMs)).FontSize(t.FontSize - 2).
							TextColor(t.TextMuted).FontFeatures("tnum").Width(56)
						ui.Column(c).Grow(1).Gap(1).Children(func() {
							ui.Text(c, seg.TranslatedText)
							ui.Text(c, seg.OriginalText).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
						})
					})
				}
			})
		})
	})
}

func langNames() []string {
	out := make([]string, len(langs))
	for i, l := range langs {
		out[i] = l.name
	}
	return out
}

func (a *App) loadTranslation() {
	id, lang := a.sel, a.trLang
	go func() {
		tr, err := a.Trans.Get(id, lang)
		a.update(func() {
			if id != a.sel || lang != a.trLang {
				return
			}
			if err != nil {
				a.setError(err)
				return
			}
			a.trans = tr
		})
	}()
}

func (a *App) startTranslate(id int64) {
	if _, err := a.Trans.StartTranslate(id, a.trLang); err != nil {
		a.setError(err)
	}
}

func (a *App) exportTranslation(id int64, format string) {
	lang := a.trLang
	go func() {
		_, err := a.Trans.Export(context.Background(), id, lang, format)
		if err != nil {
			a.update(func() { a.setError(err) })
		}
	}()
}

// ---------------------------------------------------------------- summary

func (a *App) summaryTab(c *ui.Context, t *ui.Theme, m library.Media) {
	if m.TranscriptStatus != "done" {
		a.emptyTab(c, t, "A transcript comes first",
			"Summarize this file once it has been transcribed.",
			[]action{{"Go to Transcript", true, func() { a.tab = 0 }}})
		return
	}
	if m.SummaryStatus == "pending" || a.jobRunning(jobs.KindSummarize) {
		a.working(c, t, "Summarizing", "Asking the AI model for the overview, key points and chapters.")
		return
	}
	if !a.sumLoaded {
		a.loadSummary()
		a.working(c, t, "Loading", "Reading the summary.")
		return
	}
	has := a.sum.ContentMarkdown != ""
	ui.Column(c).Fill().Gap(t.Space(2)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Model").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			ui.Text(c, modelName(a.sum.Model, m.SummaryStatus)).FontSize(t.FontSize - 2).Grow(1)
			if has {
				if ui.Button(c, "Copy").Clicked() {
					c.Services().WriteClipboard(a.sum.ContentMarkdown)
					c.Toast("Summary copied as Markdown")
				}
				if ui.Button(c, "Export…").Clicked() {
					a.exportSummary(m.ID)
				}
			}
		})
		if !has {
			a.emptyTab(c, t, "No summary yet",
				"Generate an overview, key points, action items, chapters and highlights from the transcript.",
				[]action{{"Generate", true, func() { a.startSummary(m.ID) }}})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(t.Space(4)).Children(func() {
				if a.sum.Overview != "" {
					section(c, t, "Overview")
					ui.Text(c, a.sum.Overview).Selectable()
				}
				if len(a.sum.KeyPoints) > 0 {
					section(c, t, "Key points")
					for _, p := range a.sum.KeyPoints {
						bullet(c, t, "•", p)
					}
				}
				if len(a.sum.ActionItems) > 0 {
					section(c, t, "Action items")
					for _, p := range a.sum.ActionItems {
						bullet(c, t, "☐", p)
					}
				}
				if len(a.sum.Chapters) > 0 {
					section(c, t, "Chapters")
					for _, ch := range a.sum.Chapters {
						ui.Row(c).Gap(t.Space(3)).Padding(t.Space(1), 0).Children(func() {
							ui.Text(c, fmtDuration(ch.StartMs)).FontSize(t.FontSize - 2).
								TextColor(t.TextMuted).FontFeatures("tnum").Width(56)
							ui.Column(c).Grow(1).Gap(1).Children(func() {
								ui.Text(c, ch.Title).Bold()
								if ch.Summary != "" {
									ui.Text(c, ch.Summary).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
								}
							})
						})
					}
				}
				if len(a.sum.Highlights) > 0 {
					section(c, t, "Highlights")
					for _, h := range a.sum.Highlights {
						ui.Row(c).Gap(t.Space(3)).Padding(t.Space(1), 0).Children(func() {
							ui.Text(c, fmtDuration(h.StartMs)).FontSize(t.FontSize - 2).
								TextColor(t.TextMuted).FontFeatures("tnum").Width(56)
							ui.Text(c, h.Note).Grow(1)
						})
					}
				}
			})
		})
	})
}

func modelName(model, status string) string {
	if model != "" {
		return model
	}
	if status == "error" {
		return "the last attempt failed"
	}
	return "not generated"
}

func section(c *ui.Context, t *ui.Theme, title string) {
	ui.Text(c, strings.ToUpper(title)).FontSize(t.FontSize - 3).Bold().
		TextColor(t.TextMuted).LetterSpacing(0.6)
}

func bullet(c *ui.Context, t *ui.Theme, glyph, text string) {
	ui.Row(c).Gap(t.Space(2.5)).AlignItems(ui.Start).Children(func() {
		ui.Text(c, glyph).TextColor(t.TextMuted).Width(14)
		ui.Text(c, text).Grow(1).Selectable()
	})
}

func (a *App) loadSummary() {
	id := a.sel
	go func() {
		s, err := a.Sum.Get(id)
		a.update(func() {
			if id != a.sel {
				return
			}
			a.sumLoaded = true
			if err != nil {
				a.setError(err)
				return
			}
			a.sum = s
		})
	}()
}

func (a *App) startSummary(id int64) {
	if _, err := a.Sum.StartGenerate(id); err != nil {
		a.setError(err)
	}
}

func (a *App) exportSummary(id int64) {
	go func() {
		_, err := a.Sum.Export(context.Background(), id)
		if err != nil {
			a.update(func() { a.setError(err) })
		}
	}()
}

// ---------------------------------------------------------------- shared

type action struct {
	label    string
	enabled  bool
	onChoose func()
}

// emptyTab says why a tab has nothing, and offers what would fill it.
func (a *App) emptyTab(c *ui.Context, t *ui.Theme, title, detail string, actions []action) {
	ui.Column(c).Fill().Center().Gap(t.Space(2)).Children(func() {
		ui.Text(c, title).FontSize(t.FontSize + 4).Bold()
		ui.Text(c, detail).TextColor(t.TextMuted).MaxWidth(460).TextAlign(ui.Center)
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			for _, act := range actions {
				if !act.enabled {
					continue
				}
				if ui.PrimaryButton(c, act.label).Clicked() {
					act.onChoose()
				}
			}
		})
	})
}

// working shows what a long job is doing, how far it has come, and how to
// stop it.
func (a *App) working(c *ui.Context, t *ui.Theme, title, detail string) {
	ui.Column(c).Fill().Center().Gap(t.Space(2)).Children(func() {
		ui.Spinner(c).Label(title)
		ui.Text(c, title+"…").Bold()
		ui.Text(c, detail).TextColor(t.TextMuted).MaxWidth(420).TextAlign(ui.Center)
		if !a.job.Running() {
			return
		}
		// A negative value is a bar of unknown length, which is what work
		// that has not reported any progress yet has.
		value := a.job.Progress
		if value <= 0 {
			value = -1
		}
		ui.Progress(c, value).Width(240)
		if ui.Button(c, "Cancel").Label("Stop this job").Clicked() {
			a.cancelJob()
		}
	})
}

// loadTranscript reads the transcript of the media shown.
func (a *App) loadTranscript() {
	id := a.sel
	go func() {
		tr, err := a.ASR.Get(id)
		a.update(func() {
			if id != a.sel {
				return
			}
			a.trLoaded = true
			if err != nil {
				a.setError(err)
				return
			}
			a.transcript, a.segs = tr, tr.Segments
			a.lang = tr.Language
		})
	}()
}
