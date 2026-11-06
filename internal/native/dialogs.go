package native

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/library"
)

// dialogHead is the top of a confirmation: a tinted icon beside the title and
// what it means.
func dialogHead(c *ui.Context, t *ui.Theme, icon *ui.SVG, tint ui.Color, title, detail string) {
	ui.Row(c).Gap(t.Space(3.5)).AlignItems(ui.Start).MaxWidth(420).Children(func() {
		ui.Column(c).Size(36, 36).Radius(t.Radius + 2).Center().Shrink(0).Background(tint.Alpha(0.12)).
			Children(func() { ui.Icon(c, icon).FontSize(t.FontSize + 5).TextColor(tint) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
			ui.Text(c, title).FontSize(t.FontSize + 3).Bold()
			ui.Text(c, detail).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		})
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
	ui.Modal(c, &a.confirmDel, func() {
		ui.Column(c).Gap(t.Space(4)).Width(400).Children(func() {
			detail := a.tr("The video, transcript and summary are removed from this computer. This can't be undone.")
			if imported {
				detail = a.tr("The transcript and summary are removed from the library. The original file stays where it is.")
			}
			title := a.tr("Delete this recording?")
			if imported {
				title = a.tr("Remove this file from the library?")
			}
			dialogHead(c, t, iconTrash, t.Danger, title, detail)
			ui.Row(c).FillWidth().Gap(t.Space(2.5)).Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius + 2).
				Background(t.Surface).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconPlay).FontSize(t.FontSize + 1).TextColor(t.TextMuted)
				ui.Text(c, m.Name).FontSize(t.FontSize - 1).Bold().SingleLine().Grow(1).MinWidth(0)
				facts := durationOrDash(m)
				if m.Size > 0 {
					facts += " · " + fmtSize(m.Size)
				}
				ui.Text(c, facts).FontSize(t.FontSize - 2).TextColor(t.TextMuted).FontFeatures("tnum")
			})
			if m.TranscriptStatus == "done" {
				ui.Checkbox(c, &a.keepText, a.tr("Keep the transcript as a .txt file"))
			}
			ui.Row(c).Gap(t.Space(2)).Justify(ui.End).Children(func() {
				if ui.Button(c, a.tr("Cancel")).Clicked() {
					a.confirmDel = false
				}
				label := a.tr("Delete recording")
				if imported {
					label = a.tr("Remove")
				}
				del := ui.ButtonBase(c).Label(label).Height(t.Space(9)).Padding(0, t.Space(4)).Radius(9999).
					Background(t.Danger).TextColor(ui.RGBA(255, 255, 255, 1)).Cursor(ui.CursorPointer)
				del.Children(func() { ui.Text(c, label).FontSize(t.FontSize - 1).Bold() })
				if del.Hovered() {
					del.Background(t.Danger.Alpha(0.88))
				}
				if del.Clicked() {
					a.confirmDel = false
					a.deleteMedia(m, !imported, a.keepText && m.TranscriptStatus == "done")
				}
			})
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

// exportFormats are the dialog's four choices.
var exportFormats = []struct{ ext, name, detail string }{
	{"srt", "SRT subtitles", "Timed, for video players"},
	{"txt", "Plain text", ".txt, one line per segment"},
	{"md", "Markdown", "Summary + transcript"},
	{"docx", "Word document", ".docx with speakers"},
}

// openExport shows the export dialog for the media shown.
func (a *App) openExport(m library.Media) {
	if a.exportFormat == "" {
		a.exportFormat, a.exportSpeakers, a.exportTimestamps = "srt", true, true
	}
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
	ui.Modal(c, &a.exportOpen, func() {
		ui.Column(c).Gap(t.Space(3)).Width(420).Children(func() {
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, a.tr("Export transcript")).FontSize(t.FontSize + 3).Bold()
				ui.Text(c, a.trf("%s · %d segments · %d speakers", m.Name, len(a.segs), speakerCount(a.segs))).
					FontSize(t.FontSize - 1).TextColor(t.TextMuted).SingleLine()
			})
			for i := 0; i < len(exportFormats); i += 2 {
				ui.Row(c.Key(i)).Gap(t.Space(2)).Children(func() {
					for _, f := range exportFormats[i : i+2] {
						a.formatCard(c, t, f.ext, f.name, f.detail)
					}
				})
			}
			ui.Column(c).Children(func() {
				a.exportSwitch(c, t, "Speaker names", &a.exportSpeakers, "")
				timesWhy := ""
				if a.exportFormat == "srt" {
					timesWhy = a.tr("Subtitles are always timed")
				}
				a.exportSwitch(c, t, "Timestamps", &a.exportTimestamps, timesWhy)
				transWhy := ""
				if len(a.trans.Segments) == 0 {
					transWhy = a.tr("Translate the transcript first")
				}
				transLabel := a.tr("Include translation")
				if name := langName(a.trLang); name != "" {
					transLabel = a.trf("Include %s translation", name)
				}
				a.exportSwitch(c, t, transLabel, &a.exportTranslation, transWhy)
			})
			ui.Row(c).Gap(t.Space(2)).Justify(ui.End).Children(func() {
				if ui.Button(c, a.tr("Cancel")).Clicked() {
					a.exportOpen = false
				}
				if ui.PrimaryButton(c, a.trf("Export .%s", a.exportFormat)).Clicked() {
					a.exportOpen = false
					a.runExport(m)
				}
			})
		})
	})
}

// formatCard is one of the four formats, chosen by a click.
func (a *App) formatCard(c *ui.Context, t *ui.Theme, ext, name, detail string) {
	chosen := a.exportFormat == ext
	card := ui.ButtonBase(c.Key(ext)).Label(a.tr(name)).Role(ui.RoleRadio).Checked(chosen).
		Grow(1).Width(0).Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius + 2).BorderWidth(1).
		Cursor(ui.CursorPointer)
	if chosen {
		card.BorderColor(t.Accent).Background(t.Accent.Alpha(0.08))
	} else {
		card.BorderColor(t.Border)
		if card.Hovered() {
			card.Background(t.SurfaceHover)
		}
	}
	card.Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
			ui.Text(c, a.tr(name)).FontSize(t.FontSize - 1).Bold()
			ui.Text(c, a.tr(detail)).FontSize(t.FontSize - 3).TextColor(t.TextMuted).SingleLine()
		})
	})
	if card.Clicked() {
		a.exportFormat = ext
	}
}

// exportSwitch is one option row; why, when set, says why it is off.
func (a *App) exportSwitch(c *ui.Context, t *ui.Theme, label string, on *bool, why string) {
	label = a.tr(label)
	ui.Row(c.Key(label)).Padding(t.Space(2), 0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).Children(func() {
		text := ui.Text(c, label).FontSize(t.FontSize - 1).Grow(1)
		sw := ui.Switch(c, on).Label(label)
		if why != "" {
			text.TextColor(t.TextMuted)
			sw.Disabled(true).Tooltip(why)
		}
	})
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
	ui.Modal(c, &a.failedOpen, func() {
		ui.Column(c).Gap(t.Space(4)).Width(420).Children(func() {
			detail := a.trf("“%s” could not be transcribed. The video is safe — only the text is missing.", m.Name)
			if at, ok := stoppedAt(a.failedJob, m); ok {
				detail = a.trf("“%s” stopped at %s of %s. The video is safe — only the text is incomplete.",
					m.Name, fmtDuration(at), fmtDuration(m.DurationMs))
			}
			dialogHead(c, t, iconAlert, t.Warning, a.tr("Couldn’t finish the transcript"), detail)
			if a.failedJob.Error != "" {
				ui.Text(c, a.failedJob.Error).FontSize(t.FontSize-2).Font("monospace").Selectable().
					Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius + 2).Background(t.Surface).FillWidth()
			}
			ui.Row(c).Gap(t.Space(2)).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				change := ui.ButtonBase(c).Label(a.tr("Change model")).Padding(t.Space(2), t.Space(3)).
					Radius(t.Radius).TextColor(t.TextMuted).Cursor(ui.CursorPointer)
				change.Children(func() { ui.Text(c, a.tr("Change model")).FontSize(t.FontSize - 1).Bold() })
				if change.Hovered() {
					change.Background(t.SurfaceHover)
				}
				if change.Clicked() {
					a.failedOpen = false
					a.settingsTab = 1
					a.openSettings()
				}
				if ui.Button(c, a.tr("Later")).Clicked() {
					a.failedOpen = false
				}
				if ui.PrimaryButton(c, a.tr("Try again")).Clicked() {
					a.failedOpen = false
					a.startTranscribe(m.ID)
				}
			})
		})
	})
}
