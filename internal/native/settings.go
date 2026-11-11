package native

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/native/kit"
	"ottr/internal/settings"
	"ottr/internal/summary"
	"ottr/internal/translate"
)

// openSettings shows the Settings section in this window.
func (a *App) openSettings() {
	a.settingsOpen = true
}

// themeBackground is the window background of the look chosen.
func themeBackground() string {
	if mygo.Theme.IsDark() {
		return "#16181B"
	}
	return "#FFFFFF"
}

// settingsSections are the pages Settings offers, in the order they are shown,
// with the line under each page's title.
var settingsSections = []struct {
	label, detail string
}{
	{"Appearance", "The window redraws at once, and remembers the choice."},
	{"Transcription", "Transcription runs offline, with the model chosen here."},
	{"Provider", "Used for translation and summaries. Audio and transcription never leave this computer."},
	{"Speakers", "Voices Ottr has heard. A new name applies to every transcript."},
}

// closeSettings shows the library again.
func (a *App) closeSettings() {
	a.settingsOpen = false
}

// settingsPane is the Settings section where the window has no sidebar for it:
// the way back and the section chosen on top, its page under it.
func (a *App) settingsPane(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.Sheet).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Height(52).Padding(0, 16).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
			if a.backButton(c, t, "") {
				a.closeSettings()
			}
			tab := a.settingsTab
			options := make([]kit.SelectOption[int], len(settingsSections))
			for i, section := range settingsSections {
				options[i] = kit.SelectOption[int]{Value: i, Label: a.tr(section.label)}
			}
			kit.SelectBox(c, t, &tab, options, a.tr("Settings"), 220, func(tab int) { a.settingsTab = tab })
		})
		a.settingsPage(c, t)
	})
}

// settingsPage is the scrollable page chosen on the window's left: its title
// and what it is for, then its groups.
func (a *App) settingsPage(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	section := settingsSections[a.settingsTab]
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(28, 32, 32, 32).Gap(18).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, a.tr(section.label)).FontSize(20).FontWeight(600).LetterSpacing(-0.3).TextColor(p.Ink)
					ui.Text(c, a.tr(section.detail)).FontSize(12.5).TextColor(p.Ink2)
				})
				if a.settingsTab == 3 && len(a.speakerLib) > 0 {
					ui.Row(c).Width(200).Height(32).Padding(0, 10).Gap(8).AlignItems(ui.Center).
						Radius(kit.RadiusControl).Border(1, p.Line2).Children(func() {
						ui.Icon(c, iconSearch).FontSize(15).TextColor(p.Ink3)
						ui.TextInputBase(c, &a.speakerQuery).Label(a.tr("Search speakers")).
							Placeholder(a.tr("Search speakers")).PlaceholderColor(p.Ink3).Grow(1).MinWidth(0).FontSize(13)
					})
				}
			})
			switch a.settingsTab {
			case 1:
				a.transcriptionSettings(c, t)
			case 2:
				a.providerSettings(c, t)
			case 3:
				a.speakersSettings(c, t)
			default:
				a.appearanceSettings(c, t)
			}
		})
	})
}

// --------------------------------------------------------------- appearance

// appearanceSettings picks the language, the look and the font.
func (a *App) appearanceSettings(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	kit.Group(c, t, func() {
		kit.GroupRow(c, t, true, a.tr("Language"), a.tr("Choose the language of the interface."), func() {
			language := "English"
			if a.language == "zh" {
				language = "中文（中国）"
			}
			kit.SelectBox(c, t, &language, []kit.SelectOption[string]{
				{Value: "English", Label: "English"},
				{Value: "中文（中国）", Label: "中文（中国）"},
			}, a.tr("Language"), 220, func(choice string) {
				locale := "en"
				if choice == "中文（中国）" {
					locale = "zh"
				}
				if locale != a.language && !(locale == "en" && a.language == "") {
					a.setInterfaceLanguage(locale)
				}
			})
		})
	})
	kit.Group(c, t, func() {
		ui.Column(c).FillWidth().Padding(14, 16, 16, 16).Gap(12).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, a.tr("Theme")).FontSize(13).FontWeight(500).TextColor(p.Ink)
				ui.Text(c, a.tr("Match the desktop, or pick a fixed look.")).FontSize(12).TextColor(p.Ink3)
			})
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				for _, choice := range []struct{ source, label string }{
					{"system", a.tr("System")},
					{"light", a.tr("Light")},
					{"dark", a.tr("Dark")},
				} {
					a.themeCard(c, t, choice.source, choice.label)
				}
			})
		})
	})
	kit.Group(c, t, func() {
		kit.GroupRow(c, t, true, a.tr("Display font"), a.tr("Use Geist, or pick an installed family."), func() {
			defaultFont := a.tr("Geist (default)")
			font := a.font
			if font == "" {
				font = defaultFont
			}
			options := []string{defaultFont, "Inter", "Noto Sans", "Noto Sans SC", "Segoe UI", "Microsoft YaHei UI", "Georgia"}
			if !slices.Contains(options, font) {
				options = append(options, font)
			}
			choices := make([]kit.SelectOption[string], len(options))
			for i, name := range options {
				choices[i] = kit.SelectOption[string]{Value: name, Label: name}
			}
			kit.SelectBox(c, t, &font, choices, a.tr("Display font"), 220, func(choice string) {
				if choice == defaultFont {
					choice = ""
				}
				if choice != a.font {
					a.font = choice
					a.saveFont()
				}
			})
		})
		kit.GroupRow(c, t, false, a.tr("Accent"), a.tr("Highlights the line playing, search matches and selection."), func() {
			ui.Row(c).Height(24).Padding(0, 8).Gap(6).Radius(9999).AlignItems(ui.Center).Background(p.AmberSoft).Children(func() {
				ui.Box(c).Size(10, 10).Radius(3).Background(p.Amber)
				ui.Text(c, a.tr("Highlighter")).FontSize(11.5).FontWeight(500).TextColor(p.AmberInk)
			})
		})
	})
}

// themeCard is one look to pick, drawn as a small window in it.
func (a *App) themeCard(c *ui.Context, t *ui.Theme, source, label string) {
	p := pal(t)
	chosen := a.themeSource() == source
	card := ui.ButtonBase(c.Key(source)).Label(label).Role(ui.RoleRadio).Checked(chosen).Grow(1).Width(0).
		Padding(10).Gap(10).Radius(kit.RadiusCard).Cursor(ui.CursorPointer).Background(p.Sheet)
	if chosen {
		card.Border(1, p.Ink).InsetShadow(0, 0, 0, 1, p.Ink)
	} else {
		card.Border(1, p.Line2)
		if card.Hovered() {
			card.Background(p.Sunk)
		}
	}
	card.Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			ui.Row(c).FillWidth().Height(76).Radius(kit.RadiusControl).Clip().Border(1, p.Line).Children(func() {
				switch source {
				case "system":
					themeMock(c, false)
					themeMock(c, true)
				case "dark":
					themeMock(c, true)
				default:
					themeMock(c, false)
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				radio(c, t, chosen)
				ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.Ink)
			})
		})
	})
	if card.Clicked() && !chosen {
		a.setTheme(source)
	}
}

// themeMock draws a miniature window — a sidebar, an amber line and two grey
// ones — in the colours of one look.
func themeMock(c *ui.Context, dark bool) {
	q := kit.P(&ui.Theme{Dark: dark})
	ui.Row(c).Grow(1).FillHeight().AlignItems(ui.Stretch).Background(q.Sheet).Children(func() {
		ui.Box(c).Width(22).Shrink(0).FillHeight().Background(q.Paper).BorderWidth(0, 1, 0, 0).BorderColor(q.Line)
		ui.Column(c).Grow(1).Padding(10, 8).Gap(5).AlignItems(ui.Start).Children(func() {
			ui.Box(c).Height(4).Width(30).Radius(2).Background(q.Amber)
			ui.Box(c).Height(3).Width(46).Radius(2).Background(q.Line2)
			ui.Box(c).Height(3).Width(34).Radius(2).Background(q.Line2)
		})
	})
}

// themeSource is the look chosen: system when nothing was.
func (a *App) themeSource() string {
	if a.theme == "" {
		return "system"
	}
	return a.theme
}

// setTheme picks a look, applies it at once and remembers it.
func (a *App) setTheme(source string) {
	a.theme = source
	if a.win != nil {
		mygo.Theme.SetSource(mygo.ThemeSource(source))
		a.win.SetBackgroundColor(themeBackground())
	}
	if a.Prefs != nil {
		prefs := a.Prefs
		go prefs.Set(settings.Theme, source)
	}
}

func (a *App) setInterfaceLanguage(language string) {
	a.language = language
	if a.Prefs != nil {
		prefs := a.Prefs
		go prefs.Set(settings.Language, language)
	}
}

func (a *App) saveFont() {
	if a.Prefs != nil {
		prefs, font := a.Prefs, a.font
		go prefs.Set(settings.Font, font)
	}
}

// settingRow is one setting: what it is, what it does, and its control on the
// right.
func (a *App) settingRow(c *ui.Context, t *ui.Theme, title, detail string, control func()) {
	kit.SettingRow(c, t, a.tr(title), a.tr(detail), control)
}

// ------------------------------------------------------------ transcription

func (a *App) subtitleSettingsButton(c *ui.Context, t *ui.Theme, m *library.Media) {
	a.subtitleSettingsMenu(c, t, m)
}

func (a *App) transcriptionSettings(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if a.MM == nil {
		ui.Text(c, a.tr("No model manager is configured.")).FontSize(13).TextColor(p.Ink3)
		return
	}
	if !a.modelLoaded {
		a.loadModels()
	}
	if a.modelLoaded && !a.modelLoading && a.modelInUse == "" {
		ui.Row(c).FillWidth().Padding(12, 14).Gap(10).AlignItems(ui.Start).
			Radius(kit.RadiusCard).Border(1, p.AmberLine).Background(p.AmberSoft).Children(func() {
			ui.Icon(c, iconDownload).FontSize(16).TextColor(p.AmberInk).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, a.tr("No speech recognition model yet")).FontSize(13).FontWeight(600).TextColor(p.Ink)
				ui.Text(c, a.tr("Download one below to transcribe on this computer.")).FontSize(12.5).TextColor(p.Ink2)
			})
		})
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		kit.Label(c, t, a.tr("Speech recognition model"))
		kit.Group(c, t, func() {
			if a.modelLoading {
				ui.Row(c).Padding(14, 16).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Label(a.tr("Looking for models")).Size(14, 14)
					ui.Text(c, a.tr("Looking for models…")).FontSize(12.5).TextColor(p.Ink3)
				})
			}
			for i, offer := range a.modelList {
				a.modelRow(c.Key(offer.ID), t, offer, i == 0 && !a.modelLoading)
			}
			ui.Row(c).FillWidth().Padding(10, 16).Gap(10).AlignItems(ui.Center).Background(p.Sunk).
				BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
				ui.Icon(c, iconFolderOpen).FontSize(16).TextColor(p.Ink2)
				mono(ui.Text(c, a.MM.ModelsDir())).FontSize(11.5).TextColor(p.Ink2).SingleLine().Grow(1).MinWidth(0).
					Tooltip(a.MM.ModelsDir())
				if kit.Button(c, t, kit.Ghost, nil, a.tr("Open folder"), false).
					Tooltip(a.tr("Open the folder models are installed in")).Clicked() {
					mygo.Shell.OpenPath(a.MM.ModelsDir())
				}
				if kit.Button(c, t, kit.Secondary, iconRefresh, a.tr("Look for models again"), false).
					Label(a.tr("Re-check")).Clicked() {
					a.loadModels()
				}
			})
		})
	})
	kit.Group(c, t, func() {
		kit.GroupRow(c, t, true, a.tr("Spoken language"), a.tr("Auto reads the script of the recognized text."), func() {
			names, codes := a.langChoices()
			lang := a.langLabel(a.defaultLanguage)
			options := make([]kit.SelectOption[string], len(names))
			for i, name := range names {
				options[i] = kit.SelectOption[string]{Value: name, Label: name}
			}
			kit.SelectBox(c, t, &lang, options, a.tr("Default spoken language"), 180, func(choice string) {
				for i, name := range names {
					if name == choice && codes[i] != a.defaultLanguage {
						a.setDefaultLanguage(codes[i], nil)
					}
				}
			})
		})
	})
}

// modelRow is one supported model: In use, Use this model, Download, how far
// its download has come, or what the last attempt failed with.
func (a *App) modelRow(c *ui.Context, t *ui.Theme, offer models.Offer, first bool) {
	p := pal(t)
	row := ui.Row(c).FillWidth().MinHeight(58).Padding(10, 16).Gap(14).AlignItems(ui.Center)
	if !first {
		row.BorderWidth(1, 0, 0, 0).BorderColor(p.Line)
	}
	detail := a.modelDetail(offer)
	row.Children(func() {
		ui.Column(c).Size(32, 32).Radius(kit.RadiusControl).Center().Background(p.Hover).Shrink(0).Children(func() {
			ui.Icon(c, iconWaveform).FontSize(16).TextColor(p.Ink2)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, a.tr(offer.Name)).FontSize(13).FontWeight(500).TextColor(p.Ink).SingleLine()
			mono(ui.Text(c, detail)).FontSize(11).TextColor(p.Ink3).SingleLine().Tooltip(detail)
		})
		switch {
		case offer.Installed && offer.Path == a.modelInUse:
			ui.Row(c).Height(22).Padding(0, 8).Gap(4).Radius(9999).AlignItems(ui.Center).Background(p.GreenSoft).Children(func() {
				ui.Icon(c, iconCheck).FontSize(12).TextColor(p.Green)
				ui.Text(c, a.tr("In use")).FontSize(11.5).FontWeight(500).TextColor(p.Green)
			})
		case offer.Installed:
			if kit.Button(c, t, kit.Secondary, nil, a.tr("Use this model"), false).Clicked() {
				a.useModel(offer)
			}
		case a.modelDLID == offer.ID:
			meter(c, t, float32(a.modelDLProgress), 150)
			mono(ui.Text(c, a.trf("%d%%", int(a.modelDLProgress*100)))).FontSize(11.5).
				TextColor(p.Ink2).Width(36).TextAlign(ui.End)
		case a.modelDLErrID == offer.ID:
			kit.Chip(c, t, kit.ChipBad, a.tr("Download failed")).Tooltip(a.modelDLErr)
			if kit.Button(c, t, kit.Secondary, nil, a.tr("Try again"), false).Clicked() {
				a.downloadModel(offer)
			}
		default:
			button := kit.Button(c, t, kit.Secondary, iconDownload, a.tr("Download"), false)
			button.Disabled(a.modelDLID != "")
			if button.Clicked() && a.modelDLID == "" {
				a.downloadModel(offer)
			}
		}
	})
}

// modelDetail is the line under a model's name: what it hears, how big it is,
// and what a running download has fetched so far.
func (a *App) modelDetail(offer models.Offer) string {
	size := offer.SizeBytes
	if !offer.Installed {
		size = offer.DownloadSize
	}
	parts := []string{a.tr(offer.Detail), fmtSize(size)}
	if a.modelDLID == offer.ID {
		parts[1] = a.trf("Downloading %s of %s", fmtSize(int64(a.modelDLProgress*float64(size))), fmtSize(size))
	}
	if a.modelDLErrID == offer.ID && a.modelDLErr != "" {
		parts = append(parts, a.modelDLErr)
	}
	return strings.Join(parts, " · ")
}

// guideToModels opens the download page when nothing can transcribe yet: on a
// first launch, that is the one thing the window can ask for.
func (a *App) guideToModels() {
	if a.MM == nil {
		return
	}
	go func() {
		if mdl, _ := a.MM.FindASRModel(); mdl != nil {
			return
		}
		a.update(func() {
			a.settingsTab = 1
			a.openSettings()
		})
	}()
}

// loadModels reads the models Ottr supports and which one it would use.
func (a *App) loadModels() {
	a.modelLoaded, a.modelLoading = true, true
	if a.MM == nil {
		return
	}
	go func() {
		list := a.MM.Available()
		mdl, _ := a.MM.FindASRModel() // nil when none is usable
		inUse := ""
		if mdl != nil {
			inUse = mdl.Dir
		}
		a.update(func() {
			a.modelList, a.modelInUse, a.modelLoading = list, inUse, false
		})
	}()
}

// downloadModel fetches one model in the background, showing how far it has
// come and using it when it is the first one installed.
func (a *App) downloadModel(offer models.Offer) {
	if a.MM == nil || a.modelDLID != "" {
		return
	}
	mm, id := a.MM, offer.ID
	inUse := a.modelInUse
	a.modelDLID, a.modelDLProgress, a.modelDLErrID, a.modelDLErr = id, 0, "", ""
	go func() {
		last := -1
		err := mm.Download(context.Background(), id, func(p float64) {
			if pct := int(p * 100); pct != last {
				last = pct
				a.update(func() { a.modelDLProgress = p })
			}
		})
		if err == nil && inUse == "" {
			_ = mm.Select(filepath.Join(mm.ModelsDir(), id))
		}
		a.update(func() {
			a.modelDLID = ""
			if err != nil {
				a.modelDLErrID, a.modelDLErr = id, err.Error()
			}
			a.loadModels()
		})
	}()
}

// useModel makes transcription use this model from now on.
func (a *App) useModel(offer models.Offer) {
	go func() {
		err := a.MM.Select(offer.Path)
		a.update(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.modelLoaded = false // read the list again with the choice applied
		})
	}()
}

func fmtSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	size, exp := float64(bytes), 0
	for size >= unit && exp < 3 {
		size /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", size, "KMGT"[exp-1])
}

// ---------------------------------------------------------------- provider

// providerSettings is the one AI endpoint translation and summaries share, as
// the Provider page edits it.
type providerSettings struct {
	BaseURL, APIKey, Model         string
	TranslatePrompt, SummaryPrompt string
}

// providerPresets fill the base URL in with one click; anything else is Custom.
var providerPresets = []struct {
	name, host, baseURL string
	icon                *ui.SVG
}{
	{"DeepSeek", "api.deepseek.com", "https://api.deepseek.com/v1", iconDeepSeek},
	{"OpenRouter", "openrouter.ai", "https://openrouter.ai/api/v1", iconOpenRouter},
	{"Ollama", "localhost:11434", "http://localhost:11434/v1", iconOllama},
}

// providerSettings draws the endpoint translation and summaries share, the
// import chain, and the prompt templates.
func (a *App) providerSettings(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if a.Trans == nil || a.Sum == nil {
		ui.Text(c, a.tr("No AI provider is configured.")).FontSize(13).TextColor(p.Ink3)
		return
	}
	if !a.loadedProvider {
		a.loadProvider()
		ui.Text(c, a.tr("Loading…")).FontSize(12.5).TextColor(p.Ink3)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		for _, preset := range providerPresets {
			a.providerCard(c, t, preset.name, preset.host, preset.baseURL, preset.icon)
		}
		a.providerCard(c, t, a.tr("Custom"), a.tr("OpenAI-compatible"), "", iconProvider)
	})
	kit.Group(c, t, func() {
		a.fieldRow(c, t, true, a.tr("API base URL"), func() {
			mono(kit.Field(c, t, &a.provider.BaseURL, a.tr("API base URL"), nil).
				Placeholder("https://api.deepseek.com/v1")).FontSize(12)
		})
		a.fieldRow(c, t, false, a.tr("API key"), func() {
			mono(kit.Field(c, t, &a.provider.APIKey, a.tr("API key"), iconKey).Password()).FontSize(12)
		})
		a.fieldRow(c, t, false, a.tr("Model"), func() {
			mono(kit.Field(c, t, &a.provider.Model, a.tr("Model"), nil).Placeholder("deepseek-chat")).FontSize(12)
		})
	})
	kit.Group(c, t, func() {
		kit.GroupRow(c, t, true, a.tr("Translate + summarize imports"),
			a.tr("Imported files are translated into the tab's language and summarized too, as one job. Off by default: it spends the provider's quota."),
			func() { kit.Switch(c, t, &a.importChain, a.tr("Translate and summarize imports")) })
		kit.GroupRow(c, t, false, a.tr("Prompt templates"), a.tr("Summary · Translation · Variables: {transcript}, {language}"), func() {
			label := a.tr("Edit")
			if a.promptsOpen {
				label = a.tr("Done")
			}
			if kit.Button(c, t, kit.Secondary, nil, label, false).Clicked() {
				a.promptsOpen = !a.promptsOpen
			}
		})
		if a.promptsOpen {
			ui.Column(c).FillWidth().Padding(0, 16, 16, 16).Gap(12).Children(func() {
				a.promptCard(c, t, "Translate subtitles", &a.provider.TranslatePrompt,
					translate.DefaultPrompt, "{source} · {target}")
				a.promptCard(c, t, "Summarize transcript", &a.provider.SummaryPrompt,
					summary.DefaultPrompt, "{transcript} · {language}")
			})
		}
	})
	a.saveRow(c, t, a.saveProvider)
}

// fieldRow is a label and a field, as one row of a group.
func (a *App) fieldRow(c *ui.Context, t *ui.Theme, first bool, label string, field func()) {
	p := pal(t)
	row := ui.Row(c).FillWidth().Padding(10, 16).Gap(12).AlignItems(ui.Center)
	if !first {
		row.BorderWidth(1, 0, 0, 0).BorderColor(p.Line)
	}
	row.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.Ink).Width(120).Shrink(0)
		field()
	})
}

// providerCard is one endpoint to pick: a radio tile with its name and the
// host it talks to.
func (a *App) providerCard(c *ui.Context, t *ui.Theme, label, host, baseURL string, icon *ui.SVG) {
	p := pal(t)
	chosen := a.providerMatches(baseURL)
	card := ui.ButtonBase(c.Key(label)).Label(label).Role(ui.RoleRadio).Checked(chosen).Grow(1).Width(0).
		MinWidth(0).Padding(12).Gap(10).Radius(kit.RadiusCard).AlignItems(ui.Start).Cursor(ui.CursorPointer).
		Background(p.Sheet)
	if chosen {
		card.Border(1, p.Ink).InsetShadow(0, 0, 0, 1, p.Ink)
	} else {
		card.Border(1, p.Line2)
		if card.Hovered() {
			card.Background(p.Sunk)
		}
	}
	card.Children(func() {
		radio(c, t, chosen)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(14).TextColor(p.Ink).Shrink(0)
				}
				ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.Ink).SingleLine()
			})
			ui.Text(c, host).FontSize(11.5).TextColor(p.Ink3).SingleLine()
		})
	})
	// A click chooses the endpoint: a preset fills its URL in, Custom clears
	// the field so the user types their own. An endpoint that matches no
	// preset already shows Custom as chosen, and clicking it keeps the URL.
	if card.Clicked() && !chosen {
		a.provider.BaseURL = baseURL
	}
}

// providerMatches says whether the endpoint is this card's; the empty URL is
// the Custom card.
func (a *App) providerMatches(baseURL string) bool {
	chosen := strings.TrimRight(a.provider.BaseURL, "/")
	if baseURL == "" {
		for _, preset := range providerPresets {
			if chosen == strings.TrimRight(preset.baseURL, "/") {
				return false
			}
		}
		return true
	}
	return chosen == strings.TrimRight(baseURL, "/")
}

// promptCard is one editable instruction, with the way back to the built-in
// text and the variables it fills in.
func (a *App) promptCard(c *ui.Context, t *ui.Theme, title string, value *string, fallback, variables string) {
	p := pal(t)
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr(title)).FontSize(12.5).FontWeight(500).TextColor(p.Ink).Grow(1)
			if kit.Button(c, t, kit.Ghost, nil, a.tr("Reset to default"), false).Clicked() {
				*value = fallback
			}
		})
		mono(ui.TextArea(c, value).Height(72).Label(a.tr(title))).FontSize(12)
		mono(ui.Text(c, a.trf("Variables: %s", variables))).FontSize(10.5).TextColor(p.Ink3)
	})
}

// loadProvider reads both services' settings into the one page.
func (a *App) loadProvider() {
	a.loadedProvider = true
	go func() {
		tr, sum := a.Trans.GetSettings(), a.Sum.GetSettings()
		p := providerSettings{
			BaseURL: tr.BaseURL, APIKey: tr.APIKey, Model: tr.Model,
			TranslatePrompt: tr.Prompt, SummaryPrompt: sum.Prompt,
		}
		// The summarizer keeps a usable default endpoint, so it wins when
		// translation has none.
		if sum.BaseURL != "" {
			p.BaseURL, p.APIKey, p.Model = sum.BaseURL, sum.APIKey, sum.Model
		}
		a.update(func() { a.provider = p })
	}()
}

// saveProvider writes the one endpoint into both services.
func (a *App) saveProvider() {
	p := a.provider
	go func() {
		tr := a.Trans.GetSettings()
		tr.BaseURL, tr.APIKey, tr.Model, tr.Prompt = p.BaseURL, p.APIKey, p.Model, p.TranslatePrompt
		sum := a.Sum.GetSettings()
		sum.BaseURL, sum.APIKey, sum.Model, sum.Prompt = p.BaseURL, p.APIKey, p.Model, p.SummaryPrompt
		err := a.Trans.SaveSettings(tr)
		if err == nil {
			err = a.Sum.SaveSettings(sum)
		}
		a.update(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.loadedTrans, a.loadedProvider = false, false
		})
	}()
}

// ---------------------------------------------------------------- speakers

// speakersSettings is the library of voices heard across recordings.
func (a *App) speakersSettings(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if a.ASR == nil {
		ui.Text(c, a.tr("No speaker library is configured.")).FontSize(13).TextColor(p.Ink3)
		return
	}
	if !a.loadedSpeakers {
		a.loadSpeakers()
		ui.Text(c, a.tr("Loading…")).FontSize(12.5).TextColor(p.Ink3)
		return
	}
	if len(a.speakerLib) == 0 {
		kit.Group(c, t, func() {
			kit.GroupRow(c, t, true, a.tr("Speaker library"),
				a.tr("No speaker has been heard yet. Transcribe a recording with voices in it."), nil)
		})
		return
	}
	shown := 0
	kit.Group(c, t, func() {
		for _, sp := range a.speakerLib {
			if !strings.Contains(strings.ToLower(sp.Name), strings.ToLower(strings.TrimSpace(a.speakerQuery))) {
				continue
			}
			a.speakerLibraryRow(c.Key(sp.ID), t, sp, shown == 0)
			shown++
		}
		if shown == 0 {
			ui.Text(c, a.tr("Nothing matches that search.")).FontSize(12.5).TextColor(p.Ink3).Padding(14, 16)
		}
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconCPU).FontSize(14).TextColor(p.Ink3)
		ui.Text(c, a.tr("Voices are matched on this computer. Forgetting a voice keeps its lines as Speaker N.")).
			FontSize(12).TextColor(p.Ink3)
	})
}

// speakerLibraryRow is one speaker: avatar, name, where they were heard, and
// what the library offers: rename (or name an unnamed voice) and forget.
func (a *App) speakerLibraryRow(c *ui.Context, t *ui.Theme, sp asr.Speaker, first bool) {
	p := pal(t)
	row := ui.Row(c).FillWidth().MinHeight(58).Padding(10, 16).Gap(14).AlignItems(ui.Center)
	if !first {
		row.BorderWidth(1, 0, 0, 0).BorderColor(p.Line)
	}
	unnamed := strings.HasPrefix(sp.Name, "Speaker ")
	row.Children(func() {
		kit.Avatar(c, speakerNumber(sp.Name, sp.ID), speakerLibraryInitials(sp.Name), 32)
		if a.speakerEditID == sp.ID {
			input := ui.TextInput(c, &a.speakerEditText).Label(a.tr("Speaker name")).Grow(1).MinWidth(0)
			if input.Submitted() {
				a.saveLibrarySpeakerName(sp.ID)
			}
			if kit.Button(c, t, kit.Primary, nil, a.tr("Save"), false).Clicked() {
				a.saveLibrarySpeakerName(sp.ID)
			}
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sp.Name).FontSize(13).FontWeight(500).TextColor(p.Ink).SingleLine()
				if unnamed {
					kit.Chip(c, t, kit.ChipNeutral, a.tr("Unnamed"))
				}
			})
			ui.Text(c, a.speakerMeta(sp)).FontSize(12).TextColor(p.Ink3).SingleLine()
		})
		var rename ui.Element
		if unnamed {
			rename = kit.Button(c, t, kit.Primary, nil, a.tr("Name this voice"), false)
		} else {
			rename = kit.Button(c, t, kit.Secondary, iconPencil, a.tr("Rename"), false)
		}
		if rename.Tooltip(a.tr("Rename this speaker everywhere")).Clicked() {
			a.speakerEditID, a.speakerEditText = sp.ID, sp.Name
			if unnamed {
				a.speakerEditText = ""
			}
		}
		kit.IconButton(c, t, iconUserX, a.tr("Remove"), func() { a.removeSpeaker(sp.ID) }).
			Tooltip(a.tr("Forget this voice"))
	})
}

// speakerMeta says how often and when a speaker was heard.
func (a *App) speakerMeta(sp asr.Speaker) string {
	if sp.Recordings == 0 {
		return a.tr("Not heard on a recording yet")
	}
	at := ""
	if parsed, err := time.Parse(time.RFC3339, sp.LastHeard); err == nil {
		at = a.date(parsed)
	}
	duration := speakerDuration(sp.DurationMs)
	if sp.Recordings == 1 {
		return a.trf("1 recording · %s · last heard %s", duration, at)
	}
	return a.trf("%d recordings · %s · last heard %s", sp.Recordings, duration, at)
}

// speakerDuration is a length as a person says it: hours and minutes.
func speakerDuration(ms int64) string {
	minutes := int(ms / 60000)
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

// speakerLibraryInitials is the avatar text: the first letters of a name, or
// the number an unnamed speaker goes by.
func speakerLibraryInitials(name string) string {
	if number, ok := strings.CutPrefix(name, "Speaker "); ok {
		return "S" + number
	}
	parts := strings.Fields(name)
	var initials strings.Builder
	for _, part := range parts[:min(2, len(parts))] {
		for _, r := range part {
			initials.WriteString(strings.ToUpper(string(r)))
			break
		}
	}
	return initials.String()
}

// loadSpeakers reads the library again.
func (a *App) loadSpeakers() {
	a.loadedSpeakers = true
	go func() {
		list, err := a.ASR.Speakers()
		a.update(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.speakerLib = list
		})
	}()
}

// saveLibrarySpeakerName renames one speaker everywhere.
func (a *App) saveLibrarySpeakerName(id int) {
	name := strings.TrimSpace(a.speakerEditText)
	if name == "" {
		a.setError(fmt.Errorf("speaker name cannot be empty"))
		return
	}
	go func() {
		if err := a.ASR.RenameSpeaker(id, name); err != nil {
			a.update(func() { a.setError(err) })
			return
		}
		a.update(func() {
			for i := range a.speakerLib {
				if a.speakerLib[i].ID == id {
					a.speakerLib[i].Name = name
				}
			}
			if a.transcript.SpeakerNames != nil {
				a.transcript.SpeakerNames[id] = name
			}
			a.speakerEditID, a.speakerEditText = -1, ""
		})
	}()
}

// removeSpeaker forgets a voice; transcripts keep their lines.
func (a *App) removeSpeaker(id int) {
	go func() {
		if err := a.ASR.RemoveSpeaker(id); err != nil {
			a.update(func() { a.setError(err) })
			return
		}
		a.update(func() { a.loadedSpeakers = false })
	}()
}

// saveRow is the one button that stores what the section above holds.
func (a *App) saveRow(c *ui.Context, t *ui.Theme, save func()) {
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		if kit.Button(c, t, kit.Primary, nil, a.tr("Save"), false).Clicked() {
			save()
		}
	})
}

// ---------------------------------------------------------------- settings

func (a *App) loadTransSettings() {
	go func() {
		cfg := a.Trans.GetSettings()
		a.update(func() { a.transCfg, a.loadedTrans = cfg, true })
	}()
}

func (a *App) saveTransSettings() {
	cfg := a.transCfg
	go func() {
		err := a.Trans.SaveSettings(cfg)
		a.update(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.loadedTrans = false
		})
	}()
}
