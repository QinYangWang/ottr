package native

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"

	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/settings"
)

// subtitleSettingsMenu is the shared two-level settings menu for recording
// and playback. Submenus float left of the main item and never get clipped by
// the main popover.
func (a *App) subtitleSettingsMenu(c *ui.Context, t *ui.Theme, m *library.Media) {
	button := ui.ButtonBase(c).Label(a.tr("Settings")).Tooltip(a.tr("Settings")).TextColor(t.TextMuted).
		Padding(t.Space(1.5)).Radius(t.Radius)
	button.Children(func() { ui.Icon(c, iconSliders).FontSize(t.FontSize + 3) })
	if button.Hovered() {
		button.Background(t.SurfaceHover)
	}
	if button.Clicked() {
		a.subtitleSettingsOpen = !a.subtitleSettingsOpen
		a.languageMenuOpen, a.transcriptionMenuOpen = false, false
		a.translationMenuOpen, a.styleMenuOpen = false, false
	}
	ui.PopoverBase(c, button, &a.subtitleSettingsOpen, func(panel ui.Element) {
		panel.Width(360).Padding(t.Space(2)).Radius(t.Radius).
			Background(t.Background).Border(1, t.Border)
		popoverShadow(panel, t)
		ui.Column(c).Gap(t.Space(1)).Children(func() {
			item := a.settingsMenuItem(c, t, "Spoken language", a.langLabel(a.defaultLanguage), func() {
				a.toggleSettingsSubmenu("language")
			})
			a.settingsSubmenu(c, t, item, &a.languageMenuOpen, func() { a.languageSubmenu(c, t, m) })
			modelName := ""
			if a.MM != nil {
				modelName = a.MM.CurrentName()
			}
			item = a.settingsMenuItem(c, t, "Transcription model", modelName, func() {
				a.toggleSettingsSubmenu("transcription")
			})
			a.settingsSubmenu(c, t, item, &a.transcriptionMenuOpen, func() { a.modelSubmenu(c, t) })
			item = a.settingsMenuItem(c, t, "Translation model", a.translationProviderName(), func() {
				a.toggleSettingsSubmenu("translation")
			})
			a.settingsSubmenu(c, t, item, &a.translationMenuOpen, func() { a.translationSubmenu(c, t) })
			if m != nil {
				a.settingsValueItem(c, t, "Translation", a.onOff(a.showTranslation), func() {
					a.showTranslation = !a.showTranslation
					a.setTranslationVisible(*m, a.showTranslation)
				})
			}
			item = a.settingsMenuItem(c, t, "Subtitle style", fmt.Sprintf("%.0f px", a.subtitleSize()), func() {
				a.toggleSettingsSubmenu("style")
			})
			a.settingsSubmenu(c, t, item, &a.styleMenuOpen, func() { a.styleSubmenu(c, t) })
			if m != nil {
				a.settingsValueItem(c, t, "Transcribe again", "", func() {
					a.closeSettingsMenus()
					a.startTranscribe(m.ID)
				})
				if a.showTranslation && len(a.trans.Segments) > 0 &&
					!a.transLoading && !a.jobRunning(jobs.KindTranslate) {
					a.settingsValueItem(c, t, "Translate again", "", func() {
						a.closeSettingsMenus()
						a.startTranslate(m.ID)
					})
				}
			}
		})
	})
}

// onOff is the value a toggle menu row shows.
func (a *App) onOff(on bool) string {
	if on {
		return a.tr("On")
	}
	return a.tr("Off")
}

// settingsValueItem is a menu row that acts or toggles in place: no submenu
// arrow, and an optional value on the right.
func (a *App) settingsValueItem(c *ui.Context, t *ui.Theme, label, value string, onClick func()) {
	item := ui.ButtonBase(c).Label(a.tr(label)).FillWidth().Height(44).Padding(0, t.Space(3)).
		Radius(t.Space(2)).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if item.Hovered() {
		item.Background(t.SurfaceHover)
	}
	item.Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr(label)).FontSize(t.FontSize - 1)
			if value != "" {
				ui.Spacer(c)
				ui.Text(c, value).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			}
		})
	})
	if item.Clicked() {
		onClick()
	}
}

func (a *App) settingsMenuItem(c *ui.Context, t *ui.Theme, label, value string, onClick func()) ui.Element {
	item := ui.ButtonBase(c).Label(label).FillWidth().Height(44).Padding(0, t.Space(3)).
		Radius(t.Space(2)).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if item.Hovered() {
		item.Background(t.SurfaceHover)
	}
	item.Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr(label)).FontSize(t.FontSize - 1)
			ui.Spacer(c)
			ui.Text(c, value).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine().MinWidth(0)
			ui.Text(c, "›").FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		})
	})
	if item.Clicked() {
		onClick()
	}
	return item
}

func (a *App) toggleSettingsSubmenu(which string) {
	language, transcription := a.languageMenuOpen, a.transcriptionMenuOpen
	translation, style := a.translationMenuOpen, a.styleMenuOpen
	a.languageMenuOpen = which == "language" && !language
	a.transcriptionMenuOpen = which == "transcription" && !transcription
	a.translationMenuOpen = which == "translation" && !translation
	a.styleMenuOpen = which == "style" && !style
}

func (a *App) closeSettingsMenus() {
	a.subtitleSettingsOpen = false
	a.languageMenuOpen, a.transcriptionMenuOpen = false, false
	a.translationMenuOpen, a.styleMenuOpen = false, false
}

func (a *App) settingsSubmenu(c *ui.Context, t *ui.Theme, anchor ui.Element, open *bool, build func()) {
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorTopEnd).Width(300).
			Padding(t.Space(2)).Radius(t.Radius).Background(t.Background).Border(1, t.Border)
		popoverShadow(panel, t)
		build()
	})
}

func (a *App) languageSubmenu(c *ui.Context, t *ui.Theme, m *library.Media) {
	ui.Scroll(c).MaxHeight(320).Children(func() {
		ui.Column(c).Gap(t.Space(1)).Children(func() {
			_, codes := a.langChoices()
			for i, code := range codes {
				name := a.langLabel(code)
				a.settingsOption(c, t, name, code == a.defaultLanguage, func() {
					a.setDefaultLanguage(code, m)
					a.closeSettingsMenus()
				})
				_ = i
			}
		})
	})
}

func (a *App) modelSubmenu(c *ui.Context, t *ui.Theme) {
	if !a.modelLoaded {
		a.loadModels()
	}
	ui.Scroll(c).MaxHeight(320).Children(func() {
		ui.Column(c).Gap(t.Space(1)).Children(func() {
			if a.modelLoading {
				ui.Spinner(c).Label(a.tr("Looking for models"))
				return
			}
			for _, info := range a.modelList {
				if !info.Installed {
					continue
				}
				a.settingsOption(c, t, info.Name, info.Path == a.modelInUse, func() {
					a.modelInUse = info.Path
					a.useModel(info)
					a.closeSettingsMenus()
				})
			}
		})
	})
}

func (a *App) translationProviderName() string {
	if a.transCfg.Provider == "custom" && a.transCfg.Model != "" {
		return a.transCfg.Model
	}
	if a.transCfg.Provider == "custom" {
		return a.tr("Custom AI model")
	}
	return a.tr("Free online")
}

func (a *App) translationSubmenu(c *ui.Context, t *ui.Theme) {
	if !a.loadedTrans {
		a.loadTransSettings()
	}
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		a.settingsOption(c, t, a.tr("Free online"), a.transCfg.Provider != "custom", func() {
			a.transCfg.Provider = "mymemory"
			a.saveTransSettings()
			a.closeSettingsMenus()
		})
		a.settingsOption(c, t, a.tr("Custom AI model"), a.transCfg.Provider == "custom", func() {
			a.transCfg.Provider = "custom"
			a.saveTransSettings()
			a.closeSettingsMenus()
		})
	})
}

func (a *App) styleSubmenu(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		for _, size := range []float32{12, 14, 16, 18, 20} {
			a.settingsOption(c, t, fmt.Sprintf("%.0f px", size), size == a.subtitleSize(), func() {
				a.subtitleFontSize = size
				if a.Prefs != nil {
					go a.Prefs.Set(settings.SubtitleFontSize, strconv.Itoa(int(size)))
				}
				a.closeSettingsMenus()
			})
		}
	})
}

func (a *App) settingsOption(c *ui.Context, t *ui.Theme, label string, selected bool, onClick func()) {
	item := ui.ButtonBase(c).Role(ui.RoleMenuItem).Label(label).Checked(selected).
		FillWidth().Height(40).Padding(0, t.Space(3)).Radius(9999).AlignItems(ui.Center)
	if selected {
		item.Background(t.Surface)
	} else if item.Hovered() {
		item.Background(t.SurfaceHover)
	}
	item.Children(func() {
		ui.Text(c, label).FontSize(t.FontSize - 1).SingleLine().Grow(1).MinWidth(0)
		if selected {
			ui.Icon(c, iconCheck).FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		}
	})
	if item.Clicked() {
		onClick()
	}
}

func (a *App) subtitleSize() float32 {
	if a.subtitleFontSize <= 0 {
		return 14
	}
	return a.subtitleFontSize
}

func (a *App) setDefaultLanguage(code string, m *library.Media) {
	a.defaultLanguage, a.lang = code, code
	if a.Prefs != nil {
		go a.Prefs.Set(settings.TranscriptionLanguage, code)
	}
	if m != nil {
		a.setLanguage(m.ID, code)
	}
}
