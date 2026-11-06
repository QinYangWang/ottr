// Package kit contains Ottr's reusable native UI components.
package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// IconButton is an icon-only button with an accessible name and tooltip.
func IconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) {
	btn := ui.ButtonBase(c).Label(label).Tooltip(label).TextColor(t.TextMuted).
		Padding(t.Space(1.5)).Radius(t.Radius)
	btn.Children(func() { ui.Icon(c, icon).FontSize(t.FontSize + 3) })
	if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	if btn.Clicked() {
		onClick()
	}
}

// SettingRow places a setting's explanation beside its control.
func SettingRow(c *ui.Context, t *ui.Theme, title, detail string, control func()) {
	ui.Row(c).Gap(t.Space(3)).Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius).
		BorderWidth(1).BorderColor(t.Border).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(t.FontSize - 1).Bold().SingleLine()
			if detail != "" {
				ui.Text(c, detail).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
			}
		})
		control()
	})
}

// SidePanel is a right-hand panel that enters and leaves horizontally.
func SidePanel(c *ui.Context, t *ui.Theme, width float32, children func()) {
	ui.Column(c).Width(width).Grow(1).FillHeight().MinHeight(0).
		Background(t.Background).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).
		Transition(ui.ElementTransition{
			Duration: 200 * time.Millisecond, Ease: ui.EaseOut,
			Enter: &ui.Motion{X: width, Opacity: 1},
			Exit:  &ui.Motion{X: width, Opacity: 1},
		}).Children(children)
}

var selectCheckIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`))

// SelectOption is one value in a Select.
type SelectOption[T comparable] struct {
	Value T
	Label string
}

// Select keeps its header in place and floats options over following content.
// The shared border and radius make header and options read as one container.
func Select[T comparable](c *ui.Context, t *ui.Theme, selected *T, options []SelectOption[T], label, accessibility string, icon *ui.SVG, onChoose func(T)) {
	root := ui.Column(c).FillWidth().MinWidth(0).Radius(t.Radius).Border(1, t.Border).
		Background(t.Surface)
	open := ui.Local(root, "select-open", func() bool { return false })
	if *open {
		root.Radius(t.Radius, t.Radius, 0, 0)
	}
	root.Children(func() {
		name := ""
		for _, option := range options {
			if option.Value == *selected {
				name = option.Label
				break
			}
		}
		header := ui.ButtonBase(c).Label(accessibility).Tooltip(label).Expanded(*open).
			FillWidth().MinWidth(0).Padding(t.Space(2), t.Space(2.5)).AlignItems(ui.Center)
		header.Children(func() {
			ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(t.FontSize + 2).Shrink(0).TextColor(t.TextMuted)
				}
				ui.Text(c, label).FontSize(t.FontSize - 1).Shrink(0)
				ui.Text(c, name).FontSize(t.FontSize - 1).TextColor(t.TextMuted).
					SingleLine().Grow(1).MinWidth(0).TextAlign(ui.End).Tooltip(name)
				chevron := "▾"
				if *open {
					chevron = "▴"
				}
				ui.Text(c, chevron).TextColor(t.TextMuted).Shrink(0)
			})
		})
		if header.Clicked() {
			*open = !*open
		}
		if header.Shortcut(0, ui.KeyDown) {
			*open = true
		}
		if !*open {
			return
		}
		ui.PopoverBase(c, root, open, func(panel ui.Element) {
			panel.Width(root.Bounds().W).Padding(t.Space(1)).Radius(0, 0, t.Radius, t.Radius).
				Background(t.Surface).BorderWidth(0, 1, 1, 1).BorderColor(t.Border).Clip()
			panel.Shadow(0, 8, 40, 0, ui.RGBA(0, 0, 0, 0.05)).Shadow(0, 12, 32, -16, ui.RGBA(0, 0, 0, 0.15))
			ui.Scroll(c).MaxHeight(280).Children(func() {
				ui.Column(c).Role(ui.RoleRadioGroup).Label(accessibility).Children(func() {
					items := make([]ui.Element, len(options))
					moveTo := -1
					choose := func(option SelectOption[T], collapse bool) {
						*selected = option.Value
						if collapse {
							*open = false
						}
						onChoose(option.Value)
					}
					for i, option := range options {
						item := ui.ButtonBase(c).Role(ui.RoleRadio).Label(option.Label).Tooltip(option.Label).
							Checked(option.Value == *selected).FillWidth().Height(40).Padding(0, t.Space(3)).
							Radius(9999).AlignItems(ui.Center)
						items[i] = item
						if option.Value == *selected {
							item.Background(t.Surface)
						} else if item.Hovered() {
							item.Background(t.SurfaceHover)
						}
						item.Children(func() {
							ui.Text(c, option.Label).FontSize(t.FontSize - 1).SingleLine().
								Grow(1).MinWidth(0).Tooltip(option.Label)
							if option.Value == *selected {
								ui.Icon(c, selectCheckIcon).FontSize(t.FontSize + 2).Shrink(0)
							}
						})
						if item.Clicked() {
							choose(option, true)
						}
						switch {
						case item.Shortcut(0, ui.KeyDown):
							moveTo = min(i+1, len(options)-1)
						case item.Shortcut(0, ui.KeyUp):
							moveTo = max(i-1, 0)
						case item.Shortcut(0, ui.KeyHome):
							moveTo = 0
						case item.Shortcut(0, ui.KeyEnd):
							moveTo = len(options) - 1
						}
					}
					if moveTo >= 0 {
						items[moveTo].Focus()
						choose(options[moveTo], false)
					}
				})
			})
		})
	})
}
