package native

import (
	"bytes"
	"context"
	"image"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/native/kit"
)

// libraryPage is the library: its title and import, the search and the
// list/grid switch, the filter chips, then the media grouped by day.
func (a *App) libraryPage(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	width, _ := c.Size()
	narrow := layoutOf(width) == "narrow"
	firstRun := len(a.media) == 0 && strings.TrimSpace(a.query) == "" && a.filter == filterAll && !a.recording
	ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.Sheet).Children(func() {
		if firstRun {
			if narrow {
				ui.Row(c).Padding(12, 16).Children(func() {
					if a.backButton(c, t, "") {
						a.libraryOpen = false
					}
				})
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Padding(80, 24, 24, 24).Children(func() { a.emptyLibrary(c, t) })
			})
			return
		}
		ui.Column(c).FillWidth().Shrink(0).Padding(20, 24, 12, 24).Gap(12).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
				if narrow {
					if a.backButton(c, t, "") {
						a.libraryOpen = false
					}
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, a.tr(a.filterTitle())).FontSize(20).FontWeight(600).LetterSpacing(-0.3).
						TextColor(p.Ink).SingleLine()
					ui.Text(c, a.librarySummary()).FontSize(12).TextColor(p.Ink3).SingleLine()
				})
				imp := kit.Button(c, t, kit.Secondary, iconImport, a.tr("Import media"), false).
					Tooltip(a.tr("Add a media file to the library")).Disabled(a.recording)
				if imp.Clicked() && !a.recording {
					a.importFiles()
				}
			})
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				a.librarySearch(c, t)
				a.viewSwitch(c, t)
			})
			if !narrow {
				ui.Row(c).FillWidth().Gap(6).ClipX().Children(func() {
					for _, f := range []struct{ label, filter string }{
						{"All", filterAll}, {"Recordings", filterRecordings}, {"Imports", filterImports},
						{"Has summary", filterSummary}, {"Failed", filterAttention},
					} {
						if kit.FilterChip(c.Key(f.filter), t, a.tr(f.label), a.filter == f.filter).Clicked() {
							a.filter = f.filter
						}
					}
				})
			}
		})
		a.libraryList(c, t)
	})
}

// filterTitle is the page title of the view chosen.
func (a *App) filterTitle() string {
	switch a.filter {
	case filterRecordings:
		return "Recordings"
	case filterImports:
		return "Imports"
	case filterSummary:
		return "Has summary"
	case filterAttention:
		return "Failed"
	}
	return "All media"
}

// librarySummary is the line under the title: how many items, how long they
// run together, and whether a recording is under way.
func (a *App) librarySummary() string {
	items := a.visibleMedia()
	var total int64
	for _, m := range items {
		total += m.DurationMs
	}
	line := a.trf("%d items", len(items))
	if len(items) == 1 {
		line = a.tr("1 item")
	}
	if total > 0 {
		line += " · " + fmtHours(total)
	}
	if a.recording {
		line += " · " + a.tr("recording in progress")
	}
	return line
}

// fmtHours is a total length: "21 h 14 m", or "42 m" under an hour.
func fmtHours(ms int64) string {
	m := ms / 60000
	if m < 60 {
		return fmtMinutesShort(m)
	}
	return itoa(m/60) + " h " + itoa(m%60) + " m"
}

func fmtMinutesShort(m int64) string { return itoa(m) + " m" }

// librarySearch is the search field over titles and transcripts, with its
// shortcut.
func (a *App) librarySearch(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Row(c).Grow(1).MinWidth(0).Height(32).Padding(0, 6, 0, 10).Gap(8).AlignItems(ui.Center).
		Radius(kit.RadiusControl).Border(1, p.Line2).Background(p.Sheet).Children(func() {
		ui.Icon(c, iconSearch).FontSize(16).TextColor(p.Ink3).Shrink(0)
		field := ui.TextInputBase(c, &a.query).Label(a.tr("Search the library")).
			Placeholder(a.tr("Search titles and transcripts")).PlaceholderColor(p.Ink3).
			Grow(1).MinWidth(0).FontSize(13).TextColor(p.Ink)
		if field.Changed() {
			a.reload()
		}
		if c.Shortcut(ui.Ctrl, ui.KeyK) {
			field.Focus()
		}
		if field.Shortcut(0, ui.KeyEscape) && a.query != "" {
			a.query = ""
			a.reload()
		}
		ui.Row(c).Gap(3).Children(func() {
			kit.Kbd(c, t, "Ctrl K")
		})
	})
}

// viewSwitch is the list/grid segmented pair.
func (a *App) viewSwitch(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Row(c).Height(32).Padding(2).Gap(2).Radius(kit.RadiusControl).Border(1, p.Line2).
		Background(p.Sheet).Shrink(0).Children(func() {
		for _, v := range []struct {
			icon  *ui.SVG
			label string
			grid  bool
		}{{iconList, "List view", false}, {iconGrid, "Grid view", true}} {
			on := a.libraryGridView == v.grid
			btn := ui.ButtonBase(c.Key(v.label)).Label(a.tr(v.label)).Tooltip(a.tr(v.label)).
				Size(30, 26).Radius(4).Center().TextColor(p.Ink3).Cursor(ui.CursorPointer)
			if on {
				btn.Background(p.Hover).TextColor(p.Ink)
			} else if btn.Hovered() {
				btn.TextColor(p.Ink)
			}
			btn.Children(func() { ui.Icon(c, v.icon).FontSize(16) })
			if btn.Clicked() {
				a.libraryGridView = v.grid
			}
		}
	})
}

// libraryList lists media grouped by day, or as a grid, and any recording
// files no record holds.
func (a *App) libraryList(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	items := a.visibleMedia()
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(8, 12, 20, 12).Gap(2).Children(func() {
			if len(items) == 0 && !a.recording {
				a.emptyLibrary(c, t)
			} else {
				group, now := "", time.Now()
				if a.recording {
					group = a.dayGroup(now, now)
					a.groupHead(c, t, group)
					a.recordingRow(c, t)
				}
				if a.libraryGridView {
					ui.Row(c).Wrap().Gap(16).Padding(8, 12).AlignItems(ui.Start).Children(func() {
						for _, m := range items {
							a.mediaCard(c, t, m)
						}
					})
				} else {
					for _, m := range items {
						if g := a.dayGroup(now, m.CreatedAt); g != group {
							group = g
							a.groupHead(c.Key("group-"+g), t, g)
						}
						a.mediaRow(c, t, m)
					}
				}
			}
			if len(a.orphans) > 0 {
				a.orphanNotice(c, t)
			}
		})
	})
	_ = p
}

func (a *App) groupHead(c *ui.Context, t *ui.Theme, group string) {
	ui.Row(c).Padding(14, 12, 6, 12).Children(func() { kit.Label(c, t, a.groupLabel(group)) })
}

// recordingRow is the red-tinted row of the recording under way, on top of
// the list.
func (a *App) recordingRow(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Row(c).FillWidth().Padding(8, 12).Gap(14).Radius(kit.RadiusCard).Background(p.RedSoft).
		AlignItems(ui.Center).Children(func() {
		ui.Column(c).Width(96).Height(54).Shrink(0).Radius(kit.RadiusControl + 2).Background(p.Media).
			Center().Children(func() {
			ui.Box(c).Size(12, 12).Radius(6).Background(p.Red)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, a.tr("Recording now…")).FontSize(13.5).FontWeight(500).TextColor(p.Ink).SingleLine()
			mono(ui.Text(c, fmtClock(a.recordingElapsed().Milliseconds())+" · "+a.sourceName())).
				FontSize(11.5).TextColor(p.RedInk).SingleLine()
			if n := len(a.late); n > 0 {
				ui.Text(c, a.late[n-1].text).FontSize(12).TextColor(p.Ink2).SingleLine()
			}
		})
	})
}

// emptyLibrary says the library is empty, or that nothing matched, and offers
// the two ways to fill it: record, or import.
func (a *App) emptyLibrary(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	searching := strings.TrimSpace(a.query) != ""
	ui.Column(c).FillWidth().Padding(40, 24).AlignItems(ui.Center).Gap(10).Children(func() {
		if searching || a.filter != filterAll || len(a.media) > 0 {
			ui.Icon(c, iconSearch).FontSize(28).TextColor(p.Ink4)
			title := a.tr("Nothing in this view yet")
			if searching {
				title = a.trf("No results for “%s”", strings.TrimSpace(a.query))
			}
			ui.Text(c, title).FontSize(16).FontWeight(600).TextColor(p.Ink)
			ui.Text(c, a.tr("Try a different search.")).FontSize(13).TextColor(p.Ink3).TextAlign(ui.Center)
			return
		}
		emptyIllustration(c, t)
		ui.Text(c, a.tr("Record your first screen")).FontSize(20).FontWeight(600).LetterSpacing(-0.3).
			TextColor(p.Ink).Padding(8, 0, 0, 0)
		ui.Text(c, a.tr("Ottr saves the video and writes the transcript on this computer. Nothing leaves it unless you ask for a translation or a summary.")).
			FontSize(13).TextColor(p.Ink2).TextAlign(ui.Center).MaxWidth(400)
		model := "SenseVoice"
		if d := a.details.model; d != "" {
			model = shortModel(d)
		}
		ui.Row(c).Gap(12).Padding(16, 0).AlignItems(ui.Stretch).Children(func() {
			for i, step := range []struct{ title, detail string }{
				{a.tr("Record or import"), a.tr("Pick a screen on the right, or bring in a video you already have.")},
				{a.tr("Transcribe offline"), a.trf("%s turns speech into text, with speakers, on your CPU.", model)},
				{a.tr("Read and search"), a.trf("Jump to any line, translate into %s, and get action items.", langName("zh"))},
			} {
				ui.Column(c.Key(i)).Width(184).Padding(14).Gap(6).Radius(kit.RadiusCard).Border(1, p.Line).
					Background(p.Sheet).Children(func() {
					ui.Column(c).Size(20, 20).Radius(kit.RadiusControl).Center().Background(p.AmberSoft).Children(func() {
						mono(ui.Text(c, itoa(int64(i+1)))).FontSize(11).FontWeight(600).TextColor(p.AmberInk)
					})
					ui.Text(c, step.title).FontSize(13).FontWeight(600).TextColor(p.Ink).Padding(4, 0, 0, 0)
					ui.Text(c, step.detail).FontSize(12).TextColor(p.Ink3)
				})
			}
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if kit.Button(c, t, kit.Secondary, iconImport, a.tr("Import media"), false).Disabled(a.recording).Clicked() && !a.recording {
				a.importFiles()
			}
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				for _, key := range []string{"Ctrl", "Shift", "R"} {
					kit.Kbd(c, t, key)
				}
				ui.Text(c, a.tr("starts recording from anywhere")).FontSize(12).TextColor(p.Ink3).Padding(0, 0, 0, 4)
			})
		})
		if width, _ := c.Size(); layoutOf(width) != "wide" {
			start := kit.Button(c, t, kit.Primary, nil, a.tr("Start recording"), false).Disabled(a.busy || a.sourceID == "")
			if start.Clicked() && !a.busy && a.sourceID != "" {
				a.startRecording()
			}
		}
	})
}

// emptyIllustration is a screen with a transcript card beside it, its active
// line in the highlighter: what Ottr makes of a recording.
func emptyIllustration(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Row(c).Gap(0).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Size(200, 124).Radius(kit.RadiusCard).Background(p.Media).Padding(12).Children(func() {
			ui.Column(c).Fill().Radius(4).Background(ui.Hex("#FFFFFF")).Padding(10).Gap(7).Children(func() {
				for i, w := range []float32{70, 120, 100, 110} {
					bar := ui.Box(c.Key(i)).Size(w, 5).Radius(2).Background(ui.Hex("#D5DAE0"))
					if i == 0 {
						bar.Background(ui.Hex("#9AA3AE"))
					}
				}
			})
		})
		ui.Column(c).Width(150).Padding(10).Gap(8).Radius(kit.RadiusCard).Border(1, p.Line).
			Background(p.Sheet).Margin(0, 0, 0, -24).Children(func() {
			for i := 0; i < 3; i++ {
				row := ui.Row(c.Key(i)).Gap(6).Padding(4).Radius(4).AlignItems(ui.Center)
				if i == 1 {
					row.Background(p.AmberSoft).BorderWidth(0, 0, 0, 2).BorderColor(p.Amber)
				}
				row.Children(func() {
					ui.Box(c).Size(12, 12).Radius(6).Background(kit.SpeakerColors[i])
					ui.Column(c).Gap(3).Children(func() {
						ui.Box(c).Size(80, 4).Radius(2).Background(p.Line2)
						ui.Box(c).Size(56, 4).Radius(2).Background(p.Line)
					})
				})
			}
		})
	})
}

// mediaRow is one item of the library list: a thumbnail with its duration,
// the title, what it is, its first line, and what has been made of it.
func (a *App) mediaRow(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	id := m.ID
	card := ui.ButtonBase(c.Key(id)).Label(m.Name).FillWidth().MinHeight(72).Padding(8, 12).Gap(14).
		Radius(kit.RadiusCard).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	hovered := card.Hovered()
	if id == a.sel || hovered {
		card.Background(p.Sunk)
	}
	a.ensureRowFacts(m)
	facts := a.rowFacts[m.ID]
	card.Children(func() {
		ui.Column(c).Width(96).Shrink(0).Children(func() { a.mediaThumb(c, t, m) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, m.Name).FontSize(13.5).FontWeight(500).TextColor(p.Ink).SingleLine().
					Grow(1).MinWidth(0)
				ui.Row(c).Gap(4).Shrink(0).Children(func() { a.statusChips(c, t, m) })
			})
			meta := a.cardSubtitle(m)
			if facts.speakers > 0 {
				meta += " · " + a.speakersText(facts.speakers)
			}
			ui.Text(c, meta).FontSize(11.5).TextColor(p.Ink3).SingleLine()
			switch {
			case m.TranscriptStatus == "error":
				note := a.tr("Transcription failed")
				if at, ok := stoppedAt(a.lastJobs[m.ID], m); ok {
					note += " · " + a.trf("transcript stopped at %s", fmtDuration(at))
				}
				ui.Text(c, note).FontSize(12).TextColor(p.Ink2).SingleLine()
			case m.TranscriptStatus == "pending":
				ui.Text(c, a.tr("Transcribing on this computer…")).FontSize(12).TextColor(p.Ink3).SingleLine()
			case m.Source == "screenshot":
				ui.Text(c, a.tr("Still image, no transcript.")).FontSize(12).TextColor(p.Ink3).SingleLine()
			case facts.line != "":
				ui.Text(c, facts.line).FontSize(12).TextColor(p.Ink2).SingleLine()
			}
		})
		more := kit.IconButton(c, t, iconMoreH, a.tr("More actions"), nil)
		more.Menu(a.mediaMenuItems(m))
		if !hovered && id != a.sel {
			more.Opacity(0.6)
		}
	})
	if card.Clicked() {
		if m.TranscriptStatus == "error" {
			if job, ok := a.lastJobs[m.ID]; ok && job.Kind == jobs.KindTranscribe {
				a.openDetail(id)
				a.failedJob, a.failedOpen = job, true
				return
			}
		}
		a.openDetail(id)
	}
	a.mediaMenu(card, m)
}

func (a *App) speakersText(n int) string {
	if n == 1 {
		return a.tr("1 speaker")
	}
	return a.trf("%d speakers", n)
}

// statusChips say what has been made of a media: Transcribed, Summary,
// Failed, PNG.
func (a *App) statusChips(c *ui.Context, t *ui.Theme, m library.Media) {
	if m.Source == "screenshot" {
		kit.Chip(c, t, kit.ChipNeutral, "PNG")
		return
	}
	switch m.TranscriptStatus {
	case "done":
		kit.Chip(c, t, kit.ChipOK, a.tr("Transcribed"))
	case "pending":
		kit.Chip(c, t, kit.ChipAmber, a.tr("Transcribing"))
	case "error":
		kit.Chip(c, t, kit.ChipBad, a.tr("Failed"))
	}
	if m.SummaryStatus == "done" {
		kit.Chip(c, t, kit.ChipNeutral, a.tr("Summary"))
	}
}

const mediaCardWidth = 300

// mediaCard is one tile in grid view.
func (a *App) mediaCard(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	card := ui.ButtonBase(c.Key(m.ID)).Label(m.Name).Width(mediaCardWidth).Padding(6, 6, 10, 6).Gap(8).
		Radius(kit.RadiusPanel).Cursor(ui.CursorPointer)
	if m.ID == a.sel || card.Hovered() {
		card.Background(p.Sunk)
	}
	card.Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			a.mediaThumb(c, t, m)
			ui.Column(c).FillWidth().Gap(3).Padding(0, 4).Children(func() {
				ui.Text(c, m.Name).FontSize(13.5).FontWeight(500).TextColor(p.Ink).SingleLine()
				ui.Text(c, a.cardSubtitle(m)).FontSize(11.5).TextColor(p.Ink3).SingleLine()
				ui.Row(c).Gap(4).Padding(3, 0, 0, 0).Children(func() { a.statusChips(c, t, m) })
			})
		})
	})
	if card.Clicked() {
		a.openDetail(m.ID)
	}
	a.mediaMenu(card, m)
}

// mediaThumb shows a decoded video poster or screenshot on the media ground,
// with a source glyph while it loads or when the file cannot be decoded, and
// the duration in a mono badge.
func (a *App) mediaThumb(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	thumb := ui.Column(c).FillWidth().AspectRatio(16.0 / 9.0).Radius(kit.RadiusControl + 2).Clip().
		Background(p.Media)
	thumb.Children(func() {
		if picture := a.thumbs[m.ID]; picture != nil {
			ui.Image(c, picture).Fit(ui.Cover).Grow(1)
		} else {
			icon := iconPlay
			switch {
			case m.Source == "screenshot":
				icon = iconImage
			case !m.HasVideo:
				icon = iconWaveform
			}
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Icon(c, icon).FontSize(20).TextColor(onMedia()).Opacity(0.7)
			})
		}
		label := ""
		switch {
		case m.TranscriptStatus == "pending" || m.SummaryStatus == "pending":
			label = a.tr("Working…")
		case m.DurationMs > 0:
			label = fmtDuration(m.DurationMs)
		}
		if label != "" {
			ui.Row(c).Attach(ui.AnchorBottomRight, ui.AnchorBottomRight).Right(5).Bottom(5).
				Padding(1, 5).Radius(4).Background(ui.RGBA(0, 0, 0, 0.62)).Children(func() {
				mono(ui.Text(c, label)).FontSize(10.5).TextColor(onMedia()).SingleLine()
			})
		}
	})
}

// orphanNotice says that recordings on disk are not in the library, and offers
// the two things to do about it: add them back, or delete them.
func (a *App) orphanNotice(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Row(c).FillWidth().Padding(10, 14).Gap(10).Radius(kit.RadiusCard).Border(1, p.AmberLine).
		Background(p.AmberSoft).AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() {
		n := len(a.orphans)
		said := a.trf("%d recording files are not in the library.", n)
		if n == 1 {
			said = a.tr("1 recording file is not in the library.")
		}
		ui.Icon(c, iconAlertTriangle).FontSize(16).TextColor(p.AmberInk).Shrink(0)
		ui.Text(c, said).FontSize(12.5).TextColor(p.Ink).Grow(1).MinWidth(0)
		if kit.Button(c, t, kit.Secondary, nil, a.tr("Add back"), false).Tooltip(a.tr("Add these files to the library again")).Clicked() {
			a.addOrphans()
		}
		if kit.Button(c, t, kit.Ghost, nil, a.tr("Delete files…"), false).Tooltip(a.tr("Delete these files from disk")).Clicked() {
			a.confirmOrphans = true
		}
	})
}

// sourceSelectEmpty stands in for the select when there is nothing to choose.
func (a *App) sourceSelectEmpty(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string) {
	p := pal(t)
	ui.Row(c).FillWidth().Height(32).Gap(8).AlignItems(ui.Center).Padding(0, 10).
		Radius(kit.RadiusControl).Border(1, p.Line).Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(p.Ink4)
		ui.Text(c, a.tr(label)).FontSize(12).TextColor(p.Ink3).SingleLine()
	})
}

// sourceCardsOf returns the four capture modes; the select below chooses the
// concrete display or window. Area and Camera are not built yet.
func (a *App) sourceCardsOf(sources []capture.Source) []sourceCard {
	window := a.windowSource()
	windowCard := sourceCard{label: "Window", icon: iconWindow, id: window.ID, chosen: a.sourceMode() == "window"}
	if window.ID == "" {
		windowCard.blocked = "No visible window to record"
	}
	display := a.displaySource()
	displayCard := sourceCard{label: "Full screen", icon: iconScreen, id: display.ID, chosen: a.sourceMode() == "screen"}
	if display.ID == "" {
		displayCard.blocked = "No display to record"
	}
	return []sourceCard{
		displayCard, windowCard,
		{label: "Area", icon: iconArea, soon: true, blocked: "Recording part of the screen is not built yet"},
		{label: "Camera", icon: iconCamera, soon: true, blocked: "Recording a camera is not built yet"},
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// detail reports whether one media's page is shown instead of the grid.
func (a *App) detail() bool { return a.sel != 0 }

// iconButton is a button showing only an icon, named by label for assistive
// technology and by its tooltip for the pointer.
func (a *App) iconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) {
	kit.IconButton(c, t, icon, a.tr(label), onClick)
}

// loadAudioDevices selects the default speaker and keeps the microphone off.
// shortcut: endpoints load at startup; add WASAPI notifications for hot-plug refresh.
func (a *App) loadAudioDevices() {
	go func() {
		speakers, microphones, err := a.Rec.AudioDevices()
		a.update(func() {
			a.speakers, a.microphones = speakers, microphones
			if len(speakers) > 0 {
				a.speaker = speakers[0]
			}
			if err != nil {
				a.setError(err)
			}
		})
	}()
}

// sourceSelect chooses a display in full-screen mode or a window in window
// mode. The two modes share one compact dropdown.
func (a *App) sourceSelect(c *ui.Context, t *ui.Theme) {
	if a.sourceMode() == "window" {
		windows := a.sourcesOfKind("window")
		if len(windows) == 0 {
			a.sourceSelectEmpty(c, t, iconWindow, "No visible window to record")
			return
		}
		chosen := a.windowSource()
		options := make([]kit.SelectOption[capture.Source], 0, len(windows))
		for _, window := range windows {
			options = append(options, kit.SelectOption[capture.Source]{Value: window, Label: window.Name})
		}
		kit.Select(c, t, &chosen, options, a.tr("Window"), a.tr("Choose a window"), iconWindow, func(source capture.Source) {
			a.chooseSource(source.ID)
		})
		return
	}
	displays := a.sourcesOfKind("display")
	if len(displays) == 0 {
		displays = a.sourcesOfKind("screen")
	}
	if len(displays) == 0 {
		a.sourceSelectEmpty(c, t, iconScreen, "No display to record")
		return
	}
	chosen := a.displaySource()
	options := make([]kit.SelectOption[capture.Source], 0, len(displays))
	for _, display := range displays {
		options = append(options, kit.SelectOption[capture.Source]{Value: display, Label: display.Name})
	}
	kit.Select(c, t, &chosen, options, a.tr("Display"), a.tr("Choose a display"), iconScreen, func(source capture.Source) {
		a.chooseSource(source.ID)
	})
}

func (a *App) sourceMode() string {
	for _, source := range a.sources {
		if source.ID == a.sourceID && source.Kind == "window" {
			return "window"
		}
	}
	return "screen"
}

// displaySource is the chosen display, or the first available one.
func (a *App) displaySource() capture.Source {
	for _, source := range a.sources {
		if source.ID == a.sourceID && (source.Kind == "display" || source.Kind == "screen") {
			return source
		}
	}
	if displays := a.sourcesOfKind("display"); len(displays) > 0 {
		return displays[0]
	}
	if screens := a.sourcesOfKind("screen"); len(screens) > 0 {
		return screens[0]
	}
	return capture.Source{}
}

// windowSource is the chosen window, or the first available one.
func (a *App) windowSource() capture.Source {
	for _, source := range a.sources {
		if source.ID == a.sourceID && source.Kind == "window" {
			return source
		}
	}
	if windows := a.sourcesOfKind("window"); len(windows) > 0 {
		return windows[0]
	}
	return capture.Source{}
}

func (a *App) mediaMenu(card ui.Element, m library.Media) {
	card.ContextMenu(a.mediaMenuItems(m))
}

// mediaMenuItems are what a library item offers, from its context menu and
// from its ⋯ button.
func (a *App) mediaMenuItems(m library.Media) func(mn *ui.Menu) {
	return func(mn *ui.Menu) {
		if mn.Item(a.tr("Transcript")).Chosen() {
			a.openDetail(m.ID)
			a.tab = 0
		}
		if mn.Item(a.tr("Translate")).Chosen() {
			a.openDetail(m.ID)
			a.tab = 0
			if current, ok := a.current(); ok {
				a.setTranslationVisible(current, true)
			}
		}
		if mn.Item(a.tr("Summary")).Chosen() {
			a.openDetail(m.ID)
			a.tab = 1
		}
		mn.Separator()
		if mn.Item(a.tr("Show the file")).Chosen() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		if mn.Item(a.tr("Delete…")).Chosen() {
			a.askDelete(m.ID)
		}
	}
}

// cardSubtitle is the line under a card's name: what it is and when it was
// made.
func (a *App) cardSubtitle(m library.Media) string {
	switch m.Source {
	case "screenshot":
		return a.trf("Screenshot · %s", a.dateTime(m.CreatedAt))
	case "import":
		return a.trf("Imported · %s", a.dateTime(m.CreatedAt))
	}
	return a.trf("Recorded · %s", a.dateTime(m.CreatedAt))
}

// sourcesOfKind returns the sources of one kind, in the order the platform
// listed them.
func (a *App) sourcesOfKind(kind string) []capture.Source {
	var list []capture.Source
	for _, s := range a.sources {
		if s.Kind == kind {
			list = append(list, s)
		}
	}
	return list
}

// sourceName is the label of the source chosen, or the kind when it is gone.
func (a *App) sourceName() string {
	for _, s := range a.sources {
		if s.ID == a.sourceID {
			return s.Name
		}
	}
	return a.tr("Window")
}

// sourceDetail is the size the platform reports for the source chosen.
func (a *App) sourceDetail() string {
	for _, s := range a.sources {
		if s.ID == a.sourceID {
			return s.Detail
		}
	}
	return ""
}

// chooseSource makes one source the one recorded, and shows it.
func (a *App) chooseSource(id string) {
	a.sourceID = id
	a.source = a.sourceName()
	a.showPreview()
}

func (a *App) hasSource(id string) bool {
	for _, s := range a.sources {
		if s.ID == id {
			return true
		}
	}
	return false
}

// pickSource chooses a display when one is available, else the first source.
func (a *App) pickSource() {
	if len(a.sources) == 0 {
		a.sourceID = ""
		return
	}
	if display := a.displaySource(); display.ID != "" {
		a.chooseSource(display.ID)
		return
	}
	a.chooseSource(a.sources[0].ID)
}

func (a *App) startRecording() {
	if a.busy || a.recording {
		return
	}
	a.closePlayback()
	a.busy = true
	a.closeDetail() // the live recording shows in this window
	if a.Preview != nil {
		a.Preview.Stop() // the recording draws its own preview
	}
	cfg := capture.Config{
		SourceID: a.sourceID, SystemAudio: a.speaker.ID != "", Mic: a.microphone.ID != "",
		SpeakerID: a.speaker.ID, MicrophoneID: a.microphone.ID,
	}
	go func() {
		err := a.Rec.Start(cfg)
		a.update(func() {
			a.busy = false
			if err != nil {
				a.setError(err)
				a.showPreview()
				return
			}
			st := a.Rec.State()
			a.recording, a.startedAt = st.Recording, st.StartedAt
			a.late, a.preview = nil, nil
			a.err = ""
		})
	}()
}

// stopRecording ends the recording; discard deletes the file it made.
func (a *App) stopRecording(discard bool) {
	if a.busy {
		return
	}
	a.busy = true
	go func() {
		m, err := a.Rec.Stop()
		a.update(func() {
			a.busy = false
			lines := a.late
			a.recording, a.late, a.preview = false, nil, nil
			if err != nil {
				a.setError(err)
				a.reload()
				return
			}
			if discard {
				a.removeMedia(m.ID, true)
				return
			}
			// What the live pass heard is the transcript for now; the full pass
			// (speakers, mistakes fixed) is one click away on its transcript tab.
			a.provisional, a.provisionalID = lines, m.ID
			a.reload()
			a.selectMedia(m.ID)
			a.tab = 0
			a.reload()
			a.loadDetails() // the recording took room on the disk
			a.showPreview() // back to showing the source, not the recording
		})
	}()
}

func (a *App) importFiles() {
	chain, lang := a.importChain, a.trLang
	go func() {
		imported, err := a.Lib.Import(context.Background())
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.reload()
			if len(imported) > 0 {
				a.selectMedia(imported[0].ID)
			}
		})
		for _, m := range imported {
			if m.HasAudio {
				a.startImport(m.ID, chain, lang)
			}
		}
	}()
}

// startImport runs what importing does once the file is in the library: the
// offline transcription recording does too, and when the import asked for it,
// translation into lang and a summary — all as one job, so one Cancel stops
// the chain.
func (a *App) startImport(id int64, chain bool, lang string) {
	if a.Jobs == nil || a.ASR == nil {
		return
	}
	_, err := a.Jobs.Run(id, jobs.KindImport, func(ctx context.Context, progress func(float64)) error {
		share := 1.0
		if chain {
			share = 0.6
		}
		if err := a.ASR.TranscribeNow(ctx, id, func(p float64) { progress(p * share) }); err != nil {
			return err
		}
		if !chain {
			return nil
		}
		progress(0.6)
		// Translating into the language it is already in would spend the
		// provider's quota saying nothing.
		translated := asr.Transcript{Language: lang}
		if tr, err := a.ASR.Get(id); err == nil {
			translated = tr
		}
		if translated.Language != lang {
			if _, err := a.Trans.Translate(ctx, id, lang); err != nil {
				return err
			}
		}
		progress(0.8)
		_, err := a.Sum.Generate(ctx, id)
		return err
	})
	if err != nil {
		a.setError(err)
	}
}

// removeMedia deletes a record, and its file when withFile is set.
func (a *App) removeMedia(id int64, withFile bool) {
	if a.sel == id {
		a.closePlayback()
	}
	go func() {
		err := a.Lib.Remove(id, withFile)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			if a.sel == id {
				a.forget()
			}
			delete(a.thumbs, id)
			a.reload()
		})
	}()
}

// grabPreview decodes the latest frame of what is being recorded, or of what
// will be, into the bitmap — at most five times a second: the frame is a JPEG.
func (a *App) grabPreview() {
	if time.Since(a.previewAt) < 200*time.Millisecond {
		return
	}
	var frame []byte
	if a.recording {
		frame = a.Rec.Preview()
	} else if a.Preview != nil {
		frame = a.Preview.Frame()
	}
	if len(frame) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		return
	}
	a.previewAt = time.Now()
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if a.preview == nil || w != a.previewW || h != a.previewH {
		// A new source, or a new recording: the size may have changed.
		a.preview = ui.NewBitmap(img)
		a.previewW, a.previewH = w, h
		return
	}
	a.preview.Update(img)
}

// addOrphans puts the files Ottr made back in the library.
func (a *App) addOrphans() {
	paths := a.orphans
	go func() {
		_, err := a.Lib.AddFiles(paths)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.reload()
		})
	}()
}

// deleteOrphans deletes the files Ottr made and nothing else; the records are
// already gone.
func (a *App) deleteOrphans() {
	paths := a.orphans
	go func() {
		err := a.Lib.DeleteFiles(paths)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.reload()
		})
	}()
}
