package kit

import "github.com/egoist/mygo/ui"

// Palette is the "ink and marks" system the Doop canvas is drawn in
// (docs/design/tokens.css): graphite ink on near-white paper, and three quiet
// marks — teal for what is current, olive for done, red for recording and
// destruction. ui.Theme carries the few colours MyGo's own widgets read;
// everything the app draws itself takes its colour from here.
type Palette struct {
	Paper, Sheet, Sunk, Hover                 ui.Color // chrome, content, inset wells, hover
	Ink, Ink2, Ink3, Ink4                     ui.Color // text: primary, secondary, meta, disabled
	Line, Line2                               ui.Color // dividers and card borders; control borders
	Accent, AccentSoft, AccentInk, AccentLine ui.Color
	Red, RedSoft, RedInk                      ui.Color
	Green, GreenSoft                          ui.Color
	Btn, BtnInk                               ui.Color // the primary button's fill and text
	Media                                     ui.Color // video stage and thumbnails
	SwitchOn                                  ui.Color
	Shadow, Shadow2                           ui.Color
}

var (
	light = Palette{
		Paper: ui.Hex("#F4F7F7"), Sheet: ui.Hex("#FCFDFD"), Sunk: ui.Hex("#F0F4F5"), Hover: ui.Hex("#E9EFF0"),
		Ink: ui.Hex("#1F2328"), Ink2: ui.Hex("#5D6064"), Ink3: ui.Hex("#8A9095"), Ink4: ui.Hex("#B9BEC1"),
		Line: ui.Hex("#E9EDED"), Line2: ui.Hex("#D4DADB"),
		Accent: ui.Hex("#297893"), AccentSoft: ui.Hex("#E3EFF3"), AccentInk: ui.Hex("#1B5B6F"), AccentLine: ui.Hex("#A8C9D5"),
		Red: ui.Hex("#D5283D"), RedSoft: ui.Hex("#FBEAEC"), RedInk: ui.Hex("#A31D2F"),
		Green: ui.Hex("#5F7208"), GreenSoft: ui.Hex("#EFF3DB"),
		Btn: ui.Hex("#1F2328"), BtnInk: ui.Hex("#FCFDFD"),
		Media:    ui.Hex("#171B1F"),
		SwitchOn: ui.Hex("#1F2328"),
		Shadow:   ui.RGBA(31, 35, 40, 0.14),
		Shadow2:  ui.RGBA(31, 35, 40, 0.06),
	}
	dark = Palette{
		Paper: ui.Hex("#0F1214"), Sheet: ui.Hex("#171B1E"), Sunk: ui.Hex("#1C2124"), Hover: ui.Hex("#242A2E"),
		Ink: ui.Hex("#E7EAEB"), Ink2: ui.Hex("#ACB3B6"), Ink3: ui.Hex("#8A9296"), Ink4: ui.Hex("#5C6367"),
		Line: ui.Hex("#293035"), Line2: ui.Hex("#394146"),
		Accent: ui.Hex("#4FA3BE"), AccentSoft: ui.Hex("#102F3A"), AccentInk: ui.Hex("#8AC4D6"), AccentLine: ui.Hex("#2B5D6E"),
		Red: ui.Hex("#E5485B"), RedSoft: ui.Hex("#3A161B"), RedInk: ui.Hex("#FF8896"),
		Green: ui.Hex("#B3C155"), GreenSoft: ui.Hex("#262C12"),
		Btn: ui.Hex("#E7EAEB"), BtnInk: ui.Hex("#141719"),
		Media:    ui.Hex("#080A0C"),
		SwitchOn: ui.Hex("#4FA3BE"),
		Shadow:   ui.RGBA(0, 0, 0, 0.5),
		Shadow2:  ui.RGBA(0, 0, 0, 0.3),
	}
)

// P is the palette of the look the theme is in.
func P(t *ui.Theme) Palette {
	if t.Dark {
		return dark
	}
	return light
}

// SpeakerColors fill speaker avatars, with white initials. They tell voices
// apart; they are not identities.
var SpeakerColors = []ui.Color{
	ui.Hex("#297893"), ui.Hex("#7B6BA6"), ui.Hex("#6C7600"),
	ui.Hex("#BE3346"), ui.Hex("#4E6572"), ui.Hex("#8A6A4A"),
}

// Font families. The app registers Geist and Geist Mono (OFL) at start; the
// system supplies the CJK fallback (Microsoft YaHei UI, Noto Sans SC).
const (
	FontUI   = "Geist"
	FontMono = "Geist Mono"
)

// Radii and heights of the system.
const (
	RadiusControl = 6
	RadiusCard    = 10
	RadiusPanel   = 12
	ControlHeight = 32
)

// DialogShadow is the one shadow of the system: popovers, dialogs and floating
// previews. Cards and groups use a border instead.
func DialogShadow(e ui.Element, t *ui.Theme) ui.Element {
	p := P(t)
	return e.Shadow(0, 12, 32, 0, p.Shadow).Shadow(0, 2, 6, 0, p.Shadow2)
}
