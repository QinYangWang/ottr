package native

import "github.com/egoist/mygo/ui"

// themeFor applies DESIGN.md's gallery-white, monochrome palette. Media and
// semantic recording states keep their own color; the chrome does not.
func themeFor(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	t.Dark = dark
	t.Radius = 16
	t.Spacing = 4
	t.FontSize = 14

	if dark {
		t.Background = ui.Hex("#141414")
		t.Surface = ui.Hex("#242424")
		t.SurfaceHover = ui.Hex("#2e2e2e")
		t.SurfacePressed = ui.Hex("#383838")
		t.Border = ui.Hex("#383838")
		t.Text = ui.Hex("#f5f5f5")
		t.TextMuted = ui.Hex("#adadad")
		t.Accent = ui.Hex("#f5f5f5")
		t.AccentHover = ui.Hex("#ffffff")
		t.AccentPressed = ui.Hex("#e0e0e0")
		t.AccentText = ui.Hex("#141414")
		t.Danger = ui.Hex("#ff8b96")
		t.Warning = ui.Hex("#f0b46b")
		t.Success = ui.Hex("#77d3a1")
		t.Selection = ui.Hex("#f5f5f5").Alpha(0.25)
		t.Focus = ui.Hex("#f5f5f5").Alpha(0.65)
		t.Scrollbar = ui.Hex("#ffffff").Alpha(0.18)
		t.ScrollbarWidth = 8
		return t
	}

	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#f3f3f3")
	t.SurfaceHover = ui.Hex("#ececec")
	t.SurfacePressed = ui.Hex("#e0e0e0")
	t.Border = ui.Hex("#e0e0e0")
	t.Text = ui.Hex("#141414")
	t.TextMuted = ui.Hex("#707070")
	t.Accent = ui.Hex("#141414")
	t.AccentHover = ui.Hex("#262626")
	t.AccentPressed = ui.Hex("#000000")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#c3273a")
	t.Warning = ui.Hex("#a86413")
	t.Success = ui.Hex("#16794a")
	t.Selection = ui.Hex("#141414").Alpha(0.18)
	t.Focus = ui.Hex("#141414").Alpha(0.6)
	t.Scrollbar = ui.Hex("#000000").Alpha(0.18)
	t.ScrollbarWidth = 8
	return t
}

// applyTheme is the look of the frame, applied before anything is drawn so
// that the window, its widgets and the elements built here all share it.
func (a *App) applyTheme(c *ui.Context) *ui.Theme {
	t := themeFor(c.Theme().Dark)
	t.Font = a.font
	if t.Font == "" {
		t.Font = "Noto Sans"
	}
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
