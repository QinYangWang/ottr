package native

import (
	"embed"
	"sync"

	"github.com/egoist/mygo/ui"

	"ottr/internal/native/kit"
)

// The UI is set in Geist and its numbers in Geist Mono (SIL Open Font
// License 1.1, fonts/OFL.txt). They are registered once, before the first
// frame; the system's fonts supply what they lack, such as CJK.
//
//go:embed fonts/*.ttf
var fontFiles embed.FS

var registerFonts = sync.OnceValue(func() error {
	files := []struct{ name, family string }{
		{"Geist-Regular.ttf", kit.FontUI},
		{"Geist-Medium.ttf", kit.FontUI},
		{"Geist-SemiBold.ttf", kit.FontUI},
		{"Geist-Bold.ttf", kit.FontUI},
		{"GeistMono-Regular.ttf", kit.FontMono},
		{"GeistMono-Medium.ttf", kit.FontMono},
		{"GeistMono-SemiBold.ttf", kit.FontMono},
	}
	for _, f := range files {
		data, err := fontFiles.ReadFile("fonts/" + f.name)
		if err != nil {
			return err
		}
		if err := ui.RegisterFont(data, f.family); err != nil {
			return err
		}
	}
	return nil
})

// themeFor maps the "studio console + highlighter" palette onto the colours
// MyGo's own widgets read. The accent of ui.Theme is the ink, because that is
// what fills a primary button, a checked control and a selected tab; the
// highlighter amber is drawn by the app itself (kit.Palette).
func themeFor(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	p := kit.P(&ui.Theme{Dark: dark})
	t.Dark = dark
	t.Radius = kit.RadiusControl
	t.Spacing = 4
	t.FontSize = 13

	t.Background = p.Sheet
	t.Surface = p.Sunk
	t.SurfaceHover = p.Hover
	t.SurfacePressed = p.Line
	t.Border = p.Line
	t.Text = p.Ink
	t.TextMuted = p.Ink3
	t.Accent = p.Btn
	t.AccentHover = p.Btn.Mix(ui.RGB(128, 128, 128), 0.14)
	t.AccentPressed = p.Btn.Mix(ui.RGB(128, 128, 128), 0.24)
	t.AccentText = p.BtnInk
	t.Danger = p.Red
	t.Warning = p.Amber
	t.Success = p.Green
	t.Selection = p.Amber.Alpha(0.35)
	t.Focus = p.Amber
	t.Inverse = p.Ink
	t.InverseText = p.Sheet
	t.Scrollbar = p.Ink.Alpha(0.18)
	t.ScrollbarWidth = 6
	return t
}

// applyTheme is the look of the frame, applied before anything is drawn so
// that the window, its widgets and the elements built here all share it.
func (a *App) applyTheme(c *ui.Context) *ui.Theme {
	_ = registerFonts()
	t := themeFor(c.Theme().Dark)
	t.Font = a.font
	if t.Font == "" {
		t.Font = kit.FontUI
	}
	c.SetTheme(t)
	return t
}

// pal is the palette of the look t is in.
func pal(t *ui.Theme) kit.Palette { return kit.P(t) }

// popoverShadow is the system's one shadow, for menus and popovers.
func popoverShadow(e ui.Element, t *ui.Theme) ui.Element { return kit.DialogShadow(e, t) }

// mono sets text in Geist Mono with tabular figures.
func mono(e ui.Element) ui.Element { return kit.Mono(e) }

// onMedia is the text drawn over media — a duration, a badge — which is light
// in both looks, because the ground under it always is dark.
func onMedia() ui.Color { return ui.Hex("#FFFFFF") }
