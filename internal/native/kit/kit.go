// Package kit contains Ottr's reusable native UI components, drawn in the
// "ink and marks" system (palette.go, docs/design/tokens.css).
package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

var (
	selectCheckIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`))
	chevronIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`))
)

// ------------------------------------------------------------------ text

// Mono sets text in Geist Mono with tabular figures: timecodes, durations,
// sizes, paths and keys.
func Mono(e ui.Element) ui.Element {
	return e.Font(FontMono).FontFeatures("tnum")
}

// Label is a section label: 10.5px mono capitals, spaced, in the meta colour.
func Label(c *ui.Context, t *ui.Theme, text string) ui.Element {
	return Mono(ui.Text(c, upper(text))).FontSize(10.5).FontWeight(500).LetterSpacing(0.6).
		TextColor(P(t).Ink3).SingleLine()
}

// upper capitalises ASCII letters only, so that 中文 labels stay as written.
func upper(s string) string {
	b := []rune(s)
	for i, r := range b {
		if r >= 'a' && r <= 'z' {
			b[i] = r - 32
		}
	}
	return string(b)
}

// Kbd is one key of a shortcut.
func Kbd(c *ui.Context, t *ui.Theme, key string) {
	p := P(t)
	ui.Row(c).Height(18).MinWidth(18).Padding(0, 5).Radius(4).Center().
		BorderWidth(1, 1, 2, 1).BorderColor(p.Line2).Background(p.Sheet).Children(func() {
		Mono(ui.Text(c, key)).FontSize(10.5).TextColor(p.Ink2).SingleLine()
	})
}

// --------------------------------------------------------------- buttons

// ButtonKind is the look of a Button.
type ButtonKind int

const (
	// Secondary is a sheet button with a control border.
	Secondary ButtonKind = iota
	// Primary is the ink-filled button; one per view.
	Primary
	// Danger is the red-filled button: Stop, Delete.
	Danger
	// Ghost has no fill or border until hovered.
	Ghost
)

// Button is a 32px button: an optional leading icon and a label. The caller
// reads Clicked on the element it returns; lg makes it 40px.
func Button(c *ui.Context, t *ui.Theme, kind ButtonKind, icon *ui.SVG, label string, lg bool) ui.Element {
	p := P(t)
	height := float32(ControlHeight)
	if lg {
		height = 40
	}
	btn := ui.ButtonBase(c).Label(label).Height(height).Padding(0, 12).Radius(RadiusControl).
		Shrink(0).Center().Cursor(ui.CursorPointer)
	fill, ink, border := p.Sheet, p.Ink, p.Line2
	switch kind {
	case Primary:
		fill, ink, border = p.Btn, p.BtnInk, p.Btn
	case Danger:
		fill, ink, border = p.Red, ui.RGB(255, 255, 255), p.Red
	case Ghost:
		fill, ink, border = ui.Transparent, p.Ink2, ui.Transparent
	}
	if btn.Hovered() && !btn.IsDisabled() {
		switch kind {
		case Primary, Danger:
			fill = fill.Mix(ui.RGB(128, 128, 128), 0.14)
			border = fill
		default:
			fill = p.Hover
		}
	}
	btn.Background(fill).Border(1, border).TextColor(ink)
	if btn.IsDisabled() {
		btn.Opacity(0.5).Cursor(ui.CursorDefault)
	}
	btn.Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).FontSize(16).Shrink(0)
			}
			if label != "" {
				size := float32(13)
				if lg {
					size = 13.5
				}
				ui.Text(c, label).FontSize(size).FontWeight(500).SingleLine()
			}
		})
	})
	return btn
}

// IconButton is a 32px icon-only button with an accessible name and tooltip.
func IconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) ui.Element {
	return iconButton(c, t, icon, label, 32, false, false, onClick)
}

// BorderedIconButton is IconButton with the control border, beside a field.
func BorderedIconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) ui.Element {
	return iconButton(c, t, icon, label, 32, true, false, onClick)
}

// RailButton is a 36px icon button of the rail; on marks the page shown.
func RailButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, on bool, onClick func()) ui.Element {
	return iconButton(c, t, icon, label, 36, false, on, onClick)
}

func iconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, size float32, bordered, on bool, onClick func()) ui.Element {
	p := P(t)
	btn := ui.ButtonBase(c).Label(label).Tooltip(label).Size(size, size).Shrink(0).
		Radius(RadiusControl).Center().TextColor(p.Ink2).Cursor(ui.CursorPointer)
	if bordered {
		btn.Border(1, p.Line2).Background(p.Sheet)
	}
	if on {
		btn.Background(p.Hover).TextColor(p.Ink)
	} else if btn.Hovered() && !btn.IsDisabled() {
		btn.Background(p.Hover).TextColor(p.Ink)
	}
	if btn.IsDisabled() {
		btn.Opacity(0.5)
	}
	iconSize := float32(16)
	if size >= 36 {
		iconSize = 18
	}
	btn.Children(func() { ui.Icon(c, icon).FontSize(iconSize) })
	if btn.Clicked() && onClick != nil {
		onClick()
	}
	return btn
}

// DangerButton is the filled red action: Stop while recording.
func DangerButton(c *ui.Context, t *ui.Theme, label string) ui.Element {
	return Button(c, t, Danger, nil, label, false)
}

// RoundButton is kept for the narrow layouts: a 40px bordered square with an
// icon, beside a large primary button.
func RoundButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) {
	p := P(t)
	btn := ui.ButtonBase(c).Label(label).Tooltip(label).Size(44, 44).Shrink(0).Radius(RadiusControl).
		Border(1, p.Line2).Background(p.Sheet).Center().TextColor(p.Ink2).Cursor(ui.CursorPointer)
	if btn.Hovered() {
		btn.Background(p.Hover).TextColor(p.Ink)
	}
	btn.Children(func() { ui.Icon(c, icon).FontSize(18) })
	if btn.Clicked() {
		onClick()
	}
}

// ------------------------------------------------------------------ chips

// ChipKind is the meaning of a Chip.
type ChipKind int

const (
	ChipNeutral ChipKind = iota
	ChipOK               // Transcribed, In use, Works
	ChipAccent           // 中文, Translated, the accent
	ChipBad              // Failed, Can't be used
	ChipLine             // an outlined filter
	ChipRec              // recording in progress
)

// Chip is a 20px rounded status label.
func Chip(c *ui.Context, t *ui.Theme, kind ChipKind, text string) ui.Element {
	p := P(t)
	fill, ink := p.Hover, p.Ink2
	switch kind {
	case ChipOK:
		fill, ink = p.GreenSoft, p.Green
	case ChipAccent:
		fill, ink = p.AccentSoft, p.AccentInk
	case ChipBad, ChipRec:
		fill, ink = p.RedSoft, p.RedInk
	case ChipLine:
		fill = ui.Transparent
	}
	chip := ui.Row(c).Height(20).Padding(0, 7).Radius(9999).Gap(4).AlignItems(ui.Center).
		Shrink(0).Background(fill)
	if kind == ChipLine {
		chip.Border(1, p.Line2)
	}
	chip.Children(func() {
		if kind == ChipRec {
			ui.Box(c).Size(6, 6).Radius(3).Background(p.Red)
		}
		ui.Text(c, text).FontSize(11).FontWeight(500).TextColor(ink).SingleLine()
	})
	return chip
}

// FilterChip is one of the library's filters: a 28px pill, ink-filled when on.
func FilterChip(c *ui.Context, t *ui.Theme, text string, on bool) ui.Element {
	p := P(t)
	btn := ui.ButtonBase(c).Label(text).Height(26).Padding(0, 10).Radius(9999).Shrink(0).
		Center().Cursor(ui.CursorPointer)
	switch {
	case on:
		btn.Background(p.Btn).Border(1, p.Btn).TextColor(p.BtnInk)
	case btn.Hovered():
		btn.Background(p.Hover).Border(1, p.Line2).TextColor(p.Ink)
	default:
		btn.Background(p.Sheet).Border(1, p.Line2).TextColor(p.Ink2)
	}
	btn.Children(func() { ui.Text(c, text).FontSize(12).FontWeight(500).SingleLine() })
	return btn
}

// ----------------------------------------------------------------- switch

// Switch is the 32×18 switch: line2 when off, SwitchOn (ink, or the mark
// colour in the dark look) when on, a white knob.
func Switch(c *ui.Context, t *ui.Theme, on *bool, label string) ui.Element {
	p := P(t)
	sw := ui.SwitchBase(c, on).Label(label).Size(32, 18).Radius(9).Cursor(ui.CursorPointer)
	pos := sw.Animate("knob", b2f(*on), 140*time.Millisecond)
	sw.Background(p.Line2.Mix(p.SwitchOn, pos))
	knob := ui.RGB(255, 255, 255)
	if t.Dark && *on {
		knob = ui.Hex("#121417")
	}
	sw.Draw(func(pt *ui.Painter, r ui.Rect) {
		k := ui.Rect{X: r.X + 2 + pos*(r.W-r.H), Y: r.Y + 2, W: r.H - 4, H: r.H - 4}
		pt.Shadow(k, k.W/2, 0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.2))
		pt.Fill(k, knob, k.W/2)
	})
	if sw.IsDisabled() {
		sw.Opacity(0.5)
	}
	return sw
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// ----------------------------------------------------------------- groups

// Group is a bordered sheet that holds rows: settings groups, audio rows.
func Group(c *ui.Context, t *ui.Theme, children func()) ui.Element {
	p := P(t)
	return ui.Column(c).FillWidth().Radius(RadiusPanel).Border(1, p.Line).Background(p.Sheet).
		Clip().Children(children)
}

// SettingRow places a setting's explanation beside its control, as one row of
// a Group; first leaves out the divider above it.
func SettingRow(c *ui.Context, t *ui.Theme, title, detail string, control func()) {
	GroupRow(c, t, true, title, detail, control)
}

// GroupRow is SettingRow inside a Group: rows after the first are divided by
// a hairline.
func GroupRow(c *ui.Context, t *ui.Theme, first bool, title, detail string, control func()) {
	p := P(t)
	row := ui.Row(c).FillWidth().MinHeight(58).Gap(12).Padding(10, 16).AlignItems(ui.Center)
	if !first {
		row.BorderWidth(1, 0, 0, 0).BorderColor(p.Line)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(p.Ink)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(p.Ink3)
			}
		})
		if control != nil {
			control()
		}
	})
}

// SidePanel is a right-hand panel that enters and leaves horizontally.
func SidePanel(c *ui.Context, t *ui.Theme, width float32, children func()) {
	p := P(t)
	ui.Column(c).Width(width).Shrink(0).FillHeight().MinHeight(0).
		Background(p.Sheet).BorderWidth(0, 0, 0, 1).BorderColor(p.Line).
		Transition(ui.ElementTransition{
			Duration: 200 * time.Millisecond, Ease: ui.EaseOut,
			Enter: &ui.Motion{X: width, Opacity: 1},
			Exit:  &ui.Motion{X: width, Opacity: 1},
		}).Children(children)
}

// ---------------------------------------------------------------- selects

// SelectOption is one value in a Select.
type SelectOption[T comparable] struct {
	Value T
	Label string
}

func nameOf[T comparable](selected T, options []SelectOption[T]) string {
	for _, option := range options {
		if option.Value == selected {
			return option.Label
		}
	}
	return ""
}

// Select is a full-width 32px select with a leading icon: the display, the
// window. Its options open in a panel joined to the header, and the arrows
// choose while it is open. accessibility names it to assistive technology;
// label is its tooltip.
func Select[T comparable](c *ui.Context, t *ui.Theme, selected *T, options []SelectOption[T], label, accessibility string, icon *ui.SVG, onChoose func(T)) {
	p := P(t)
	root := ui.Column(c).FillWidth().MinWidth(0).Radius(RadiusControl).Border(1, p.Line2).
		Background(p.Sheet).Clip()
	open := ui.Local(root, "select-open", func() bool { return false })
	if *open {
		root.Radius(RadiusControl, RadiusControl, 0, 0)
	}
	root.Children(func() {
		name := nameOf(*selected, options)
		header := ui.ButtonBase(c).Label(accessibility).Tooltip(label).Expanded(*open).
			FillWidth().MinWidth(0).Height(ControlHeight-2).Padding(0, 10).AlignItems(ui.Center).
			Cursor(ui.CursorPointer)
		if header.Hovered() {
			header.Background(p.Hover)
		}
		header.Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(16).Shrink(0).TextColor(p.Ink2)
				}
				ui.Text(c, name).FontSize(13).TextColor(p.Ink).SingleLine().Grow(1).MinWidth(0).Tooltip(name)
				ui.Icon(c, chevronIcon).FontSize(16).Shrink(0).TextColor(p.Ink3)
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
			panel.Width(root.Bounds().W).Padding(4).Radius(0, 0, RadiusControl, RadiusControl).
				Background(p.Sheet).BorderWidth(0, 1, 1, 1).BorderColor(p.Line2).Clip()
			DialogShadow(panel, t)
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
							Checked(option.Value == *selected).FillWidth().Height(32).Padding(0, 8).
							Radius(RadiusControl).AlignItems(ui.Center).Gap(8)
						items[i] = item
						if item.Hovered() {
							item.Background(p.Hover)
						} else if option.Value == *selected {
							item.Background(p.Sunk)
						}
						item.Children(func() {
							ui.Text(c, option.Label).FontSize(13).TextColor(p.Ink).SingleLine().
								Grow(1).MinWidth(0).Tooltip(option.Label)
							if option.Value == *selected {
								ui.Icon(c, selectCheckIcon).FontSize(14).Shrink(0).TextColor(p.Ink2)
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

// SelectBox is the bordered select Settings uses: the value on the left, a
// chevron on the right. A width of 0 fills the row.
func SelectBox[T comparable](c *ui.Context, t *ui.Theme, selected *T, options []SelectOption[T], accessibility string, width float32, onChoose func(T)) {
	selectBox(c, t, selected, options, accessibility, width, ControlHeight, nil, false, onChoose)
}

// CompactSelect is the select of a toolbar, optionally with a leading icon.
func CompactSelect[T comparable](c *ui.Context, t *ui.Theme, selected *T, options []SelectOption[T], accessibility string, width float32, icon *ui.SVG, onChoose func(T)) {
	selectBox(c, t, selected, options, accessibility, width, 30, icon, false, onChoose)
}

// TextSelect is a select drawn as its value alone, in the meta colour with a
// small chevron: the device under an audio row's name.
func TextSelect[T comparable](c *ui.Context, t *ui.Theme, selected *T, options []SelectOption[T], accessibility string, onChoose func(T)) {
	selectBox(c, t, selected, options, accessibility, 0, 18, nil, true, onChoose)
}

// selectBox is the shared body of the selects. The popup is opaque and
// carries the one shadow of the system.
func selectBox[T comparable](c *ui.Context, t *ui.Theme, selected *T, options []SelectOption[T], accessibility string, width, height float32, icon *ui.SVG, bare bool, onChoose func(T)) {
	p := P(t)
	sel := ui.SelectBase(c, selected)
	trigger := sel.Trigger.Label(accessibility).Tooltip(accessibility).Height(height).MinWidth(0).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if width > 0 {
		trigger.Width(width).Shrink(0)
	} else {
		trigger.FillWidth()
	}
	if bare {
		trigger.TextColor(p.Ink3)
		if sel.Trigger.Hovered() {
			trigger.TextColor(p.Ink)
		}
	} else {
		trigger.Padding(0, 10).Radius(RadiusControl).Border(1, p.Line2).Background(p.Sheet).TextColor(p.Ink)
		if sel.Trigger.Hovered() {
			trigger.Background(p.Hover)
		}
	}
	sel.Trigger.Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).FontSize(16).Shrink(0).TextColor(p.Ink2)
			}
			name := nameOf(*selected, options)
			text := ui.Text(c, name).SingleLine().MinWidth(0).Tooltip(name)
			if bare {
				text.FontSize(11.5)
			} else {
				text.FontSize(13).Grow(1)
			}
			chevron := ui.Icon(c, chevronIcon).Shrink(0).TextColor(p.Ink3)
			if bare {
				chevron.FontSize(12)
			} else {
				chevron.FontSize(16)
			}
		})
	})
	sel.Popup(func(panel ui.Element) {
		w := max(width, 200)
		if width == 0 {
			w = max(sel.Trigger.Bounds().W, 200)
		}
		panel.Width(w).Padding(4).Radius(RadiusCard).Background(p.Sheet).Border(1, p.Line).Clip()
		DialogShadow(panel, t)
		ui.Scroll(c).MaxHeight(320).Children(func() {
			ui.Column(c).Children(func() {
				for _, option := range options {
					item := sel.Item(option.Value).Height(32).Padding(0, 8).Radius(RadiusControl).
						AlignItems(ui.Center).Gap(8)
					switch {
					case item.Highlighted():
						item.Background(p.Hover)
					case option.Value == *selected:
						item.Background(p.Sunk)
					}
					item.Children(func() {
						ui.Text(c, option.Label).FontSize(13).TextColor(p.Ink).SingleLine().Grow(1).MinWidth(0)
						if option.Value == *selected {
							ui.Icon(c, selectCheckIcon).FontSize(14).Shrink(0).TextColor(p.Ink2)
						}
					})
					if item.Clicked() {
						onChoose(option.Value)
					}
				}
			})
		})
	})
}

// ---------------------------------------------------------------- avatars

// Avatar is a speaker's round tile: a colour by number and white initials.
func Avatar(c *ui.Context, number int, initials string, size float32) {
	fill := SpeakerColors[max(number-1, 0)%len(SpeakerColors)]
	ui.Column(c).Size(size, size).Shrink(0).Radius(size / 2).Center().Background(fill).Children(func() {
		ui.Text(c, initials).FontSize(max(9, float32(int(size*0.4)))).FontWeight(600).
			TextColor(ui.RGB(255, 255, 255)).SingleLine()
	})
}

// UnknownAvatar is the grey "?" tile of a line no speaker owns yet.
func UnknownAvatar(c *ui.Context, t *ui.Theme, size float32) {
	p := P(t)
	ui.Column(c).Size(size, size).Shrink(0).Radius(size / 2).Center().Background(p.Hover).Children(func() {
		ui.Text(c, "?").FontSize(11).FontWeight(600).TextColor(p.Ink3)
	})
}

// ----------------------------------------------------------------- fields

// Field is the bordered text input of the system (.field in tokens.css): a
// 32px sheet with a control border that turns teal while it has focus, an
// optional leading icon, and the bare input growing inside it. The input is
// returned so the caller can add a placeholder, Password or a font.
func Field(c *ui.Context, t *ui.Theme, value *string, label string, icon *ui.SVG) ui.Element {
	p := P(t)
	var input ui.Element
	box := ui.Row(c).Height(ControlHeight).Grow(1).MinWidth(0).Padding(0, 10).Gap(8).
		AlignItems(ui.Center).Radius(RadiusControl).Background(p.Sheet)
	border := p.Line2
	if box.FocusWithin() {
		border = p.Accent
	}
	box.Border(1, border).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(14).TextColor(p.Ink3)
		}
		input = ui.TextInputBase(c, value).Label(label).PlaceholderColor(p.Ink3).
			TextColor(p.Ink).FontSize(13).Grow(1).MinWidth(0)
	})
	return input
}
