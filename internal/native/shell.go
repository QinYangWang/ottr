package native

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"ottr/internal/library"
	"ottr/internal/native/kit"
)

// The library's views: what the sidebar and the filter chips choose.
const (
	filterAll        = ""
	filterRecordings = "recording"
	filterImports    = "import"
	filterSummary    = "summary"
	filterAttention  = "attention"
)

// matchesFilter reports whether a media belongs in a library view.
func matchesFilter(m library.Media, filter string) bool {
	switch filter {
	case filterRecordings, filterImports:
		return m.Source == filter
	case filterSummary:
		return m.SummaryStatus == "done"
	case filterAttention:
		return m.TranscriptStatus == "error"
	}
	return true
}

// visibleMedia is the library as the view chosen shows it.
func (a *App) visibleMedia() []library.Media {
	if a.filter == filterAll {
		return a.media
	}
	var out []library.Media
	for _, m := range a.media {
		if matchesFilter(m, a.filter) {
			out = append(out, m)
		}
	}
	return out
}

func (a *App) countOf(filter string) int {
	n := 0
	for _, m := range a.media {
		if matchesFilter(m, filter) {
			n++
		}
	}
	return n
}

// navSidebar is the 232px sidebar of the wide window: the library, the views
// that need a look, the Settings sections, and the disk card at the bottom.
func (a *App) navSidebar(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Padding(12, 10).
		Background(p.Paper).BorderWidth(0, 1, 0, 0).BorderColor(p.Line).Children(func() {
		a.navLibrary(c, t)
		navLabel(c, t, a.tr("Settings"))
		if !a.loadedSpeakers {
			a.loadSpeakers()
		}
		a.navSettings(c, t, 0, "Appearance", iconPalette)
		a.navSettings(c, t, 1, "Transcription", iconCaptions)
		a.navSettings(c, t, 2, "Provider", iconProvider)
		a.navSettings(c, t, 3, "Speakers", iconSpeakers)
		ui.Spacer(c)
		a.statusCard(c, t)
	})
}

func navLabel(c *ui.Context, t *ui.Theme, text string) {
	ui.Row(c).Padding(14, 10, 6, 10).Children(func() { kit.Label(c, t, text) })
}

// navLibrary is the one way into the library: the chips beside it choose the
// view.
func (a *App) navLibrary(c *ui.Context, t *ui.Theme) {
	if a.navRow(c, t, iconLibrary, "Library", strconv.Itoa(a.countOf(filterAll)), !a.settingsOpen) {
		a.closeSettings()
		a.filter = filterAll
		a.libraryOpen = true
		a.closeDetail()
		a.reload()
	}
}

// navSettings is one Settings section in the sidebar.
func (a *App) navSettings(c *ui.Context, t *ui.Theme, tab int, label string, icon *ui.SVG) {
	count := ""
	if tab == 3 {
		count = strconv.Itoa(len(a.speakerLib))
	}
	if a.navRow(c, t, icon, label, count, a.settingsOpen && a.settingsTab == tab) {
		a.settingsTab = tab
		a.openSettings()
		a.closeDetail()
	}
}

func (a *App) navRow(c *ui.Context, t *ui.Theme, icon *ui.SVG, label, count string, on bool) bool {
	p := pal(t)
	label = a.tr(label)
	row := ui.ButtonBase(c.Key(label)).Label(label).FillWidth().Height(32).Padding(0, 10).Gap(10).
		Radius(kit.RadiusControl).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	switch {
	case on && t.Dark:
		row.Background(p.Hover)
	case on:
		row.Background(p.Sheet).Border(1, p.Line)
	case row.Hovered():
		row.Background(p.Hover)
	}
	row.Children(func() {
		ink := p.Ink2
		if on {
			ink = p.Ink
		}
		ui.Icon(c, icon).FontSize(16).TextColor(ink).Shrink(0)
		text := ui.Text(c, label).FontSize(13).TextColor(ink).SingleLine().Grow(1).MinWidth(0)
		if on {
			text.FontWeight(500)
		}
		if count != "" {
			mono(ui.Text(c, count)).FontSize(11).TextColor(p.Ink3)
		}
	})
	return row.Clicked()
}

// statusCard is the room left on the volume recordings go to, seen before it
// matters: how much of the disk is used, and how much is free.

// meter is the 4px bar a share is drawn with, a download or the disk.
func meter(c *ui.Context, t *ui.Theme, value, width float32) {
	p := pal(t)
	bar := ui.Box(c).Height(4).Radius(2).Background(p.Hover)
	if width > 0 {
		bar.Width(width).Shrink(0)
	} else {
		bar.FillWidth()
	}
	bar.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * min(max(value, 0), 1), H: r.H}, p.Ink2, 2)
	})
}
func (a *App) statusCard(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if !a.details.loaded {
		a.loadDetails()
	}
	d := a.details
	ui.Column(c).FillWidth().Padding(12).Gap(4).Radius(kit.RadiusCard).Border(1, p.Line).
		Background(p.Sheet).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("Disk usage")).FontSize(11).TextColor(p.Ink3).Grow(1)
			text := "—"
			switch {
			case d.freeErr != "":
				text = a.tr("Unknown")
			case d.free > 0:
				text = a.trf("%s free", fmtSize(int64(d.free)))
			}
			label := mono(ui.Text(c, text)).FontSize(11).TextColor(p.Ink2)
			if d.freeErr != "" {
				label.Tooltip(d.freeErr)
			}
		})
		used := float32(0)
		if d.total > 0 && d.free < d.total {
			used = float32(d.total-d.free) / float32(d.total)
		}
		meter(c, t, used, 0)
	})
}

// rail is the 56px column of item pages and the 900px window: the record
// button, the library, its summaries, speakers, and Settings at the bottom.
func (a *App) rail(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	_, detail := a.current()
	ui.Column(c).Width(railWidth).Shrink(0).FillHeight().Padding(10, 0).Gap(4).AlignItems(ui.Center).
		Background(p.Paper).BorderWidth(0, 1, 0, 0).BorderColor(p.Line).Children(func() {
		if !detail {
			rec := ui.ButtonBase(c).Label(a.tr("New recording")).Tooltip(a.tr("New recording")).Size(36, 36).
				Radius(kit.RadiusControl).Background(p.Btn).Center().Cursor(ui.CursorPointer)
			rec.Children(func() { ui.Box(c).Size(10, 10).Radius(5).Background(p.Red) })
			if rec.Clicked() {
				a.closeSettings()
				a.closeDetail()
				a.libraryOpen = false
			}
			ui.Box(c).Height(8)
		}
		onLibrary := detail || a.libraryOpen
		kit.RailButton(c, t, iconLibrary, a.tr("Library"), !a.settingsOpen && onLibrary && a.filter != filterSummary, func() {
			a.closeSettings()
			a.filter = filterAll
			if detail {
				a.closeDetail()
				a.libraryOpen = true
				return
			}
			a.libraryOpen = !a.libraryOpen
		})
		kit.RailButton(c, t, iconSparkles, a.tr("Has summary"), !a.settingsOpen && onLibrary && a.filter == filterSummary, func() {
			a.closeSettings()
			a.closeDetail()
			a.filter, a.libraryOpen = filterSummary, true
		})
		kit.RailButton(c, t, iconSpeakers, a.tr("Speakers"), a.settingsOpen && a.settingsTab == 3, func() {
			a.settingsTab = 3
			a.openSettings()
		})
		ui.Spacer(c)
		kit.RailButton(c, t, iconSettings, a.tr("Settings"), a.settingsOpen, func() {
			a.openSettings()
		})
	})
}
