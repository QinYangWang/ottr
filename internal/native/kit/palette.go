package kit

import "github.com/egoist/mygo/ui"

// Palette is the "studio console + highlighter" system the Doop canvas is
// drawn in (docs/design/tokens.css): graphite ink on cool paper, one amber
// accent for what is current, and red only for recording and destruction.
// ui.Theme carries the few colours MyGo's own widgets read; everything the
// app draws itself takes its colour from here.
type Palette struct {
	Paper, Sheet, Sunk, Hover             ui.Color // chrome, content, inset wells, hover
	Ink, Ink2, Ink3, Ink4                 ui.Color // text: primary, secondary, meta, disabled
	Line, Line2                           ui.Color // dividers and card borders; control borders
	Amber, AmberSoft, AmberInk, AmberLine ui.Color
	Red, RedSoft, RedInk                  ui.Color
	Green, GreenSoft                      ui.Color
	Btn, BtnInk                           ui.Color // the primary button's fill and text
	Media                                 ui.Color // video stage and thumbnails
	SwitchOn                              ui.Color
	Shadow, Shadow2                       ui.Color
}

var (
	light = Palette{
		Paper: ui.Hex("#F3F4F5"), Sheet: ui.Hex("#FFFFFF"), Sunk: ui.Hex("#F7F8F9"), Hover: ui.Hex("#EEF0F2"),
		Ink: ui.Hex("#121417"), Ink2: ui.Hex("#4A5058"), Ink3: ui.Hex("#7C838C"), Ink4: ui.Hex("#A6ACB3"),
		Line: ui.Hex("#E4E6E9"), Line2: ui.Hex("#CDD1D6"),
		Amber: ui.Hex("#FFB020"), AmberSoft: ui.Hex("#FFF4DB"), AmberInk: ui.Hex("#8A5600"), AmberLine: ui.Hex("#F5C46B"),
		Red: ui.Hex("#E5383B"), RedSoft: ui.Hex("#FDECEC"), RedInk: ui.Hex("#C42B2E"),
		Green: ui.Hex("#1F9D61"), GreenSoft: ui.Hex("#E4F5EC"),
		Btn: ui.Hex("#121417"), BtnInk: ui.Hex("#FFFFFF"),
		Media:    ui.Hex("#1B1E22"),
		SwitchOn: ui.Hex("#121417"),
		Shadow:   ui.RGBA(17, 20, 23, 0.14),
		Shadow2:  ui.RGBA(17, 20, 23, 0.06),
	}
	dark = Palette{
		Paper: ui.Hex("#0E0F11"), Sheet: ui.Hex("#16181B"), Sunk: ui.Hex("#1B1E22"), Hover: ui.Hex("#22262B"),
		Ink: ui.Hex("#EDEEF0"), Ink2: ui.Hex("#A9AFB7"), Ink3: ui.Hex("#737A83"), Ink4: ui.Hex("#555B62"),
		Line: ui.Hex("#2A2E33"), Line2: ui.Hex("#3A3F46"),
		Amber: ui.Hex("#FFB020"), AmberSoft: ui.Hex("#33270F"), AmberInk: ui.Hex("#FFC453"), AmberLine: ui.Hex("#6B4E14"),
		Red: ui.Hex("#E5383B"), RedSoft: ui.Hex("#3A1516"), RedInk: ui.Hex("#FF6B6E"),
		Green: ui.Hex("#3DD68C"), GreenSoft: ui.Hex("#12301F"),
		Btn: ui.Hex("#EDEEF0"), BtnInk: ui.Hex("#121417"),
		Media:    ui.Hex("#0A0B0D"),
		SwitchOn: ui.Hex("#FFB020"),
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
	ui.Hex("#3E63DD"), ui.Hex("#12A594"), ui.Hex("#D6409F"),
	ui.Hex("#8E4EC6"), ui.Hex("#E54666"), ui.Hex("#978365"),
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
