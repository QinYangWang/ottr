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
	filterShots      = "screenshot"
	filterSummary    = "summary"
	filterAttention  = "attention"
)

// matchesFilter reports whether a media belongs in a library view.
func matchesFilter(m library.Media, filter string) bool {
	switch filter {
	case filterRecordings, filterImports, filterShots:
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

// navSidebar is the 232px sidebar of the wide window: the library's views,
// the workspace, and the status card at the bottom.
func (a *App) navSidebar(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Padding(12, 10).
		Background(p.Paper).BorderWidth(0, 1, 0, 0).BorderColor(p.Line).Children(func() {
		ui.Row(c).Padding(4, 10, 6, 10).Children(func() { kit.Label(c, t, a.tr("Library")) })
		a.navItem(c, t, iconLibrary, "All media", filterAll)
		a.navItem(c, t, iconFilm, "Recordings", filterRecordings)
		a.navItem(c, t, iconImport, "Imports", filterImports)
		a.navItem(c, t, iconImage, "Screenshots", filterShots)
		navLabel(c, t, a.tr("Views"))
		a.navItem(c, t, iconSparkles, "Has summary", filterSummary)
		a.navItem(c, t, iconAlertTriangle, "Needs attention", filterAttention)
		navLabel(c, t, a.tr("Workspace"))
		if !a.loadedSpeakers {
			a.loadSpeakers()
		}
		a.navLink(c, t, iconSpeakers, "Speakers", strconv.Itoa(len(a.speakerLib)), func() {
			a.settingsTab = 3
			a.openSettings()
		})
		a.navLink(c, t, iconSettings, "Settings", "", func() {
			a.settingsTab = 0
			a.openSettings()
		})
		ui.Spacer(c)
		a.statusCard(c, t)
	})
}

func navLabel(c *ui.Context, t *ui.Theme, text string) {
	ui.Row(c).Padding(14, 10, 6, 10).Children(func() { kit.Label(c, t, text) })
}

// navItem is one library view: its icon, name and count, on the sheet with a
// hairline when chosen.
func (a *App) navItem(c *ui.Context, t *ui.Theme, icon *ui.SVG, label, filter string) {
	count := ""
	if filter == filterAttention {
		if a.countOf(filter) > 0 {
			count = "•"
		}
	} else {
		count = strconv.Itoa(a.countOf(filter))
	}
	if a.navRow(c, t, icon, label, count, a.filter == filter, filter == filterAttention) {
		a.filter = filter
		a.libraryOpen = true
		a.closeDetail()
	}
}

// navLink is a sidebar row that goes somewhere instead of filtering.
func (a *App) navLink(c *ui.Context, t *ui.Theme, icon *ui.SVG, label, count string, onClick func()) {
	if a.navRow(c, t, icon, label, count, false, false) {
		onClick()
	}
}

func (a *App) navRow(c *ui.Context, t *ui.Theme, icon *ui.SVG, label, count string, on, alert bool) bool {
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
		switch {
		case alert && count != "":
			ui.Box(c).Size(7, 7).Radius(4).Background(p.Red)
		case count != "":
			mono(ui.Text(c, count)).FontSize(11).TextColor(p.Ink3)
		}
	})
	return row.Clicked()
}

// statusCard is what a recording depends on, seen before it matters: the
// model that transcribes and the room left on the disk.
func (a *App) statusCard(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if !a.details.loaded {
		a.loadDetails()
	}
	d := a.details
	ui.Column(c).FillWidth().Padding(12).Gap(10).Radius(kit.RadiusCard).Border(1, p.Line).
		Background(p.Sheet).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, a.tr("Transcription")).FontSize(11).TextColor(p.Ink3)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				dot, text, tip := p.Green, d.model+" · "+a.tr("offline"), ""
				switch {
				case !d.loaded:
					dot, text = p.Ink4, "—"
				case d.model == "":
					dot, text = p.Red, a.tr("No model installed")
				case !d.ready:
					dot, text, tip = p.Amber, d.model+" · "+a.tr("not ready"), d.why
				}
				ui.Box(c).Size(7, 7).Radius(4).Background(dot).Shrink(0)
				label := ui.Text(c, text).FontSize(12).TextColor(p.Ink).SingleLine().Grow(1).MinWidth(0)
				if tip != "" {
					label.Tooltip(tip)
				}
				mono(ui.Text(c, "CPU")).FontSize(11).TextColor(p.Ink3)
			})
		})
		ui.Column(c).Gap(4).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("Disk")).FontSize(11).TextColor(p.Ink3).Grow(1)
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
			ui.Text(c, a.trf("Saves to %s", shortDir(a.recordingsDir()))).FontSize(11).TextColor(p.Ink3).
				SingleLine().Tooltip(a.recordingsDir())
		})
	})
}

// rail is the 56px column of item pages and the 900px window: the record
// button, the library, its summaries, speakers, and Settings at the bottom.
func (a *App) rail(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Column(c).Width(railWidth).Shrink(0).FillHeight().Padding(10, 0).Gap(4).AlignItems(ui.Center).
		Background(p.Paper).BorderWidth(0, 1, 0, 0).BorderColor(p.Line).Children(func() {
		rec := ui.ButtonBase(c).Label(a.tr("New recording")).Tooltip(a.tr("New recording")).Size(36, 36).
			Radius(kit.RadiusControl).Background(p.Btn).Center().Cursor(ui.CursorPointer)
		rec.Children(func() { ui.Box(c).Size(10, 10).Radius(5).Background(p.Red) })
		if rec.Clicked() {
			a.closeDetail()
			a.libraryOpen = false
		}
		ui.Box(c).Height(8)
		_, detail := a.current()
		onLibrary := detail || a.libraryOpen
		kit.RailButton(c, t, iconLibrary, a.tr("Library"), onLibrary && a.filter != filterSummary, func() {
			a.filter = filterAll
			if detail {
				a.closeDetail()
				a.libraryOpen = true
				return
			}
			a.libraryOpen = !a.libraryOpen
		})
		kit.RailButton(c, t, iconSparkles, a.tr("Has summary"), onLibrary && a.filter == filterSummary, func() {
			a.closeDetail()
			a.filter, a.libraryOpen = filterSummary, true
		})
		kit.RailButton(c, t, iconSpeakers, a.tr("Speakers"), false, func() {
			a.settingsTab = 3
			a.openSettings()
		})
		ui.Spacer(c)
		kit.RailButton(c, t, iconSettings, a.tr("Settings"), false, func() {
			a.settingsTab = 0
			a.openSettings()
		})
	})
}
