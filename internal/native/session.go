package native

import (
	"context"
	"fmt"
	"path/filepath"
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

// Tabs of an item page, in their indices; the bar shows Transcript then
// Summary.
const (
	tabTranscript = 0
	tabSummary    = 1
)

// sessionPage is one media's page, the reading desk: the media, its seek bar
// split into chapters, the chapters and the facts in the centre, and the
// document panel (Transcript / translation / Summary) on the right. The
// narrow window stacks them.
func (a *App) sessionPage(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	width, _ := c.Size()
	layout := layoutOf(width)
	if m.SummaryStatus == "done" && !a.sumLoaded && !a.sumLoading {
		a.loadSummary() // the chapters under the player come from it
	}
	if layout == "narrow" {
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.Sheet).Children(func() {
			ui.Column(c).Shrink(0).Padding(12, 16).Gap(12).Children(func() {
				a.sessionHead(c, t, m, true)
				a.playerStage(c, t, m)
			})
			ui.Column(c).Grow(1).MinHeight(0).BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
				a.docPanel(c, t, m)
			})
		})
		return
	}
	panel := float32(docWidth)
	if layout == "mid" {
		panel = docWidthMid
	}
	ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Background(p.Sheet).Children(func() {
		ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
			ui.Column(c).Padding(16, 24, 24, 24).Gap(16).Children(func() {
				a.sessionHead(c, t, m, layout == "mid")
				a.playerStage(c, t, m)
				a.factStrip(c, t, m)
			})
		})
		ui.Column(c).Width(panel).Shrink(0).FillHeight().MinWidth(0).
			BorderWidth(0, 0, 0, 1).BorderColor(p.Line).Children(func() {
			a.docPanel(c, t, m)
		})
	})
}

// sessionHead is the way back, then the title (a click renames it) and its
// meta, with what to do with the file — show it, export, delete — on the right.
func (a *App) sessionHead(c *ui.Context, t *ui.Theme, m library.Media, compact bool) {
	p := pal(t)
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		back := ui.ButtonBase(c).Label(a.tr("Back to library")).Height(32).Padding(0, 10).Gap(6).
			Radius(kit.RadiusControl).AlignItems(ui.Center).AlignSelf(ui.Start).TextColor(p.Ink2).
			Cursor(ui.CursorPointer)
		if back.Hovered() {
			back.Background(p.Hover).TextColor(p.Ink)
		}
		back.Children(func() {
			ui.Icon(c, iconBack).FontSize(16)
			ui.Text(c, a.tr("Back to library")).FontSize(13)
		})
		if back.Clicked() {
			a.closeDetail()
		}
		a.sessionTitle(c, t, m, compact)
	})
}

// sessionTitle is the name (a click renames it), its meta and the file's
// actions.
func (a *App) sessionTitle(c *ui.Context, t *ui.Theme, m library.Media, compact bool) {
	p := pal(t)
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if a.renameID == m.ID {
			input := ui.TextInput(c, &a.renameText).Label(a.tr("Media name")).Grow(1).MinWidth(0)
			if input.Submitted() {
				a.saveRename(m.ID)
			}
			if kit.Button(c, t, kit.Primary, nil, a.tr("Save"), false).Clicked() {
				a.saveRename(m.ID)
			}
			if kit.Button(c, t, kit.Secondary, nil, a.tr("Cancel"), false).Clicked() {
				a.renameID, a.renameText = 0, ""
			}
			return
		}
		title := ui.ButtonBase(c).Label(a.tr("Rename")).Tooltip(a.tr("Rename this media")).Grow(1).MinWidth(0).
			Padding(0, 4)
		title.Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				ui.Text(c, m.Name).FontSize(18).FontWeight(600).LetterSpacing(-0.2).TextColor(p.Ink).
					SingleLine().MinWidth(0)
				ui.Text(c, a.sessionMeta(m)).FontSize(12).TextColor(p.Ink3).SingleLine()
			})
		})
		if title.Clicked() {
			a.renameID, a.renameText = m.ID, m.Name
		}
		if compact {
			if len(a.segs) > 0 {
				kit.IconButton(c, t, iconDownload, a.tr("Export"), func() { a.openExport(m) })
			}
		} else if kit.Button(c, t, kit.Ghost, iconFolderOpen, a.tr("Show file"), false).
			Tooltip(a.tr("Show the file in its folder")).Clicked() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		if len(a.segs) > 0 && !compact {
			if kit.Button(c, t, kit.Secondary, iconDownload, a.tr("Export"), false).Clicked() {
				a.openExport(m)
			}
		}
		if m.TranscriptStatus == "pending" {
			// Deleting under a running transcription would leave it writing
			// to a record that is gone.
			return
		}
		del := ui.ButtonBase(c).Label(a.tr("Delete")).Tooltip(a.tr("Delete this media from the library")).
			Size(32, 32).Radius(kit.RadiusControl).Center().TextColor(p.Red).Cursor(ui.CursorPointer)
		del.Children(func() { ui.Icon(c, iconTrash).FontSize(16) })
		if del.Hovered() {
			del.Background(p.RedSoft)
		}
		if del.Clicked() {
			a.askDelete(m.ID)
		}
	})
}

// sessionMeta is the line under the title: "Recorded · Nov 6, 10:02 · 42:18 ·
// 3 speakers".
func (a *App) sessionMeta(m library.Media) string {
	parts := []string{a.mediaSourceLabel(m), a.dateTime(m.CreatedAt)}
	if m.DurationMs > 0 {
		parts = append(parts, fmtDuration(m.DurationMs))
	}
	if n := speakerCount(a.segs); n > 0 && a.trLoaded {
		parts = append(parts, a.speakersText(n))
	}
	return strings.Join(parts, " · ")
}

// playerStage presents the media with MyGo's in-window player.
func (a *App) playerStage(c *ui.Context, t *ui.Theme, m library.Media) {
	a.nativePlayerStage(c, t, m)
}

// factStrip is the media's four facts under a hairline: where it came from,
// the file, where it is saved, and its languages.
func (a *App) factStrip(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	file := []string{}
	if m.Height > 0 {
		file = append(file, strconv.Itoa(m.Height)+"p")
	}
	if m.Size > 0 {
		file = append(file, fmtSize(m.Size))
	}
	if len(file) == 0 {
		file = append(file, strings.TrimPrefix(filepath.Ext(m.Path), "."))
	}
	lang := a.langLabel(a.lang)
	if len(a.trans.Segments) > 0 {
		lang += " → " + langName(a.trLang)
	}
	facts := [][2]string{
		{a.tr("Source"), a.mediaSourceLabel(m)},
		{a.tr("File"), strings.Join(file, " · ")},
		{a.tr("Saved to"), shortDir(filepath.Dir(m.Path))},
		{a.tr("Language"), lang},
	}
	ui.Row(c).FillWidth().Padding(14, 0, 0, 0).Gap(16).BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
		for i, fact := range facts {
			ui.Column(c.Key(i)).Grow(1).Width(0).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, fact[0]).FontSize(11.5).TextColor(p.Ink3).SingleLine()
				ui.Text(c, fact[1]).FontSize(12.5).TextColor(p.Ink).SingleLine().Tooltip(fact[1])
			})
		}
	})
}

// ---------------------------------------------------------- document panel

// docPanel is the right pane of a media's page: its tabs with search, export
// and the settings menu across the top, and the chosen tab below.
func (a *App) docPanel(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	width, _ := c.Size()
	gap, pad := float32(20), float32(20)
	if layoutOf(width) == "mid" {
		gap, pad = 16, 16
	}
	ui.Column(c).Fill().MinHeight(0).Background(p.Sheet).Children(func() {
		ui.Row(c).FillWidth().Height(48).Shrink(0).Padding(0, 8, 0, pad).Gap(gap).AlignItems(ui.Stretch).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
			a.docTab(c, t, tabTranscript, a.tr("Transcript"), "", kit.ChipNeutral)
			a.docTab(c, t, tabSummary, a.tr("Summary"), "", kit.ChipNeutral)
			ui.Spacer(c)
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				if a.tab != tabSummary && len(a.segs) > 0 {
					kit.IconButton(c, t, iconSearch, a.tr("Search the subtitles"), func() {
						a.segSearchOpen = !a.segSearchOpen
						if !a.segSearchOpen {
							a.segQuery = ""
						}
					})
				}
				if layoutOf(width) != "mid" {
					a.exportButton(c, t, m)
				}
				a.subtitleSettingsButton(c, t, &m)
			})
		})
		switch a.tab {
		case tabSummary:
			a.summaryTab(c, t, m)
		default:
			a.transcriptTab(c, t, m)
		}
	})
}

// docTab is one tab: its name, an optional chip, and the ink underline when
// chosen.
func (a *App) docTab(c *ui.Context, t *ui.Theme, index int, label, chip string, kind kit.ChipKind) {
	p := pal(t)
	on := a.tab == index
	tab := ui.ButtonBase(c.Key(index)).Label(label).Role(ui.RoleTab).Checked(on).Gap(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).BorderWidth(0, 0, 2, 0)
	if on {
		tab.BorderColor(p.Ink)
	} else {
		tab.BorderColor(ui.Transparent)
	}
	tab.Children(func() {
		text := ui.Text(c, label).FontSize(13).SingleLine()
		if on {
			text.FontWeight(500).TextColor(p.Ink)
		} else if tab.Hovered() {
			text.TextColor(p.Ink)
		} else {
			text.TextColor(p.Ink3)
		}
		if chip != "" {
			kit.Chip(c, t, kind, chip)
		}
	})
	if tab.Clicked() && !on {
		a.tab = index
	}
}

// exportButton opens the export dialog, from any tab.
func (a *App) exportButton(c *ui.Context, t *ui.Theme, m library.Media) {
	label := ""
	switch {
	case a.tab == tabSummary && a.sum.ContentMarkdown != "":
		label = a.tr("Export the summary")
	case a.tab != tabSummary && len(a.segs) > 0:
		label = a.tr("Export the transcript")
	default:
		return
	}
	kit.IconButton(c, t, iconDownload, label, func() {
		if a.tab == tabSummary && len(a.segs) == 0 {
			a.exportSummary(m.ID)
			return
		}
		a.openExport(m)
		if a.tab == tabSummary {
			a.exportKind = 2
		}
	})
}

// ------------------------------------------------------------- transcript

func (a *App) transcriptTab(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	if m.TranscriptStatus == "pending" {
		if a.job.Kind == jobs.KindTranscribe {
			a.transcribingPanel(c, t, m)
			return
		}
		a.working(c, t, "Transcribing", "Recognizing the speech in this file on this computer.")
		return
	}
	if m.TranscriptStatus != "done" && a.provisionalID == m.ID && len(a.provisional) > 0 {
		a.liveTranscriptTab(c, t, m)
		return
	}
	if m.TranscriptStatus != "done" {
		detail := "Offline speech recognition turns the audio into timestamped subtitles."
		switch {
		case a.job.Kind == jobs.KindTranscribe && a.job.State == jobs.StateFailed && a.job.Error != "":
			detail = a.job.Error
		case a.trErr != "":
			detail = a.trErr
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
	hits := a.searchHits()
	width, _ := c.Size()
	if a.segSearchOpen {
		a.searchBar(c, t, m, hits)
	} else if layoutOf(width) != "mid" {
		ui.Row(c).FillWidth().Height(48).Shrink(0).Padding(0, 20).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
			ui.Text(c, a.tr("Spoken")).FontSize(12).TextColor(p.Ink3)
			a.spokenLanguageSelect(c, t, m)
			ui.Spacer(c)
			if a.transLoading || a.jobRunning(jobs.KindTranslate) {
				ui.Spinner(c).Label(a.tr("Translating")).Size(12, 12)
			}
			ui.Text(c, a.trf("%s under lines", langName(a.trLang))).FontSize(12).TextColor(p.Ink2).SingleLine()
			on := a.showTranslation
			if kit.Switch(c, t, &on, a.tr("Show translation")).Changed() {
				a.setTranslationVisible(m, on)
			}
		})
	}
	translated := map[int64]string{}
	if a.showTranslation {
		for _, seg := range a.trans.Segments {
			translated[seg.SegmentID] = seg.TranslatedText
		}
	}
	active := a.activeSegment(m)
	current := int64(-1)
	if len(hits) > 0 {
		current = hits[min(a.segHit, len(hits)-1)]
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(10, 10, 16, 10).Gap(2).Children(func() {
			for _, seg := range a.segs {
				a.transcriptRow(c.Key(seg.ID), t, m, seg, translated[seg.ID], seg.ID == active, seg.ID == current)
			}
		})
	})
	a.docFooter(c, t, func() {
		ui.Icon(c, iconCPU).FontSize(14).TextColor(p.Ink3)
		ui.Text(c, a.trf("Transcribed on this computer · %s", a.speakersText(speakerCount(a.segs)))).
			FontSize(11.5).TextColor(p.Ink3).SingleLine().Grow(1).MinWidth(0)
		mono(ui.Text(c, a.trf("%d lines", len(a.segs)))).FontSize(11).TextColor(p.Ink3)
	})
}

// docFooter is the strip at the bottom of the document panel.
func (a *App) docFooter(c *ui.Context, t *ui.Theme, children func()) {
	p := pal(t)
	ui.Row(c).FillWidth().Height(44).Shrink(0).Padding(0, 20).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(children)
}

// activeSegment is the line under the playhead, or -1 when nothing plays.
func (a *App) activeSegment(m library.Media) int64 {
	if a.playback.id != m.ID || a.playback.loading || a.playback.err != "" {
		return -1
	}
	at := a.playbackPosition(m).Milliseconds()
	for _, seg := range a.segs {
		if at >= seg.StartMs && at < max(seg.EndMs, seg.StartMs+1) {
			return seg.ID
		}
	}
	return -1
}

// transcriptRow is one line: the speaker's avatar, name and time, the text,
// and the translation under it. The line playing gets the highlighter: an
// amber-soft fill and a 3px amber bar.
func (a *App) transcriptRow(c *ui.Context, t *ui.Theme, m library.Media, seg asr.Segment, translation string, active, currentHit bool) {
	p := pal(t)
	at := fmtDuration(seg.StartMs)
	row := ui.ButtonBase(c).Label(a.tr("Seek to ")+at).Tooltip(a.tr("Seek to ")+at).FillWidth().
		Padding(9, 10, 9, 10).Radius(kit.RadiusCard).Cursor(ui.CursorPointer)
	switch {
	case active:
		row.Background(p.AmberSoft)
	case row.Hovered():
		row.Background(p.Sunk)
	}
	if currentHit {
		row.ScrollIntoView()
	}
	row.Children(func() {
		if active {
			ui.Box(c).Absolute().Left(0).Top(8).Bottom(8).Width(3).Radius(2).Background(p.Amber)
		}
		a.lineContent(c, t, seg.Speaker, at, seg.Text, translation, active, currentHit)
	})
	if row.Clicked() {
		a.seekPlayback(m, time.Duration(seg.StartMs)*time.Millisecond)
	}
}

// lineContent is a transcript line's avatar, name, time, text and translation.
func (a *App) lineContent(c *ui.Context, t *ui.Theme, speaker int, at, text, translation string, active, current bool) {
	p := pal(t)
	name := a.transcript.SpeakerNames[speaker]
	if name == "" {
		name = a.trf("Speaker %d", speaker+1)
	}
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		kit.Avatar(c, speakerNumber(name, speaker), speakerInitials(name, speaker), 24)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if a.renameSpeakerID == speaker {
					input := ui.TextInput(c, &a.renameSpeakerText).Label(a.tr("Speaker name")).Grow(1).MinWidth(0)
					if input.Submitted() {
						a.saveSpeakerName(speaker)
					}
					if kit.Button(c, t, kit.Primary, nil, a.tr("Save"), false).Clicked() {
						a.saveSpeakerName(speaker)
					}
				} else {
					button := ui.ButtonBase(c).Label(a.tr("Rename speaker")).Tooltip(a.tr("Rename speaker")).
						Cursor(ui.CursorPointer)
					button.Children(func() {
						ui.Text(c, name).FontSize(12).FontWeight(600).TextColor(p.Ink).SingleLine()
					})
					if button.Clicked() {
						a.renameSpeakerID, a.renameSpeakerText = speaker, name
					}
				}
				ink := p.Ink3
				if active {
					ink = p.AmberInk
				}
				mono(ui.Text(c, at)).FontSize(11.5).TextColor(ink)
			})
			a.highlightedText(c, t, text, a.subtitleSize(), p.Ink, current)
			if translation != "" {
				ui.Text(c, translation).FontSize(max(a.subtitleSize()-0.5, 10)).TextColor(p.Ink3).Selectable()
			}
		})
	})
}

// highlightedText draws text with the search query marked: amber-soft with an
// amber underline, and solid amber on the match shown.
func (a *App) highlightedText(c *ui.Context, t *ui.Theme, text string, size float32, ink ui.Color, current bool) {
	p := pal(t)
	q := strings.TrimSpace(a.segQuery)
	if !a.segSearchOpen || q == "" {
		ui.Text(c, text).FontSize(size).TextColor(ink).Selectable()
		return
	}
	lower, lq := strings.ToLower(text), strings.ToLower(q)
	var spans []ui.Span
	for {
		i := strings.Index(lower, lq)
		if i < 0 {
			spans = append(spans, ui.Span{Text: text, Color: ink, Size: size})
			break
		}
		if i > 0 {
			spans = append(spans, ui.Span{Text: text[:i], Color: ink, Size: size})
		}
		hit := ui.Span{Text: text[i : i+len(lq)], Color: ink, Size: size,
			Background: p.AmberSoft, Underline: true, DecorationColor: p.Amber, DecorationThickness: 2}
		if current {
			hit.Background, hit.Color, hit.Underline = p.Amber, ui.Hex("#121417"), false
		}
		spans = append(spans, hit)
		text, lower = text[i+len(lq):], lower[i+len(lq):]
		if text == "" {
			break
		}
	}
	ui.RichText(c, spans...).Selectable()
}

// searchHits are the lines that contain the query, in order.
func (a *App) searchHits() []int64 {
	q := strings.TrimSpace(strings.ToLower(a.segQuery))
	if !a.segSearchOpen || q == "" {
		return nil
	}
	var hits []int64
	for _, s := range a.segs {
		if strings.Contains(strings.ToLower(s.Text), q) {
			hits = append(hits, s.ID)
		}
	}
	return hits
}

// searchBar replaces the panel's sub-bar while searching: the field, "3 of 5",
// up and down through the hits, and close (Esc).
func (a *App) searchBar(c *ui.Context, t *ui.Theme, m library.Media, hits []int64) {
	p := pal(t)
	if a.segHit >= len(hits) {
		a.segHit = 0
	}
	ui.Row(c).FillWidth().Height(48).Shrink(0).Padding(0, 12, 0, 20).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Height(32).Padding(0, 10).Gap(8).AlignItems(ui.Center).
			Radius(kit.RadiusControl).Border(1, p.Amber).Shadow(0, 0, 0, 3, p.AmberSoft).
			Background(p.Sheet).Children(func() {
			ui.Icon(c, iconSearch).FontSize(16).TextColor(p.Ink3)
			field := ui.TextInputBase(c, &a.segQuery).Label(a.tr("Search the subtitles")).
				Placeholder(a.tr("Search the transcript")).PlaceholderColor(p.Ink3).
				Grow(1).MinWidth(0).FontSize(13).TextColor(p.Ink).AutoFocus()
			if field.Changed() {
				a.segHit = 0
			}
			if field.Shortcut(0, ui.KeyEscape) {
				a.segSearchOpen, a.segQuery = false, ""
			}
			if field.Submitted() && len(hits) > 0 {
				a.segHit = (a.segHit + 1) % len(hits)
			}
			if strings.TrimSpace(a.segQuery) != "" {
				count := a.trf("%d of %d", min(a.segHit+1, len(hits)), len(hits))
				if len(hits) == 0 {
					count = a.tr("No matches")
				}
				mono(ui.Text(c, count)).FontSize(11.5).TextColor(p.Ink3).SingleLine()
			}
		})
		kit.IconButton(c, t, iconArrowUp, a.tr("Previous match"), func() {
			if len(hits) > 0 {
				a.segHit = (a.segHit + len(hits) - 1) % len(hits)
			}
		})
		kit.IconButton(c, t, iconArrowDown, a.tr("Next match"), func() {
			if len(hits) > 0 {
				a.segHit = (a.segHit + 1) % len(hits)
			}
		})
		kit.IconButton(c, t, iconClose, a.tr("Close search"), func() {
			a.segSearchOpen, a.segQuery = false, ""
		})
	})
}

// translationTargetSelect picks the language translations are shown in.
func (a *App) translationTargetSelect(c *ui.Context, t *ui.Theme, m library.Media) {
	name := langName(a.trLang)
	options := make([]kit.SelectOption[string], len(langs))
	for i, l := range langs {
		options[i] = kit.SelectOption[string]{Value: l.name, Label: l.name}
	}
	kit.CompactSelect(c, t, &name, options, a.tr("Translate into"), 130, nil, func(choice string) {
		for _, l := range langs {
			if l.name == choice && l.code != a.trLang {
				a.trLang = l.code
				a.trans = translate.Translation{}
				if !a.showTranslation {
					// Picking a language is an explicit request to see it.
					a.setTranslationVisible(m, true)
					return
				}
				a.loadTranslation()
			}
		}
	})
}

// spokenLanguageSelect overrules the detected language.
func (a *App) spokenLanguageSelect(c *ui.Context, t *ui.Theme, m library.Media) {
	names, codes := a.langChoices()
	lang := a.langLabel(a.lang)
	options := make([]kit.SelectOption[string], len(names))
	for i, name := range names {
		options[i] = kit.SelectOption[string]{Value: name, Label: name}
	}
	kit.CompactSelect(c, t, &lang, options, a.tr("Spoken language"), 140, nil, func(choice string) {
		for i, name := range names {
			if name == choice && codes[i] != a.lang {
				a.setLanguage(m.ID, codes[i])
			}
		}
	})
}

// transcriptSearchButton is kept for the live pane: the magnifier that opens
// the search field.
func (a *App) transcriptSearchButton(c *ui.Context, t *ui.Theme) {
	kit.IconButton(c, t, iconSearch, a.tr("Search the subtitles"), func() {
		a.segSearchOpen = !a.segSearchOpen
		if !a.segSearchOpen {
			a.segQuery = ""
		}
	})
}

// ---------------------------------------------------------------- summary

func (a *App) summaryTab(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	if m.TranscriptStatus != "done" {
		a.emptyTab(c, t, "A transcript comes first",
			"Summarize this file once it has been transcribed.",
			[]action{{"Go to Transcript", true, func() { a.tab = tabTranscript }}})
		return
	}
	if m.SummaryStatus == "pending" || a.jobRunning(jobs.KindSummarize) {
		a.working(c, t, "Summarizing", "Asking the AI model for the overview, key points and chapters.")
		return
	}
	if !a.sumLoaded {
		if !a.sumLoading {
			a.loadSummary()
		}
		a.working(c, t, "Loading", "Reading the summary.")
		return
	}
	if a.sum.ContentMarkdown == "" {
		a.emptyTab(c, t, "No summary yet",
			"Generate an overview, key points, action items, chapters and highlights from the transcript.",
			[]action{{"Generate", true, func() { a.startSummary(m.ID) }}})
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(16, 20, 20, 20).Gap(18).Children(func() {
			if a.sum.Overview != "" {
				ui.Column(c).Gap(8).Children(func() {
					kit.Label(c, t, a.tr("Overview"))
					ui.Text(c, a.sum.Overview).FontSize(13).LineHeight(1.6).TextColor(p.Ink).Selectable()
				})
			}
			if len(a.sum.KeyPoints) > 0 {
				ui.Column(c).Gap(8).Children(func() {
					kit.Label(c, t, a.tr("Key points"))
					for i, point := range a.sum.KeyPoints {
						numbered(c.Key(i), t, fmt.Sprintf("%02d", i+1), point)
					}
				})
			}
			if len(a.sum.ActionItems) > 0 {
				ui.Column(c).Gap(0).Children(func() {
					ui.Row(c).Padding(0, 0, 8, 0).Children(func() {
						kit.Label(c, t, a.tr("Action items")).Grow(1)
						mono(ui.Text(c, fmt.Sprintf("%d", len(a.sum.ActionItems)))).FontSize(10.5).TextColor(p.Ink3)
					})
					for i, item := range a.sum.ActionItems {
						ui.Row(c.Key(i)).FillWidth().Padding(8, 0).Gap(10).AlignItems(ui.Center).
							BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
							ui.Box(c).Size(16, 16).Radius(4).Border(1, p.Line2).Shrink(0)
							ui.Text(c, item).FontSize(13).TextColor(p.Ink).Grow(1).Selectable()
						})
					}
				})
			}
			if len(a.sum.Chapters) > 0 {
				ui.Column(c).Gap(2).Children(func() {
					kit.Label(c, t, a.tr("Chapters"))
					for i, ch := range a.sum.Chapters {
						a.timedRow(c.Key(i), t, m, ch.StartMs, ch.Title, ch.Summary)
					}
				})
			}
			if len(a.sum.Highlights) > 0 {
				ui.Column(c).Gap(8).Children(func() {
					kit.Label(c, t, a.tr("Highlights"))
					for i, h := range a.sum.Highlights {
						a.highlight(c.Key(i), t, m, h.StartMs, h.Note)
					}
				})
			}
		})
	})
	a.docFooter(c, t, func() {
		ui.Icon(c, iconSparkles).FontSize(14).TextColor(p.Ink3)
		ui.Text(c, a.modelName(a.sum.Model, m.SummaryStatus)).FontSize(11.5).TextColor(p.Ink3).
			SingleLine().Grow(1).MinWidth(0)
		kit.IconButton(c, t, iconCopy, a.tr("Copy the summary"), func() {
			mygo.Clipboard.WriteText(a.sum.ContentMarkdown)
		})
		if kit.Button(c, t, kit.Secondary, iconRefresh, a.tr("Generate again"), false).Clicked() {
			a.startSummary(m.ID)
		}
	})
}

// numbered is a key point: its mono number and the text.
func numbered(c *ui.Context, t *ui.Theme, n, text string) {
	p := pal(t)
	ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
		mono(ui.Text(c, n)).FontSize(11).TextColor(p.Ink3).Padding(2, 0, 0, 0).Shrink(0)
		ui.Text(c, text).FontSize(13).TextColor(p.Ink).Grow(1).Selectable()
	})
}

// timedRow is a chapter: its start, which seeks the player there, beside its
// title and what it covers.
func (a *App) timedRow(c *ui.Context, t *ui.Theme, m library.Media, at int64, title, detail string) {
	p := pal(t)
	clock := fmtDuration(at)
	row := ui.ButtonBase(c).Label(a.tr("Seek to ")+clock).Tooltip(a.tr("Seek to ")+clock).FillWidth().
		Padding(6, 8).Radius(kit.RadiusControl).Justify(ui.Start).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(p.Sunk)
	}
	row.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
			mono(ui.Text(c, clock)).FontSize(11.5).TextColor(p.AmberInk).Width(44).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(13).TextColor(p.Ink)
				if detail != "" {
					ui.Text(c, detail).FontSize(12).TextColor(p.Ink3)
				}
			})
		})
	})
	if row.Clicked() {
		a.seekPlayback(m, time.Duration(at)*time.Millisecond)
	}
}

// highlight is a key moment as a quote in the highlighter, its time under it
// seeking there.
func (a *App) highlight(c *ui.Context, t *ui.Theme, m library.Media, at int64, note string) {
	p := pal(t)
	clock := fmtDuration(at)
	quote := ui.ButtonBase(c).Label(a.tr("Seek to ")+clock).Tooltip(a.tr("Seek to ")+clock).FillWidth().
		Padding(12, 14).Radius(0, kit.RadiusControl, kit.RadiusControl, 0).BorderWidth(0, 0, 0, 3).
		BorderColor(p.Amber).Background(p.AmberSoft).Justify(ui.Start).Cursor(ui.CursorPointer)
	quote.Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "“"+note+"”").FontSize(13).TextColor(p.Ink).Selectable()
			mono(ui.Text(c, clock)).FontSize(11.5).TextColor(p.AmberInk)
		})
	})
	if quote.Clicked() {
		a.seekPlayback(m, time.Duration(at)*time.Millisecond)
	}
}

// ---------------------------------------------------------------- shared

// emptyTab says why a tab has nothing, and offers what would fill it.
func (a *App) emptyTab(c *ui.Context, t *ui.Theme, title, detail string, actions []action) {
	p := pal(t)
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(8).Children(func() {
		ui.Text(c, a.tr(title)).FontSize(16).FontWeight(600).TextColor(p.Ink).TextAlign(ui.Center)
		ui.Text(c, a.tr(detail)).FontSize(13).TextColor(p.Ink3).MaxWidth(340).TextAlign(ui.Center)
		ui.Row(c).Gap(8).Padding(6, 0, 0, 0).Children(func() {
			for _, act := range actions {
				if !act.enabled {
					continue
				}
				if kit.Button(c, t, kit.Primary, nil, a.tr(act.label), false).Clicked() {
					act.onChoose()
				}
			}
		})
	})
}

// working shows what a long job is doing, how far it has come, and how to
// stop it.
func (a *App) working(c *ui.Context, t *ui.Theme, title, detail string) {
	p := pal(t)
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(8).Children(func() {
		ui.Spinner(c).Label(a.tr(title))
		ui.Text(c, a.tr(title)+"…").FontSize(13).FontWeight(500).TextColor(p.Ink)
		ui.Text(c, a.tr(detail)).FontSize(12.5).TextColor(p.Ink3).MaxWidth(340).TextAlign(ui.Center)
		if !a.job.Running() {
			return
		}
		value := a.job.Progress
		if value <= 0 {
			value = -1
		}
		progressBar(c, t, value, 240)
		if kit.Button(c, t, kit.Secondary, nil, a.tr("Cancel"), false).Label(a.tr("Stop this job")).Clicked() {
			a.cancelJob()
		}
	})
}

// progressBar is the 6px amber bar of a job; a negative value is a bar of
// unknown length.
func progressBar(c *ui.Context, t *ui.Theme, value float64, width float32) {
	p := pal(t)
	if value < 0 {
		ui.Progress(c, -1).Width(width)
		return
	}
	bar := ui.Box(c).Height(6).Radius(3).Background(p.Hover)
	if width > 0 {
		bar.Width(width)
	} else {
		bar.FillWidth()
	}
	fraction := float32(min(max(value, 0), 1))
	bar.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * fraction, H: r.H}, p.Amber, 3)
	})
}

// langs are the languages Ottr names. spoken is true for the ones the
// recognizer can be told to listen for; every one of them can be a translation
// target.
var langs = []struct {
	code, name string
	spoken     bool
}{
	{"en", "English", true},
	{"zh", "中文", true},
	{"ja", "日本語", true},
	{"ko", "한국어", false},
	{"es", "Español", false},
	{"fr", "Français", false},
	{"de", "Deutsch", false},
	{"it", "Italiano", false},
	{"pt", "Português", false},
	{"ru", "Русский", false},
	{"ar", "العربية", false},
	{"hi", "हिन्दी", false},
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
// select shows them: what was detected first, then the ones the recognizer
// knows.
func (a *App) langChoices() (names, codes []string) {
	names = append(names, a.tr("Auto"))
	codes = append(codes, asr.DefaultLanguage)
	for _, l := range langs {
		if !l.spoken {
			continue
		}
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
		status := a.tr("Transcribing")
		if a.job.Running() && a.job.MediaID == m.ID && a.job.Progress > 0 {
			status += a.trf(" · %d%%", int(a.job.Progress*100))
		}
		parts = append(parts, status)
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
	// The full pass replaces the live lines; keeping them would show two
	// transcripts for one recording once it lands.
	if a.provisionalID == id {
		a.provisional, a.provisionalID = nil, 0
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

// ---------------------------------------------------------------- summary

func (a *App) modelName(model, status string) string {
	if model != "" {
		return model
	}
	if status == "error" {
		return a.tr("the last attempt failed")
	}
	return a.tr("not generated")
}

func (a *App) loadSummary() {
	id := a.sel
	a.sumLoading = true
	go func() {
		s, err := a.Sum.Get(id)
		a.update(func() {
			if id != a.sel {
				return
			}
			a.sumLoading = false
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
