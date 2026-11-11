package native

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/library"
	"ottr/internal/native/kit"
)

// modal is a dialog of the system: a sheet of the given width over a dimmed
// window, 22px padding, a 12px radius and the one shadow. Clicking outside
// it or Escape sets *open to false.
func (a *App) modal(c *ui.Context, t *ui.Theme, open *bool, width float32, build func()) {
	p := pal(t)
	ui.DialogBase(c, open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(10, 12, 14, 0.45))
		panel.Width(width).Padding(22).Gap(16).
			Radius(kit.RadiusPanel).Background(p.Sheet).Border(1, p.Line)
		kit.DialogShadow(panel, t)
		build()
	})
}

// dialogHead is the top of a dialog: a 36px tinted tile with its icon, the
// title and what it means, and a close button when close is set.
func dialogHead(c *ui.Context, t *ui.Theme, icon *ui.SVG, danger bool, title, detail string, close *bool) {
	p := pal(t)
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
		if icon != nil {
			fill, ink := p.AccentSoft, p.AccentInk
			if danger {
				fill, ink = p.RedSoft, p.Red
			}
			ui.Column(c).Size(36, 36).Radius(kit.RadiusCard).Center().Shrink(0).Background(fill).
				Children(func() { ui.Icon(c, icon).FontSize(18).TextColor(ink) })
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, title).FontSize(16).FontWeight(600).TextColor(p.Ink)
			if detail != "" {
				ui.Text(c, detail).FontSize(13).TextColor(p.Ink2)
			}
		})
		if close != nil {
			kit.IconButton(c, t, iconClose, "Close", func() { *close = false })
		}
	})
}

// dialogFooter is the row of a dialog's buttons, right-aligned, with
// something quiet on the left when left is set.
func (a *App) dialogFooter(c *ui.Context, t *ui.Theme, left, right func()) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(4, 0, 0, 0).Children(func() {
		if left != nil {
			left()
		}
		ui.Spacer(c)
		right()
	})
}

// ------------------------------------------------------------------ delete

// askDelete opens the delete dialog for one media.
func (a *App) askDelete(id int64) {
	a.delID, a.confirmDel, a.keepText = id, true, false
}

// deleteDialog confirms deleting a media: the recording's file goes with it,
// an imported file stays where the user keeps it, and the transcript can be
// kept as text.
func (a *App) deleteDialog(c *ui.Context, t *ui.Theme) {
	if !a.confirmDel {
		return
	}
	m, ok := a.byID(a.delID)
	if !ok {
		a.confirmDel = false
		return
	}
	imported := m.Source == "import"
	p := pal(t)
	a.modal(c, t, &a.confirmDel, 440, func() {
		detail := a.tr("The video, transcript and summary are removed from this computer. This can't be undone.")
		if imported {
			detail = a.tr("The transcript and summary are removed from the library. The original file stays where it is.")
		}
		title := a.tr("Delete this recording?")
		if imported {
			title = a.tr("Remove this file from the library?")
		}
		dialogHead(c, t, iconTrash, true, title, detail, nil)
		ui.Row(c).FillWidth().Gap(10).Padding(10, 12).Radius(kit.RadiusCard).
			Background(p.Sunk).Border(1, p.Line).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconFilm).FontSize(16).TextColor(p.Ink3)
			ui.Text(c, m.Name).FontSize(13).FontWeight(500).SingleLine().Grow(1).MinWidth(0)
			facts := durationOrDash(m)
			if m.Size > 0 {
				facts += " · " + fmtSize(m.Size)
			}
			mono(ui.Text(c, facts)).FontSize(11.5).TextColor(p.Ink3)
		})
		if m.TranscriptStatus == "done" {
			ui.Checkbox(c, &a.keepText, a.tr("Keep the transcript as a .txt file"))
		}
		a.dialogFooter(c, t, nil, func() {
			if kit.Button(c, t, kit.Secondary, nil, a.tr("Cancel"), false).Clicked() {
				a.confirmDel = false
			}
			label := a.tr("Delete recording")
			if imported {
				label = a.tr("Remove")
			}
			if kit.Button(c, t, kit.Danger, iconTrash, label, false).Clicked() {
				a.confirmDel = false
				a.deleteMedia(m, !imported, a.keepText && m.TranscriptStatus == "done")
			}
		})
	})
}

// deleteMedia removes a media, writing its transcript beside the file first
// when asked; a transcript that cannot be kept stops the deletion.
func (a *App) deleteMedia(m library.Media, withFile, keepText bool) {
	if !keepText {
		a.removeMedia(m.ID, withFile)
		return
	}
	path := strings.TrimSuffix(m.Path, filepath.Ext(m.Path)) + ".txt"
	go func() {
		err := a.ASR.ExportTo(m.ID, "txt", asr.ExportOptions{Speakers: true, Timestamps: true}, path)
		a.update(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.removeMedia(m.ID, withFile)
		})
	}()
}

// ------------------------------------------------------------------ export

// exportFormats are the transcript's formats; exportTransFormats the
// translation's. The summary saves as Markdown.
var exportFormats = []struct{ ext, name, detail string }{
	{"srt", "SRT subtitles", ".srt · timed lines for any player"},
	{"txt", "Plain text", ".txt · one line per segment"},
	{"md", "Markdown", ".md · with the summary on top"},
	{"docx", "Word document", ".docx · with speakers"},
}

var exportTransFormats = []struct{ ext, name, detail string }{
	{"srt", "SRT subtitles", ".srt · the translated lines, timed"},
	{"bilingual_srt", "Bilingual SRT", ".srt · both languages, line under line"},
	{"txt", "Plain text", ".txt · the translated lines"},
}

// openExport shows the export dialog for the media shown.
func (a *App) openExport(m library.Media) {
	if a.exportFormat == "" {
		a.exportFormat, a.exportSpeakers, a.exportTimestamps = "srt", true, true
	}
	if a.exportTransFormat == "" {
		a.exportTransFormat = "srt"
	}
	a.exportKind = 0
	a.exportTranslation = a.showTranslation && len(a.trans.Segments) > 0
	a.exportOpen = true
	if m.SummaryStatus == "done" && !a.sumLoaded {
		a.loadSummary() // the Markdown file opens with it
	}
}

func (a *App) exportDialog(c *ui.Context, t *ui.Theme) {
	if !a.exportOpen {
		return
	}
	m, ok := a.current()
	if !ok || len(a.segs) == 0 {
		a.exportOpen = false
		return
	}
	p := pal(t)
	a.modal(c, t, &a.exportOpen, 460, func() {
		dialogHead(c, t, nil, false, a.tr("Export"),
			a.trf("%s · %d segments · %d speakers", m.Name, len(a.segs), speakerCount(a.segs)), &a.exportOpen)
		a.exportKinds(c, t)
		ext := a.exportFormat
		switch a.exportKind {
		case 1:
			if len(a.trans.Segments) == 0 {
				ui.Text(c, a.tr("Translate the transcript first")).FontSize(13).TextColor(p.Ink3).
					Padding(12, 0)
			}
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for _, f := range exportTransFormats {
					a.formatOption(c, t, &a.exportTransFormat, f.ext, f.name, f.detail)
				}
			})
			ext = strings.TrimPrefix(a.exportTransFormat, "bilingual_")
		case 2:
			if a.sum.ContentMarkdown == "" {
				ui.Text(c, a.tr("Generate the summary first")).FontSize(13).TextColor(p.Ink3).Padding(12, 0)
			}
			md := "md"
			a.formatOption(c, t, &md, "md", "Markdown", ".md · overview, chapters, key points")
			ext = "md"
		default:
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for _, f := range exportFormats {
					a.formatOption(c, t, &a.exportFormat, f.ext, f.name, f.detail)
				}
			})
			ui.Column(c).FillWidth().Children(func() {
				transWhy := ""
				if len(a.trans.Segments) == 0 {
					transWhy = a.tr("Translate the transcript first")
				}
				transLabel := a.tr("Include translation")
				if name := langName(a.trLang); name != "" {
					transLabel = a.trf("Include %s under each line", name)
				}
				a.exportSwitch(c, t, transLabel, &a.exportTranslation, transWhy)
				a.exportSwitch(c, t, "Speaker names", &a.exportSpeakers, "")
				timesWhy := ""
				if a.exportFormat == "srt" {
					timesWhy = a.tr("Subtitles are always timed")
				}
				a.exportSwitch(c, t, "Timestamps", &a.exportTimestamps, timesWhy)
			})
		}
		a.dialogFooter(c, t, func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
				ui.Icon(c, iconFolderOpen).FontSize(14).TextColor(p.Ink3)
				ui.Text(c, shortDir(filepath.Dir(m.Path))).FontSize(12).TextColor(p.Ink3).SingleLine().
					MinWidth(0).Tooltip(filepath.Dir(m.Path))
			})
		}, func() {
			if kit.Button(c, t, kit.Secondary, nil, a.tr("Cancel"), false).Clicked() {
				a.exportOpen = false
			}
			ready := a.exportKind == 0 || a.exportKind == 1 && len(a.trans.Segments) > 0 ||
				a.exportKind == 2 && a.sum.ContentMarkdown != ""
			if kit.Button(c, t, kit.Primary, iconDownload, a.trf("Export .%s", ext), false).Disabled(!ready).Clicked() && ready {
				a.exportOpen = false
				switch a.exportKind {
				case 1:
					a.exportTranslated(m)
				case 2:
					a.exportSummary(m.ID)
				default:
					a.runExport(m)
				}
			}
		})
	})
}

// exportKinds is the segmented control over what is exported.
func (a *App) exportKinds(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Row(c).FillWidth().Height(34).Padding(3).Gap(2).Radius(8).Background(p.Hover).Children(func() {
		for i, name := range []string{"Transcript", "Translation", "Summary"} {
			seg := ui.ButtonBase(c.Key(i)).Label(a.tr(name)).Role(ui.RoleTab).Checked(a.exportKind == i).
				Grow(1).Width(0).FillHeight().Radius(kit.RadiusControl).Center().Cursor(ui.CursorPointer)
			if a.exportKind == i {
				seg.Background(p.Sheet).Border(1, p.Line).TextColor(p.Ink)
			} else {
				seg.TextColor(p.Ink2)
			}
			seg.Children(func() { ui.Text(c, a.tr(name)).FontSize(13).FontWeight(500) })
			if seg.Clicked() {
				a.exportKind = i
			}
		}
	})
}

// formatOption is one format, a radio row chosen by a click.
func (a *App) formatOption(c *ui.Context, t *ui.Theme, chosen *string, ext, name, detail string) {
	p := pal(t)
	on := *chosen == ext
	card := ui.ButtonBase(c.Key(ext)).Label(a.tr(name)).Role(ui.RoleRadio).Checked(on).
		FillWidth().Padding(10, 14).Gap(14).Radius(kit.RadiusCard).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Background(p.Sheet)
	if on {
		card.Border(1, p.Ink).InsetShadow(0, 0, 0, 1, p.Ink)
	} else {
		card.Border(1, p.Line)
		if card.Hovered() {
			card.Background(p.Sunk)
		}
	}
	card.Children(func() {
		radio(c, t, on)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, a.tr(name)).FontSize(13).FontWeight(500).TextColor(p.Ink)
			mono(ui.Text(c, a.tr(detail))).FontSize(11).TextColor(p.Ink3).SingleLine()
		})
	})
	if card.Clicked() {
		*chosen = ext
	}
}

// radio is the round mark of a choice: ink with a white dot when on.
func radio(c *ui.Context, t *ui.Theme, on bool) {
	p := pal(t)
	mark := ui.Column(c).Size(16, 16).Radius(8).Center().Shrink(0)
	if on {
		mark.Background(p.Ink).Children(func() { ui.Box(c).Size(6, 6).Radius(3).Background(p.Sheet) })
		return
	}
	mark.Border(1, p.Line2).Background(p.Sheet)
}

// exportSwitch is one option row; why, when set, says why it is off.
func (a *App) exportSwitch(c *ui.Context, t *ui.Theme, label string, on *bool, why string) {
	p := pal(t)
	label = a.tr(label)
	ui.Row(c.Key(label)).FillWidth().Padding(6, 0).AlignItems(ui.Center).Children(func() {
		text := ui.Text(c, label).FontSize(13).TextColor(p.Ink).Grow(1)
		sw := kit.Switch(c, t, on, label)
		if why != "" {
			text.TextColor(p.Ink4)
			sw.Disabled(true).Tooltip(why)
		}
	})
}

// exportTranslated saves the translation in the format chosen.
func (a *App) exportTranslated(m library.Media) {
	lang, format := a.trLang, a.exportTransFormat
	go func() {
		if _, err := a.Trans.Export(context.Background(), m.ID, lang, format); err != nil {
			a.update(func() { a.setError(err) })
		}
	}()
}

// exportOptions are the dialog's choices as the transcript service takes them.
func (a *App) exportOptions(m library.Media) asr.ExportOptions {
	opts := asr.ExportOptions{
		Speakers: a.exportSpeakers, Timestamps: a.exportTimestamps, Title: m.Name,
	}
	if a.exportTranslation && len(a.trans.Segments) > 0 {
		opts.Translation = map[int64]string{}
		for _, seg := range a.trans.Segments {
			opts.Translation[seg.SegmentID] = seg.TranslatedText
		}
	}
	if a.exportFormat == "md" {
		opts.Preamble = a.sum.ContentMarkdown
	}
	return opts
}

func (a *App) runExport(m library.Media) {
	format, opts := a.exportFormat, a.exportOptions(m)
	go func() {
		if _, err := a.ASR.ExportWith(context.Background(), m.ID, format, opts); err != nil {
			a.update(func() { a.setError(err) })
		}
	}()
}

// speakerCount is how many voices a transcript tells apart.
func speakerCount(segs []asr.Segment) int {
	seen := map[int]bool{}
	for _, seg := range segs {
		seen[seg.Speaker] = true
	}
	return len(seen)
}

// ------------------------------------------------------------------ failed

// failedDialog says a transcription stopped, where, and what to do next:
// retry, choose another model, or leave it for later.
func (a *App) failedDialog(c *ui.Context, t *ui.Theme) {
	if !a.failedOpen {
		return
	}
	m, ok := a.byID(a.failedJob.MediaID)
	if !ok {
		a.failedOpen = false
		return
	}
	p := pal(t)
	a.modal(c, t, &a.failedOpen, 460, func() {
		meta := []string{m.Name, durationOrDash(m)}
		if model := a.modelInUseName(); model != "" {
			meta = append(meta, model)
		}
		dialogHead(c, t, iconAlertTriangle, true, a.tr("Transcription failed"), strings.Join(meta, " · "), nil)
		if a.failedJob.Error != "" {
			mono(ui.Text(c, a.failedJob.Error)).FontSize(11.5).TextColor(p.Ink).Selectable().
				Padding(10, 12).Radius(kit.RadiusControl).Background(p.Sunk).Border(1, p.Line).FillWidth()
		}
		detail := a.trf("“%s” could not be transcribed. The video is safe — only the text is missing.", m.Name)
		if at, ok := stoppedAt(a.failedJob, m); ok {
			detail = a.trf("“%s” stopped at %s of %s. The video is safe — only the text is incomplete.",
				m.Name, fmtDuration(at), fmtDuration(m.DurationMs))
		}
		ui.Text(c, detail+" "+a.tr("Check that system audio or the microphone was on while recording, or try another model in Settings › Transcription.")).
			FontSize(12.5).TextColor(p.Ink2)
		a.dialogFooter(c, t, func() {
			if kit.Button(c, t, kit.Ghost, nil, a.tr("Dismiss"), false).Clicked() {
				a.failedOpen = false
			}
		}, func() {
			if kit.Button(c, t, kit.Secondary, nil, a.tr("Open settings"), false).Clicked() {
				a.failedOpen = false
				a.settingsTab = 1
				a.openSettings()
			}
			if kit.Button(c, t, kit.Primary, iconRefresh, a.tr("Try again"), false).Clicked() {
				a.failedOpen = false
				a.startTranscribe(m.ID)
			}
		})
	})
}
