package native

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"ottr/internal/capture"
)

// sourcesPage chooses one source of a kind: the windows, which are too many
// for the sidebar to show, after a search.
func (a *App) sourcesPage(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Gap(t.Space(3)).Padding(t.Space(3.5), t.Space(5)).AlignItems(ui.Center).
			Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			a.backButton(c, t)
			ui.SearchField(c, &a.pickQuery).Label("Search" + " " + a.pickKind + "s").Grow(1)
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(t.Space(4), t.Space(5)).Gap(t.Space(1)).Children(func() {
				a.caption(c, t, "Choose a "+a.pickKind)
				shown := a.pickList()
				if len(shown) == 0 {
					ui.Text(c, "Nothing to record was found.").TextColor(t.TextMuted).
						Padding(t.Space(2), t.Space(1))
					return
				}
				for _, s := range shown {
					a.sourceRow(c, t, s)
				}
			})
		})
	})
}

// pickList is the sources of the kind being picked, as far as the search
// narrows them.
func (a *App) pickList() []capture.Source {
	all := a.sourcesOfKind(a.pickKind)
	query := strings.ToLower(strings.TrimSpace(a.pickQuery))
	if query == "" {
		return all
	}
	var shown []capture.Source
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Name), query) {
			shown = append(shown, s)
		}
	}
	return shown
}

// sourceRow is one thing that can be recorded: what it is, how big it is, and
// a mark on the one already chosen.
func (a *App) sourceRow(c *ui.Context, t *ui.Theme, s capture.Source) {
	row := ui.ButtonBase(c).Label(s.Name).Gap(t.Space(3)).
		Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	row.Children(func() {
		icon := iconWindow
		if s.Kind == "display" {
			icon = iconScreen
		}
		ui.Icon(c, icon).FontSize(t.FontSize + 5).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, s.Name).Bold().SingleLine()
			detail := s.Detail
			if detail == "" {
				detail = "Window"
				if s.Kind == "display" {
					detail = "Display"
				}
			}
			ui.Text(c, detail).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		})
		if s.ID == a.sourceID {
			ui.Icon(c, iconCheck).FontSize(t.FontSize + 2).TextColor(t.Accent).Label("Chosen")
		}
	})
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() {
		a.chooseSource(s.ID)
		a.closePane()
	}
}

// backButton leaves a page for the one it opened from, which for the picker is
// the recording form alone.
func (a *App) backButton(c *ui.Context, t *ui.Theme) {
	btn := ui.ButtonBase(c).Label("Back").Gap(t.Space(1)).Padding(t.Space(1.5), t.Space(2)).
		Radius(t.Radius).TextColor(t.TextMuted)
	btn.Children(func() {
		ui.Icon(c, iconBack).FontSize(t.FontSize + 2)
		ui.Text(c, "Back").FontSize(t.FontSize - 1)
	})
	if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	if btn.Clicked() {
		a.closePane()
	}
}
