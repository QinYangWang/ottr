package native

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// fallbackFonts are offered until the platform's families have been read: the
// ones every Windows and macOS install has under one of these names.
var fallbackFonts = []string{
	"Inter", "Noto Sans", "Noto Sans SC", "Segoe UI", "Microsoft YaHei UI", "Georgia",
}

// loadFontFamilies reads the families text can be set in, once, off the frame:
// MyGo lists its registered faces and then the system's, which is exactly what
// Font() resolves — a family from anywhere else would be accepted and then
// silently fall back.
func (a *App) loadFontFamilies() {
	if a.fontsLoaded || a.fontsLoading {
		return
	}
	a.fontsLoading = true
	go func() {
		var families []string
		for _, family := range ui.FontFamilies() {
			families = append(families, family.Name)
		}
		a.update(func() {
			a.fontFamilies, a.fontsLoaded, a.fontsLoading = families, true, false
		})
	}()
}

// fontChoices is the display-font list: the default first, the installed
// families after it, and whatever is selected even when it is no longer
// installed — dropping it would silently change the font.
func (a *App) fontChoices() []string {
	families := a.fontFamilies
	if len(families) == 0 {
		families = fallbackFonts
	}
	out := []string{a.tr("Geist (default)")}
	seen := map[string]bool{}
	for _, family := range append(append([]string{}, families...), a.font) {
		key := strings.ToLower(family)
		if family == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, family)
	}
	return out
}
