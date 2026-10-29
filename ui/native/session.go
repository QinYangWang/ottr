package native

import (
	"context"
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

// sessionPage is one media's page: the still of the file and what it is on the
// left, its tabs on the right — the same page the prototype opens in place of
// the grid.
func (a *App) sessionPage(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
			ui.Column(c).Padding(t.Space(4), t.Space(5), t.Space(5), t.Space(5)).Gap(t.Space(4)).Children(func() {
				a.sessionHead(c, t, m)
				a.playerStage(c, t, m)
				a.factStrip(c, t, m)
			})
		})
		ui.Column(c).Width(sideWidth).Shrink(0).FillHeight().MinWidth(0).
			Background(t.Surface).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
			a.tabs(c, t, m)
		})
	})
}

// sideWidth is the column the tabs take beside a media, as the prototype does.
const sideWidth = 356

// sessionHead names the media and holds the two things to do with its file: an
// icon button that shows it in the file manager, and a Delete in the danger
// tint. The name is all the text: the rest is in the fact strip.
func (a *App) sessionHead(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		if a.backButton(c, t, "") {
			a.closeDetail()
		}
		ui.Text(c, m.Name).FontSize(t.FontSize + 3).Bold().Grow(1).MinWidth(0).SingleLine()
		btn := ui.ButtonBase(c).Label("Show file").Tooltip("Show the file in its folder").
			Padding(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).TextColor(t.TextMuted)
		btn.Children(func() { ui.Icon(c, iconFolder).FontSize(t.FontSize + 2) })
		if btn.Hovered() {
			btn.Background(t.SurfaceHover).TextColor(t.Text)
		}
		if btn.Clicked() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		del := ui.ButtonBase(c).Label("Delete").Tooltip("Delete this media from the library").
			Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).TextColor(t.Danger)
		del.Children(func() {
			ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconTrash).FontSize(t.FontSize + 1)
				ui.Text(c, "Delete").FontSize(t.FontSize - 1).Bold()
			})
		})
		if del.Hovered() {
			del.Background(t.Danger.Alpha(0.12))
		}
		if del.Clicked() {
			a.delID, a.confirmDel = m.ID, true
		}
	})
}

// playerStage is the dark ground the media sits on, with its controls in a
// strip below. There is no decoder in the window yet (see
// docs/native-playback.md), so the picture is the badge of what the file is and
// Play hands it to the system's player.
func (a *App) playerStage(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).FillWidth().Radius(t.Radius + 2).Clip().
		BorderWidth(1).BorderColor(t.Border).Children(func() {
		from, to := mediaBackdrop(t.Dark)
		stage := ui.Column(c).FillWidth().AspectRatio(16.0/9.0).Gradient(from, to, 90).Center()
		stage.Children(func() {
			if m.HasVideo || m.Source == "screenshot" {
				play := ui.ButtonBase(c).Label("Play").Tooltip("Open the media in your player").
					Padding(t.Space(4)).Radius(t.Space(10)).Background(ui.RGBA(255, 255, 255, 0.12))
				play.Children(func() {
					icon := iconPlay
					if m.Source == "screenshot" {
						icon = iconScreen
					}
					ui.Icon(c, icon).FontSize(t.FontSize + 12).TextColor(onMedia())
				})
				if play.Hovered() {
					play.Background(ui.RGBA(255, 255, 255, 0.2))
				}
				if play.Clicked() && m.Source != "screenshot" {
					mygo.Shell.OpenPath(m.Path)
				}
				return
			}
			ui.Icon(c, iconMic).FontSize(t.FontSize + 14).TextColor(onMedia()).Opacity(0.7)
		})
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(3)).AlignItems(ui.Center).
			Background(playerBar(t)).Children(func() {
			icon := iconPlay
			if m.Source == "screenshot" {
				icon = iconScreen
			}
			ui.Icon(c, icon).FontSize(t.FontSize + 1).TextColor(onMedia()).Opacity(0.9)
			ui.Text(c, fmtDuration(m.DurationMs)+" / "+fmtDuration(m.DurationMs)).
				FontSize(t.FontSize - 2).TextColor(onMedia()).Opacity(0.75).FontFeatures("tnum")
			ui.Spacer(c)
			note := "Opens in your player"
			if m.Source == "screenshot" {
				note = "Still image"
			}
			ui.Text(c, note).FontSize(t.FontSize - 3).TextColor(onMedia()).Opacity(0.6)
		})
	})
}

// factStrip is the four facts of a media, in one row divided by hairlines.
func (a *App) factStrip(c *ui.Context, t *ui.Theme, m library.Media) {
	facts := [][2]string{
		{"Source", mediaSourceLabel(m)},
		{"Duration", durationOrDash(m)},
		{"Created", m.CreatedAt.Local().Format("Jan 2, 15:04")},
		{"Status", mediaStatus(m)},
	}
	ui.Row(c).FillWidth().Radius(t.Radius).Border(1, t.Border).Clip().
		AlignItems(ui.Stretch).Children(func() {
		for i, fact := range facts {
			if i > 0 {
				ui.Column(c).Width(1).Shrink(0).Background(t.Border)
			}
			ui.Column(c).Grow(1).MinWidth(0).Padding(t.Space(2.5), t.Space(3.5)).Gap(t.Space(1)).Children(func() {
				ui.Text(c, strings.ToUpper(fact[0])).FontSize(t.FontSize - 4).Bold().
					TextColor(t.TextMuted).SingleLine()
				ui.Text(c, fact[1]).FontSize(t.FontSize - 1).Bold().SingleLine()
			})
		}
	})
}

func mediaSourceLabel(m library.Media) string {
	switch m.Source {
	case "screenshot":
		return "Screenshot"
	case "import":
		return "Imported"
	default:
		return "Recorded"
	}
}

func durationOrDash(m library.Media) string {
	if m.DurationMs <= 0 {
		return "—"
	}
	return fmtDuration(m.DurationMs)
}

// mediaStatus says what has been made of the media, in words.
func mediaStatus(m library.Media) string {
	var parts []string
	switch m.TranscriptStatus {
	case "done":
		parts = append(parts, "Transcribed")
	case "pending":
		parts = append(parts, "Transcribing")
	case "error":
		parts = append(parts, "Transcription failed")
	default:
		parts = append(parts, "No transcript")
	}
	if m.SummaryStatus == "done" {
		parts = append(parts, "Summarized")
	}
	return strings.Join(parts, " · ")
}

// tabs is the right pane of a media's page: its tabs across the top, the
// export button beside them, and the chosen tab below.
func (a *App) tabs(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(4), t.Space(4), 0, t.Space(4)).Gap(t.Space(2)).
			AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Tabs(c, &a.tab, "Transcript", "Translate", "Summary").Grow(1)
			a.exportButton(c, t, m)
		})
		ui.Column(c).Grow(1).MinHeight(0).Padding(t.Space(3), t.Space(4)).Children(func() {
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

// exportButton is the one way out of whatever the tabs made: the icon alone,
// with its name in the label and tooltip.
func (a *App) exportButton(c *ui.Context, t *ui.Theme, m library.Media) {
	exports, label := a.exportsFor(m)
	if len(exports) == 0 {
		return
	}
	btn := ui.ButtonBase(c).Label(label).Tooltip(label).Padding(t.Space(1.5)).
		Radius(t.Radius).TextColor(t.TextMuted).Border(1, t.Border)
	btn.Children(func() { ui.Icon(c, iconDownload).FontSize(t.FontSize + 1) })
	if btn.Hovered() {
		btn.Background(t.SurfaceHover).TextColor(t.Text)
	}
	btn.Menu(func(mn *ui.Menu) {
		for _, item := range exports {
			if mn.Item(item.name).Chosen() {
				item.run()
			}
		}
	})
}

// exportItem is one entry of a tab's export menu.
type exportItem struct {
	name string
	run  func()
}

// exportsFor lists what the tab shown can be exported as, and the menu's name.
func (a *App) exportsFor(m library.Media) ([]exportItem, string) {
	switch a.tab {
	case 1:
		if len(a.trans.Segments) == 0 {
			return nil, ""
		}
		id := m.ID
		return []exportItem{
			{"Bilingual SRT", func() { a.exportTranslation(id, "bilingual_srt") }},
			{"SRT", func() { a.exportTranslation(id, "srt") }},
			{"WebVTT", func() { a.exportTranslation(id, "vtt") }},
			{"Text", func() { a.exportTranslation(id, "txt") }},
		}, "Export the translation"
	case 2:
		if a.sum.ContentMarkdown == "" {
			return nil, ""
		}
		id := m.ID
		return []exportItem{
			{"Markdown", func() { a.exportSummary(id) }},
		}, "Export the summary"
	default:
		if len(a.segs) == 0 {
			return nil, ""
		}
		id := m.ID
		return []exportItem{
			{"SRT", func() { a.exportTranscript(id, "srt") }},
			{"WebVTT", func() { a.exportTranscript(id, "vtt") }},
			{"Text", func() { a.exportTranscript(id, "txt") }},
		}, "Export the transcript"
	}
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
					line := ui.Row(c.Key(seg.ID)).Gap(t.Space(3)).Padding(t.Space(2), t.Space(2.5)).
						Radius(t.Space(2))
					if line.Hovered() {
						line.Background(t.SurfaceHover)
					}
					line.Children(func() {
						ui.Text(c, fmtDuration(seg.StartMs)).FontSize(t.FontSize - 3).
							TextColor(t.TextMuted).FontFeatures("tnum").Width(44).Shrink(0)
						ui.Text(c, seg.Text).Grow(1).MinWidth(0).Selectable()
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
			ui.Text(c, "Translation").FontSize(t.FontSize - 3).TextColor(t.TextMuted).Shrink(0)
			name := langName(a.trLang)
			ui.Select(c, &name, langNames()).Label("Translate into").Width(150).Changed()
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
				if ui.Button(c, "Translate again").Clicked() {
					a.startTranslate(m.ID)
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
			ui.Text(c, modelName(a.sum.Model, m.SummaryStatus)).FontSize(t.FontSize - 2).
				TextColor(t.TextMuted).Grow(1).SingleLine()
			if has {
				if ui.Button(c, "Generate again").Clicked() {
					a.startSummary(m.ID)
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
