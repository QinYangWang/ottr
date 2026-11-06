package native

import "github.com/egoist/mygo/ui"

// themeFor applies Interfere's Light/Dark palette. Controls use its 8px
// rounded-lg radius; media and semantic states keep their own colors.
func themeFor(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	t.Dark = dark
	t.Radius = 8
	t.Spacing = 4
	t.FontSize = 14

	if dark {
		t.Background = ui.Hex("#000000")
		t.Surface = ui.Hex("#ffffff").Alpha(0.07)
		t.SurfaceHover = ui.Hex("#ffffff").Alpha(0.105)
		t.SurfacePressed = ui.Hex("#ffffff").Alpha(0.133)
		t.Border = ui.Hex("#ffffff").Alpha(0.07)
		t.Text = ui.Hex("#ffffff").Alpha(0.93)
		t.TextMuted = ui.Hex("#ffffff").Alpha(0.69)
		t.Accent = ui.Hex("#0090ff")
		t.AccentHover = ui.Hex("#3b9eff")
		t.AccentPressed = ui.Hex("#70b8ff")
		t.AccentText = ui.Hex("#ffffff")
		t.Danger = ui.Hex("#ec6142")
		t.Warning = ui.Hex("#ffa057")
		t.Success = ui.Hex("#3dd68c")
		t.Selection = ui.Hex("#0090ff").Alpha(0.35)
		t.Focus = ui.Hex("#0090ff")
		t.Scrollbar = ui.Hex("#ffffff").Alpha(0.18)
		t.ScrollbarWidth = 8
		return t
	}

	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#000000").Alpha(0.06)
	t.SurfaceHover = ui.Hex("#000000").Alpha(0.09)
	t.SurfacePressed = ui.Hex("#000000").Alpha(0.12)
	t.Border = ui.Hex("#000000").Alpha(0.06)
	t.Text = ui.Hex("#000000").Alpha(0.875)
	t.TextMuted = ui.Hex("#000000").Alpha(0.61)
	t.Accent = ui.Hex("#0090ff")
	t.AccentHover = ui.Hex("#0588f0")
	t.AccentPressed = ui.Hex("#0d74ce")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#e54d2e")
	t.Warning = ui.Hex("#f76b15")
	t.Success = ui.Hex("#30a46c")
	t.Selection = ui.Hex("#0090ff").Alpha(0.22)
	t.Focus = ui.Hex("#0084e6").Alpha(0.63)
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

// popoverShadow applies Interfere's Extra Large elevation to menus.
func popoverShadow(e ui.Element, t *ui.Theme) ui.Element {
	return e.Shadow(0, 8, 40, 0, ui.RGBA(0, 0, 0, 0.05)).
		Shadow(0, 12, 32, -16, ui.RGBA(0, 0, 0, 0.15))
}

// cardShadow applies Interfere's Default elevation to a raised card.
func cardShadow(e ui.Element, t *ui.Theme) ui.Element {
	if t.Dark {
		return e.Shadow(0, 1, 2, 0, ui.RGBA(255, 255, 255, 0.07))
	}
	return e.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.1))
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
