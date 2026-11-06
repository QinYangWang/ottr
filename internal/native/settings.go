package native

import (
	"fmt"
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

const (
	settingsWidth  = 920
	settingsHeight = 660
)

// openSettings gives Settings its own native window, leaving the main window
// exactly as it was.
func (a *App) openSettings() {
	if a.settingsWin != nil && !a.settingsWin.IsDestroyed() {
		a.settingsWin.Focus()
		return
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title: "Ottr Settings", Width: settingsWidth, Height: settingsHeight,
		MinWidth: 800, MinHeight: 480, Parent: a.win,
		TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: titleBarHeight,
		BackgroundColor: themeBackground(), Content: ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() { a.settingsWin = nil })
}

func themeBackground() string {
	if mygo.Theme.IsDark() {
		return "#000000"
	}
	return "#ffffff"
}

// settingsSections are the pages Settings offers, in the order they are shown.
var settingsSections = []struct {
	label string
	icon  *ui.SVG
}{
	{"Appearance", iconSliders},
	{"Transcription", iconMic},
	{"Provider", iconProvider},
	{"Speakers", iconSpeakers},
}

// settingsView draws sections on the left and the chosen one on the right.
func (a *App) settingsView(c *ui.Context) {
	t := a.applyTheme(c)
	a.drain()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.windowTitleBar(c, t, "Ottr", nil)
		ui.Row(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Width(200).Shrink(0).FillHeight().Padding(t.Space(3)).
				Gap(t.Space(1)).Background(t.Background).BorderWidth(0, 1, 0, 0).
				BorderColor(t.Border).Children(func() {
				ui.Text(c, a.tr("Settings")).FontSize(t.FontSize+1).Bold().TextColor(t.TextMuted).
					Padding(t.Space(1), t.Space(2), t.Space(2), t.Space(2))
				for i, item := range settingsSections {
					a.settingsNav(c, t, i, a.tr(item.label), item.icon)
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
				ui.Row(c).Padding(t.Space(5), t.Space(5), t.Space(2), t.Space(5)).Children(func() {
					ui.Text(c, a.tr(settingsSections[a.settingsTab].label)).
						FontSize(t.FontSize + 5).Bold()
				})
				a.settingsPage(c, t)
			})
		})
	})
	if a.err != "" {
		a.banner(c, t)
	}
}

func (a *App) settingsNav(c *ui.Context, t *ui.Theme, tab int, label string, icon *ui.SVG) {
	nav := ui.ButtonBase(c).Label(label).FillWidth().Padding(t.Space(2), t.Space(2.5)).
		Radius(t.Space(2)).Cursor(ui.CursorPointer)
	if a.settingsTab == tab {
		nav.Background(t.Surface)
		cardShadow(nav, t)
	} else {
		nav.TextColor(t.TextMuted)
	}
	nav.Children(func() {
		ui.Row(c).Grow(1).Gap(t.Space(2.5)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).FontSize(t.FontSize + 2)
			ui.Text(c, label).FontSize(t.FontSize - 1)
		})
	})
	if a.settingsTab != tab && nav.Hovered() {
		nav.Background(t.SurfaceHover)
	}
	if nav.Clicked() {
		a.settingsTab = tab
	}
}

// settingsPage is the scrollable half chosen on the window's left.
func (a *App) settingsPage(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(t.Space(2), t.Space(5), t.Space(5), t.Space(5)).Gap(t.Space(3)).Children(func() {
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

// appearanceSettings picks the look: the desktop's, or a fixed one.
func (a *App) appearanceSettings(c *ui.Context, t *ui.Theme) {
	a.settingRow(c, t, "Language", "Choose the language of the interface.", func() {
		language := "English (United States)"
		if a.language == "zh" {
			language = "中文（中国）"
		}
		kit.SelectBox(c, t, &language, []kit.SelectOption[string]{
			{Value: "English (United States)", Label: "English (United States)"},
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
	a.settingRow(c, t, "Display font", "Use the system font, or pick an installed family.", func() {
		defaultFont := a.tr("System default")
		font := a.font
		if font == "" {
			font = defaultFont
		}
		options := []string{defaultFont, "Inter", "Noto Sans", "Noto Sans SC", "Segoe UI", "Georgia", "JetBrains Mono"}
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
	ui.Text(c, a.tr("Match the desktop, or pick a fixed look.")).
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)
	ui.Row(c).Gap(t.Space(3)).Children(func() {
		for _, choice := range []struct{ source, label string }{
			{"system", a.tr("System")},
			{"light", a.tr("Light")},
			{"dark", a.tr("Dark")},
		} {
			a.themeCard(c, t, choice.source, choice.label)
		}
		ui.Spacer(c)
	})
	ui.Text(c, a.tr("The window redraws at once, and remembers the choice.")).
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)
}

// themeCard is one look to pick, drawn as a small window in it.
func (a *App) themeCard(c *ui.Context, t *ui.Theme, source, label string) {
	chosen := a.themeSource() == source
	card := ui.ButtonBase(c).Label(label).Width(170).Shrink(0).Height(112).
		Padding(t.Space(2)).Radius(t.Radius).BorderWidth(1).Cursor(ui.CursorPointer)
	if chosen {
		card.BorderColor(t.Accent).Background(t.Surface)
	} else {
		card.BorderColor(t.Border)
	}
	card.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(2)).Children(func() {
			box := ui.Row(c).Height(66).Radius(6).Clip().BorderWidth(1).BorderColor(t.Border)
			box.Children(func() {
				ui.Row(c).Fill().Children(func() {
					switch source {
					case "system":
						a.themeMock(c, ui.RGB(0xdf, 0xe4, 0xeb), ui.RGB(0xff, 0xff, 0xff), ui.RGB(0x8d, 0x95, 0xa1))
						a.themeMock(c, ui.RGB(0x1b, 0x1c, 0x20), ui.RGB(0x33, 0x35, 0x3b), ui.RGB(0xa8, 0xae, 0xb8))
					case "dark":
						a.themeMock(c, ui.RGB(0x19, 0x1a, 0x1d), ui.RGB(0x26, 0x27, 0x2b), ui.RGB(0x8f, 0x94, 0x9d))
					default:
						a.themeMock(c, ui.RGB(0xee, 0xf0, 0xf3), ui.RGB(0xff, 0xff, 0xff), ui.RGB(0x9a, 0xa0, 0xaa))
					}
				})
			})
			ui.Text(c, label).FontSize(t.FontSize - 2).Bold().SingleLine().TextAlign(ui.Center)
		})
	})
	if !chosen && card.Hovered() {
		card.Background(t.SurfaceHover)
	}
	if card.Clicked() && !chosen {
		a.setTheme(source)
	}
}

// themeMock draws a miniature window — a sidebar, a title line and two lines —
// in the colors of one look.
func (a *App) themeMock(c *ui.Context, canvas, side, ink ui.Color) {
	ui.Row(c).Grow(1).Fill().AlignItems(ui.Stretch).Background(canvas).Children(func() {
		ui.Box(c).Width(18).Shrink(0).FillHeight().Background(side)
		ui.Column(c).Grow(1).Padding(8, 8).Gap(5).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Height(5).Width(34).Radius(3).Background(ink)
			ui.Row(c).Height(4).Width(46).Radius(2).Background(ink).Opacity(0.55)
			ui.Row(c).Height(4).Width(30).Radius(2).Background(ink).Opacity(0.55)
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
	for _, win := range []*mygo.Window{a.settingsWin} {
		if win != nil {
			win.SetBackgroundColor(themeBackground())
		}
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
	if a.MM == nil {
		ui.Text(c, a.tr("No model manager is configured.")).TextColor(t.TextMuted)
		return
	}
	if !a.modelLoaded {
		a.loadModels()
	}
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, a.tr("Speech recognition model")).Bold()
			ui.Text(c, a.tr("Transcription runs offline, with the model chosen here.")).
				FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		})
		if ui.Button(c, a.tr("Open folder")).Tooltip(a.tr("Open the folder models are installed in")).Clicked() {
			mygo.Shell.OpenPath(a.MM.ModelsDir())
		}
		if ui.Button(c, a.tr("Re-check")).Tooltip(a.tr("Look for models again")).Clicked() {
			a.loadModels()
		}
	})
	if a.modelLoading {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Label(a.tr("Looking for models"))
			ui.Text(c, a.tr("Looking for models…")).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		})
	} else if len(a.modelList) == 0 {
		ui.Text(c, a.trf("No models in %s yet.", a.MM.ModelsDir())).
			TextColor(t.TextMuted).Selectable()
		ui.Text(c, a.tr("Install SenseVoice, FunASR Nano or Qwen3 ASR. Other model families are not supported yet.")).
			FontSize(t.FontSize - 2).TextColor(t.TextMuted)
	}
	for _, info := range a.modelList {
		a.modelRow(c, t, info)
	}
}

func (a *App) modelRow(c *ui.Context, t *ui.Theme, info models.Info) {
	a.settingRow(c, t, info.Name, modelDetail(info), func() {
		if info.Path == a.modelInUse {
			ui.Badge(c, a.tr("In use"))
			return
		}
		if info.Installed && ui.Button(c, a.tr("Use")).Label(a.tr("Use this model")).Clicked() {
			a.useModel(info)
		}
	})
}

func modelDetail(info models.Info) string {
	if info.Error != "" {
		return info.Error
	}
	return fmtSize(info.SizeBytes)
}

// loadModels reads the models on disk, and which one transcription would use.
func (a *App) loadModels() {
	a.modelLoaded, a.modelLoading = true, true
	if a.MM == nil {
		return
	}
	go func() {
		list, err := a.MM.ListModels()
		mdl, _ := a.MM.FindASRModel() // nil when none is usable
		inUse := ""
		if mdl != nil {
			inUse = mdl.Dir
		}
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.modelList, a.modelInUse, a.modelLoading = list, inUse, false
		})
	}()
}

// useModel makes transcription use this model from now on.
func (a *App) useModel(info models.Info) {
	go func() {
		err := a.MM.Select(info.Path)
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

// providerSettings draws the credentials and prompt templates that translation
// and summaries share.
func (a *App) providerSettings(c *ui.Context, t *ui.Theme) {
	if a.Trans == nil || a.Sum == nil {
		ui.Text(c, a.tr("No AI provider is configured.")).TextColor(t.TextMuted)
		return
	}
	if !a.loadedProvider {
		a.loadProvider()
		ui.Text(c, a.tr("Loading…")).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		return
	}
	ui.Text(c, a.tr("Provider")).Bold()
	ui.Text(c, a.tr("Translation and summaries share one OpenAI-compatible endpoint.")).
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)
	ui.Row(c).Gap(t.Space(2)).Children(func() {
		for _, preset := range providerPresets {
			a.providerCard(c, t, preset.name, preset.host, preset.baseURL, preset.icon)
		}
		a.providerCard(c, t, a.tr("Custom"), a.tr("OpenAI-compatible"), "", iconProvider)
		ui.Spacer(c)
	})
	a.settingRow(c, t, "API base URL", "Any OpenAI-compatible chat endpoint.", func() {
		ui.TextInput(c, &a.provider.BaseURL).Placeholder("https://api.deepseek.com/v1").Grow(1).Label(a.tr("API base URL"))
	})
	a.settingRow(c, t, "API key", "Kept in the database on this computer.", func() {
		ui.TextInput(c, &a.provider.APIKey).Password().Grow(1).Label(a.tr("API key"))
	})
	a.settingRow(c, t, "Model", "Used for translation and summaries.", func() {
		ui.TextInput(c, &a.provider.Model).Placeholder("deepseek-chat").Grow(1).Label(a.tr("Model"))
	})
	a.settingRow(c, t, "Translate + summarize imports",
		"Imported files are translated into the tab's language and summarized too, as one job. Off by default: it spends the provider's quota.",
		func() { ui.Switch(c, &a.importChain).Label(a.tr("Translate and summarize imports")) })
	ui.Text(c, a.tr("Prompt templates")).Bold()
	a.promptCard(c, t, "Translate subtitles", &a.provider.TranslatePrompt,
		translate.DefaultPrompt, "{source} · {target}")
	a.promptCard(c, t, "Summarize transcript", &a.provider.SummaryPrompt,
		summary.DefaultPrompt, "{transcript} · {language}")
	a.saveRow(c, t, a.saveProvider)
}

// providerCard is one endpoint to pick: its name and the host it talks to.
func (a *App) providerCard(c *ui.Context, t *ui.Theme, label, host, baseURL string, icon *ui.SVG) {
	chosen := a.providerMatches(baseURL)
	card := ui.ButtonBase(c).Label(label).Width(158).Shrink(0).Height(56).
		Padding(t.Space(1.5), t.Space(2)).Radius(t.Space(1)).BorderWidth(1).
		Cursor(ui.CursorPointer).AlignItems(ui.Start)
	if chosen {
		card.BorderColor(t.Accent).Background(t.Surface)
	} else {
		card.BorderColor(t.Border)
	}
	card.Children(func() {
		ui.Column(c).Fill().MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(t.FontSize + 2).TextColor(t.Text).Shrink(0)
				}
				ui.Text(c, label).FontSize(t.FontSize - 2).Bold().SingleLine()
				ui.Spacer(c)
				if chosen {
					ui.Icon(c, iconCheck).FontSize(t.FontSize).TextColor(t.Accent)
				}
			})
			ui.Text(c, host).FontSize(t.FontSize - 4).TextColor(t.TextMuted).SingleLine()
		})
	})
	if !chosen && card.Hovered() {
		card.Background(t.SurfaceHover)
	}
	// Custom is the state of an endpoint that matches no preset, so it is
	// shown rather than clicked.
	if card.Clicked() && baseURL != "" && !chosen {
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
	ui.Column(c).Gap(t.Space(1.5)).Padding(t.Space(2), t.Space(2.5)).
		BorderWidth(1).BorderColor(t.Border).Radius(t.Space(1)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr(title)).FontSize(t.FontSize - 1).Bold().Grow(1)
			if ui.Button(c, a.tr("Reset to default")).Clicked() {
				*value = fallback
			}
		})
		ui.TextArea(c, value).Height(64).Label(a.tr(title))
		ui.Text(c, a.trf("Variables: %s", variables)).
			FontSize(t.FontSize - 4).TextColor(t.TextMuted)
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
	if a.ASR == nil {
		ui.Text(c, a.tr("No speaker library is configured.")).TextColor(t.TextMuted)
		return
	}
	if !a.loadedSpeakers {
		a.loadSpeakers()
		ui.Text(c, a.tr("Loading…")).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		return
	}
	a.settingRow(c, t, "Speaker library",
		"Voices are matched across recordings. Renaming a speaker renames them in every transcript.",
		func() { ui.Badge(c, a.trf("%d speakers", len(a.speakerLib))) })
	if len(a.speakerLib) == 0 {
		ui.Text(c, a.tr("No speaker has been heard yet. Transcribe a recording with voices in it.")).
			FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		return
	}
	ui.SearchField(c, &a.speakerQuery).Label(a.tr("Search speakers")).FillWidth()
	shown := 0
	for _, sp := range a.speakerLib {
		if !strings.Contains(strings.ToLower(sp.Name), strings.ToLower(strings.TrimSpace(a.speakerQuery))) {
			continue
		}
		shown++
		a.speakerLibraryRow(c, t, sp)
	}
	if shown == 0 {
		ui.Text(c, a.tr("Nothing matches that search.")).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
	}
	ui.Text(c, a.tr("Voices are matched locally with an embedding model; no audio leaves this computer.")).
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)
}

// speakerLibraryRow is one speaker: avatar, name, where they were heard, and
// the two things the library offers.
func (a *App) speakerLibraryRow(c *ui.Context, t *ui.Theme, sp asr.Speaker) {
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Padding(t.Space(1.5), t.Space(2)).
		BorderWidth(1).BorderColor(t.Border).Radius(t.Space(1)).Children(func() {
		avatar, ink := speakerTint(t, sp.ID)
		ui.Column(c).Size(30, 30).Shrink(0).Radius(15).Center().Background(avatar).Children(func() {
			ui.Text(c, speakerLibraryInitials(sp.Name)).FontSize(t.FontSize - 3).Bold().TextColor(ink)
		})
		if a.speakerEditID == sp.ID {
			input := ui.TextInput(c, &a.speakerEditText).Label(a.tr("Speaker name")).Grow(1).MinWidth(0)
			if input.Submitted() {
				a.saveLibrarySpeakerName(sp.ID)
			}
			if ui.PrimaryButton(c, a.tr("Save")).Clicked() {
				a.saveLibrarySpeakerName(sp.ID)
			}
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, sp.Name).FontSize(t.FontSize - 1).Bold().SingleLine()
			ui.Text(c, a.speakerMeta(sp)).FontSize(t.FontSize - 3).TextColor(t.TextMuted).SingleLine()
		})
		if ui.Button(c, a.tr("Rename")).Tooltip(a.tr("Rename this speaker everywhere")).Clicked() {
			a.speakerEditID, a.speakerEditText = sp.ID, sp.Name
		}
		if ui.Button(c, a.tr("Remove")).Tooltip(a.tr("Forget this voice")).Clicked() {
			a.removeSpeaker(sp.ID)
		}
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
	ui.Row(c).Justify(ui.End).Children(func() {
		if ui.PrimaryButton(c, a.tr("Save")).Clicked() {
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
