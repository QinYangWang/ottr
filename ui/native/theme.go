package native

import "github.com/egoist/mygo/ui"

// The look of the three windows: the palette, radii and type of the HTML
// prototype (docs/prototypes/native-ui.html). mygo supplies a neutral theme;
// these keep every window on the same three surfaces — the window's own, the
// raised one of cards and rows, and the sunken one of navigation and fields.
func themeFor(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	t.Dark = dark
	t.Radius = 10
	t.Spacing = 4
	t.FontSize = 13

	if dark {
		t.Background = ui.Hex("#0f1013")
		t.Surface = ui.Hex("#17181d")
		t.SurfaceHover = ui.Hex("#24262d")
		t.SurfacePressed = ui.Hex("#2c2e36")
		t.Border = ui.Hex("#272830")
		t.Text = ui.Hex("#f2f2f5")
		t.TextMuted = ui.Hex("#a0a0ac")
		t.Accent = ui.Hex("#98a6ff")
		t.AccentHover = ui.Hex("#b0bbff")
		t.AccentPressed = ui.Hex("#8494ff")
		t.AccentText = ui.Hex("#101227")
		t.Danger = ui.Hex("#ff8b96")
		t.Warning = ui.Hex("#f0b46b")
		t.Success = ui.Hex("#77d3a1")
		t.Selection = ui.Hex("#98a6ff").Alpha(0.35)
		t.Focus = ui.Hex("#98a6ff").Alpha(0.6)
		t.Scrollbar = ui.Hex("#ffffff").Alpha(0.18)
		t.ScrollbarWidth = 8
		return t
	}

	t.Background = ui.Hex("#f5f5f7")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#ededf2")
	t.SurfacePressed = ui.Hex("#e3e3ea")
	t.Border = ui.Hex("#e2e2e9")
	t.Text = ui.Hex("#17171b")
	t.TextMuted = ui.Hex("#6d6d78")
	t.Accent = ui.Hex("#4658d6")
	t.AccentHover = ui.Hex("#3b4bc0")
	t.AccentPressed = ui.Hex("#33409f")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#c3273a")
	t.Warning = ui.Hex("#a86413")
	t.Success = ui.Hex("#16794a")
	t.Selection = ui.Hex("#4658d6").Alpha(0.22)
	t.Focus = ui.Hex("#4658d6").Alpha(0.55)
	t.Scrollbar = ui.Hex("#000000").Alpha(0.18)
	t.ScrollbarWidth = 8
	return t
}

// applyTheme is the look of the frame, applied before anything is drawn so
// that the window, its widgets and the elements built here all share it.
func (a *App) applyTheme(c *ui.Context) *ui.Theme {
	t := themeFor(c.Theme().Dark)
	c.SetTheme(t)
	return t
}

// mediaBackdrop is the dark ground thumbnails, the player and the recording
// preview share, so that media always sits on the same surface in both looks.
func mediaBackdrop(dark bool) (from, to ui.Color) {
	if dark {
		return ui.Hex("#1e2637"), ui.Hex("#0b1017")
	}
	return ui.Hex("#33456a"), ui.Hex("#121a26")
}

// playerBar is the strip under the player's picture, a shade darker than the
// picture so that the two read as one unit.
func playerBar(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#12141a")
	}
	return ui.Hex("#0d1119")
}

// onMedia is the text drawn over media — chips, a duration, a badge — which is
// light in both looks, because the ground under it always is.
func onMedia() ui.Color { return ui.Hex("#f4f6fa") }
