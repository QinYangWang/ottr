package native

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/native/kit"
	"ottr/internal/translate"
)

var langs = []struct{ code, name string }{
	{"en", "English"},
	{"zh", "中文"},
	{"ja", "日本語"},
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
func (a *App) langChoices() (names, codes []string) {
	names = append(names, a.tr("Auto"))
	codes = append(codes, asr.DefaultLanguage)
	for _, l := range langs {
		names = append(names, l.name)
		codes = append(codes, l.code)
	}
	return names, codes
}

func (a *App) langLabel(code string) string {
	if code == "" || code == asr.DefaultLanguage {
		return a.tr("Auto")
	}
	return langName(code)
}

// sessionPage is one media's page. Wide windows put the player and tabs side
// by side; compact windows stack them so both remain readable.
func (a *App) sessionPage(c *ui.Context, t *ui.Theme, m library.Media) {
	width, _ := c.Size()
	if width < 900 {
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(t.Space(3), t.Space(4), 0, t.Space(4)).Gap(t.Space(3)).Children(func() {
				a.sessionHead(c, t, m)
				a.playerStage(c, t, m)
			})
			ui.Column(c).Grow(1).MinHeight(0).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
				a.tabs(c, t, m)
			})
		})
		return
	}
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

// sideWidth is the column the tabs take beside a media.
const sideWidth = 356

// sessionHead names the media and holds the two things to do with its file: an
// icon button that shows it in the file manager, and a Delete in the danger
// tint. The name is all the text: the rest is in the fact strip.
func (a *App) sessionHead(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		if a.backButton(c, t, "") {
			a.closeDetail()
		}
		if a.renameID == m.ID {
			input := ui.TextInput(c, &a.renameText).Label(a.tr("Media name")).Grow(1).MinWidth(0)
			if input.Submitted() {
				a.saveRename(m.ID)
			}
			if ui.Button(c, a.tr("Save")).Clicked() {
				a.saveRename(m.ID)
			}
			if ui.Button(c, a.tr("Cancel")).Clicked() {
				a.renameID, a.renameText = 0, ""
			}
			return
		}
		title := ui.ButtonBase(c).Label(a.tr("Rename")).Tooltip(a.tr("Rename this media")).Grow(1).MinWidth(0)
		title.Children(func() {
			ui.Text(c, m.Name).FontSize(t.FontSize + 3).Bold().SingleLine().MinWidth(0)
		})
		if title.Clicked() {
			a.renameID, a.renameText = m.ID, m.Name
		}
		btn := ui.ButtonBase(c).Label(a.tr("Show file")).Tooltip(a.tr("Show the file in its folder")).
			Padding(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).TextColor(t.TextMuted)
		btn.Children(func() { ui.Icon(c, iconFolder).FontSize(t.FontSize + 2) })
		if btn.Hovered() {
			btn.Background(t.SurfaceHover).TextColor(t.Text)
		}
		if btn.Clicked() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		del := ui.ButtonBase(c).Label(a.tr("Delete")).Tooltip(a.tr("Delete this media from the library")).
			Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).TextColor(t.Danger)
		del.Children(func() {
			ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconTrash).FontSize(t.FontSize + 1)
				ui.Text(c, a.tr("Delete")).FontSize(t.FontSize - 1).Bold()
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

func (a *App) saveRename(id int64) {
	name := strings.TrimSpace(a.renameText)
	if name == "" {
		a.setError(fmt.Errorf("media name cannot be empty"))
		return
	}
	go func() {
		if err := a.Lib.Rename(id, name); err != nil {
			a.update(func() { a.setError(err) })
			return
		}
		a.update(func() {
			a.renameID, a.renameText = 0, ""
			a.reload()
		})
	}()
}

// playerStage presents finished media with MyGo's in-window player.
func (a *App) playerStage(c *ui.Context, t *ui.Theme, m library.Media) {
	a.nativePlayerStage(c, t, m)
}

// factStrip is the four facts of a media, in one row divided by hairlines.
func (a *App) factStrip(c *ui.Context, t *ui.Theme, m library.Media) {
	facts := [][2]string{
		{a.tr("Source"), a.mediaSourceLabel(m)},
		{a.tr("Duration"), durationOrDash(m)},
		{a.tr("Created"), a.dateTime(m.CreatedAt)},
		{a.tr("Status"), a.mediaStatus(m)},
	}
	ui.Row(c).FillWidth().Radius(t.Radius).Border(1, t.Border).Clip().
		AlignItems(ui.Stretch).Children(func() {
		for i, fact := range facts {
			if i > 0 {
				ui.Column(c).Width(1).Shrink(0).Background(t.Border)
			}
			ui.Column(c).Grow(1).MinWidth(0).Padding(t.Space(2.5), t.Space(3.5)).Gap(t.Space(1)).Children(func() {
				ui.Text(c, fact[0]).FontSize(t.FontSize - 3).Bold().
					TextColor(t.TextMuted).SingleLine()
				ui.Text(c, fact[1]).FontSize(t.FontSize - 1).Bold().SingleLine()
			})
		}
	})
}

func (a *App) mediaSourceLabel(m library.Media) string {
	switch m.Source {
	case "screenshot":
		return a.tr("Screenshot")
	case "import":
		return a.tr("Imported")
	default:
		return a.tr("Recorded")
	}
}

func durationOrDash(m library.Media) string {
	if m.DurationMs <= 0 {
		return "—"
	}
	return fmtDuration(m.DurationMs)
}

// mediaStatus says what has been made of the media, in words.
func (a *App) mediaStatus(m library.Media) string {
	var parts []string
	switch m.TranscriptStatus {
	case "done":
		parts = append(parts, a.tr("Transcribed"))
	case "pending":
		parts = append(parts, a.tr("Transcribing"))
	case "error":
		parts = append(parts, a.tr("Transcription failed"))
	default:
		parts = append(parts, a.tr("No transcript"))
	}
	if m.SummaryStatus == "done" {
		parts = append(parts, a.tr("Summarized"))
	}
	return strings.Join(parts, " · ")
}

// tabs is the right pane of a media's page: its tabs across the top, the
// export button beside them, and the chosen tab below.
func (a *App) tabs(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(4), t.Space(4), 0, t.Space(4)).Gap(t.Space(2)).
			AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Tabs(c, &a.tab, a.tr("Transcript"), a.tr("Summary")).Grow(1)
			a.exportButton(c, t, m)
			a.subtitleSettingsButton(c, t, &m)
		})
		ui.Column(c).Grow(1).MinHeight(0).Padding(t.Space(3), t.Space(4)).Children(func() {
			switch a.tab {
			case 1:
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
	label = a.tr(label)
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
		if a.sum.ContentMarkdown == "" {
			return nil, ""
		}
		id := m.ID
		return []exportItem{
			{"Markdown", func() { a.exportSummary(id) }},
		}, "Export the summary"
	default:
		if a.showTranslation && len(a.trans.Segments) > 0 {
			id := m.ID
			return []exportItem{
				{"Bilingual SRT", func() { a.exportTranslation(id, "bilingual_srt") }},
				{"SRT", func() { a.exportTranslation(id, "srt") }},
				{"WebVTT", func() { a.exportTranslation(id, "vtt") }},
				{"Text", func() { a.exportTranslation(id, "txt") }},
			}, "Export the translation"
		}
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
			if a.segSearchOpen {
				ui.SearchField(c, &a.segQuery).Label(a.tr("Search the subtitles")).Grow(1).MinWidth(120)
				kit.IconButton(c, t, iconClose, a.tr("Close search"), func() {
					a.segSearchOpen, a.segQuery = false, ""
				})
			} else {
				kit.IconButton(c, t, iconSearch, a.tr("Search the subtitles"), func() { a.segSearchOpen = true })
				a.spokenLanguageSelect(c, t, m)
				if a.showTranslation {
					a.translationTargetSelect(c, t)
				}
			}
			if a.transLoading || a.jobRunning(jobs.KindTranslate) {
				ui.Spinner(c).Label(a.tr("Translating"))
			}
		})
		rows := a.filteredSegments()
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(t.Space(2)).Children(func() {
				if len(rows) == 0 {
					ui.Text(c, a.tr("Nothing matches that search.")).TextColor(t.TextMuted)
				}
				translated := map[int64]string{}
				if a.showTranslation {
					for _, seg := range a.trans.Segments {
						translated[seg.SegmentID] = seg.TranslatedText
					}
				}
				for _, seg := range rows {
					at := fmtDuration(seg.StartMs)
					line := ui.ButtonBase(c.Key(seg.ID)).Label(a.tr("Seek to ")+at).Tooltip(a.tr("Seek to ")+at).
						FillWidth().Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Space(2)).Cursor(ui.CursorPointer)
					if line.Hovered() {
						line.Background(t.SurfaceHover)
					}
					line.Children(func() {
						a.subtitleContent(c, t, seg.Speaker, at, seg.Text, translated[seg.ID])
					})
					if line.Clicked() {
						a.seekPlayback(m, time.Duration(seg.StartMs)*time.Millisecond)
					}
				}
			})
		})
	})
}

func (a *App) setTranslationVisible(m library.Media, visible bool) {
	a.showTranslation = visible
	if !visible {
		return
	}
	if a.transLang != a.trLang {
		a.transLang = a.trLang
		a.loadTranslation()
		return
	}
	if len(a.trans.Segments) == 0 && !a.transLoading && !a.jobRunning(jobs.KindTranslate) {
		a.startTranslate(m.ID)
	}
}

// translationTargetSelect picks the language translations are shown in; it sits
// in the transcript toolbar, beside the spoken language.
func (a *App) translationTargetSelect(c *ui.Context, t *ui.Theme) {
	name := langName(a.trLang)
	ui.Select(c, &name, langNames()).Label(a.tr("Translate into")).Width(130).Shrink(0).Changed()
	for _, l := range langs {
		if l.name == name && l.code != a.trLang {
			a.trLang = l.code
			a.trans = translate.Translation{}
			a.loadTranslation()
		}
	}
}

// subtitleContent is shared by saved and live transcript rows: speaker, time and
// message, laid out like a chat thread — avatar on the left, the line in a
// bubble so that consecutive turns are easy to tell apart.
func (a *App) subtitleContent(c *ui.Context, t *ui.Theme, speaker int, at, text, translation string) {
	name := a.transcript.SpeakerNames[speaker]
	if name == "" {
		name = a.trf("Speaker %d", speaker+1)
	}
	avatar, ink := speakerTint(t, speakerNumber(name, speaker))
	ui.Row(c).FillWidth().Gap(t.Space(2.5)).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Size(32, 32).Shrink(0).Radius(16).Center().Background(avatar).Children(func() {
			ui.Text(c, speakerInitials(name, speaker)).FontSize(t.FontSize - 3).Bold().TextColor(ink)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Padding(0, 0, t.Space(0.5), 0).Children(func() {
				ui.Text(c, at).FontSize(t.FontSize - 3).TextColor(t.TextMuted).FontFeatures("tnum")
				if a.renameSpeakerID == speaker {
					input := ui.TextInput(c, &a.renameSpeakerText).Label(a.tr("Speaker name")).Grow(1).MinWidth(0)
					if input.Submitted() {
						a.saveSpeakerName(speaker)
					}
					if ui.Button(c, a.tr("Save")).Clicked() {
						a.saveSpeakerName(speaker)
					}
				} else {
					button := ui.ButtonBase(c).Label(a.tr("Rename speaker")).Tooltip(a.tr("Rename speaker"))
					button.Children(func() { ui.Text(c, name).FontSize(t.FontSize - 2).Bold().SingleLine() })
					if button.Clicked() {
						a.renameSpeakerID, a.renameSpeakerText = speaker, name
					}
				}
			})
			ui.Column(c).FillWidth().Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Space(2.5)).
				Background(t.Surface).Gap(t.Space(1)).Children(func() {
				ui.Text(c, text).FontSize(a.subtitleSize()).Selectable()
				if translation != "" {
					ui.Text(c, translation).FontSize(max(a.subtitleSize()-1, 10)).TextColor(t.TextMuted).Selectable()
				}
			})
		})
	})
}

// speakerTint is the colour of a speaker's avatar: decoration like a chat app's,
// so two voices are told apart at a glance, not an identity. The number starts at
// one, so Speaker 1 takes the first colour.
func speakerTint(t *ui.Theme, number int) (ui.Color, ui.Color) {
	colors := []ui.Color{t.Accent, t.Success, t.Warning, t.Danger}
	c := colors[max(number-1, 0)%len(colors)]
	return c.Alpha(0.16), c
}

// speakerNumber is the number a speaker is shown with: the digits of a generated
// name ("Speaker 3", "讲话者 3"), else one past the zero-based id.
func speakerNumber(name string, speaker int) int {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] < '0' || name[i] > '9' {
			if n, err := strconv.Atoi(strings.TrimSpace(name[i+1:])); err == nil && n > 0 {
				return n
			}
			break
		}
	}
	return speaker + 1
}

func speakerInitials(name string, speaker int) string {
	parts := strings.Fields(name)
	if len(parts) == 0 || strings.HasPrefix(name, "Speaker ") {
		return fmt.Sprintf("S%d", speakerNumber(name, speaker))
	}
	var initials strings.Builder
	for _, part := range parts[:min(2, len(parts))] {
		for _, r := range part {
			initials.WriteString(strings.ToUpper(string(r)))
			break
		}
	}
	return initials.String()
}

func (a *App) saveSpeakerName(id int) {
	name := strings.TrimSpace(a.renameSpeakerText)
	if name == "" {
		a.setError(fmt.Errorf("speaker name cannot be empty"))
		return
	}
	go func() {
		if err := a.ASR.RenameSpeaker(id, name); err != nil {
			a.update(func() { a.setError(err) })
			return
		}
		a.update(func() {
			if a.transcript.SpeakerNames == nil {
				a.transcript.SpeakerNames = map[int]string{}
			}
			a.transcript.SpeakerNames[id] = name
			a.renameSpeakerID, a.renameSpeakerText = -1, ""
		})
	}()
}

// spokenLanguageSelect overrules the detected language without adding a
// second toolbar or a visible label.
func (a *App) spokenLanguageSelect(c *ui.Context, t *ui.Theme, m library.Media) {
	names, codes := a.langChoices()
	lang := a.langLabel(a.lang)
	ui.Select(c, &lang, names).Label(a.tr("Spoken language")).Width(120).Shrink(0).Changed()
	for i, name := range names {
		if name == lang && codes[i] != a.lang {
			a.setLanguage(m.ID, codes[i])
		}
	}
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

func langNames() []string {
	out := make([]string, len(langs))
	for i, l := range langs {
		out[i] = l.name
	}
	return out
}

func (a *App) loadTranslation() {
	id, lang := a.sel, a.trLang
	a.transLoading = true
	go func() {
		tr, err := a.Trans.Get(id, lang)
		a.update(func() {
			if id != a.sel || lang != a.trLang {
				return
			}
			a.transLoading = false
			if err != nil {
				a.setError(err)
				return
			}
			a.trans = tr
			if a.showTranslation && len(tr.Segments) == 0 && !a.jobRunning(jobs.KindTranslate) {
				a.startTranslate(id)
			}
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
			ui.Text(c, a.modelName(a.sum.Model, m.SummaryStatus)).FontSize(t.FontSize - 2).
				TextColor(t.TextMuted).Grow(1).SingleLine()
			if has {
				if ui.Button(c, a.tr("Generate again")).Clicked() {
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
					a.section(c, t, "Overview")
					ui.Text(c, a.sum.Overview).Selectable()
				}
				if len(a.sum.KeyPoints) > 0 {
					a.section(c, t, "Key points")
					for _, p := range a.sum.KeyPoints {
						bullet(c, t, "•", p)
					}
				}
				if len(a.sum.ActionItems) > 0 {
					a.section(c, t, "Action items")
					for _, p := range a.sum.ActionItems {
						bullet(c, t, "☐", p)
					}
				}
				if len(a.sum.Chapters) > 0 {
					a.section(c, t, "Chapters")
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
					a.section(c, t, "Highlights")
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

func (a *App) modelName(model, status string) string {
	if model != "" {
		return model
	}
	if status == "error" {
		return a.tr("the last attempt failed")
	}
	return a.tr("not generated")
}

func (a *App) section(c *ui.Context, t *ui.Theme, title string) {
	ui.Text(c, a.tr(title)).FontSize(t.FontSize - 3).Bold().
		TextColor(t.TextMuted)
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
		ui.Text(c, a.tr(title)).FontSize(t.FontSize + 4).Bold()
		ui.Text(c, a.tr(detail)).TextColor(t.TextMuted).MaxWidth(460).TextAlign(ui.Center)
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			for _, act := range actions {
				if !act.enabled {
					continue
				}
				if ui.PrimaryButton(c, a.tr(act.label)).Clicked() {
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
		ui.Spinner(c).Label(a.tr(title))
		ui.Text(c, a.tr(title)+"…").Bold()
		ui.Text(c, a.tr(detail)).TextColor(t.TextMuted).MaxWidth(420).TextAlign(ui.Center)
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
		if ui.Button(c, a.tr("Cancel")).Label(a.tr("Stop this job")).Clicked() {
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
