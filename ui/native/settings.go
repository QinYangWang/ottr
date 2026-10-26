package native

import (
	"context"
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/models"
	"ottr/internal/settings"
	"ottr/internal/tools"
)

// settingsPage holds what the app can be told: how it looks, which model
// recognizes speech on this computer, and which provider translates and
// summarizes.
func (a *App) settingsPage(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(4), t.Space(5)).Gap(t.Space(2)).AlignItems(ui.Center).
			Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "Settings").FontSize(t.FontSize + 4).Bold()
				ui.Text(c, "Speech recognition runs here; translation and summaries use a remote model.").
					FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			})
			if ui.Button(c, "Close").Clicked() {
				a.closePane()
			}
		})
		ui.Row(c).Padding(t.Space(3.5), t.Space(5), 0, t.Space(5)).Children(func() {
			ui.Tabs(c, &a.settingsTab, "Appearance", "Transcription", "Translation", "Summary")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(t.Space(4), t.Space(5)).Gap(t.Space(3)).Children(func() {
				switch a.settingsTab {
				case 1:
					a.transcriptionSettings(c, t)
				case 2:
					a.translationSettings(c, t)
				case 3:
					a.summarySettings(c, t)
				default:
					a.appearanceSettings(c, t)
				}
			})
		})
	})
}

// --------------------------------------------------------------- appearance

// appearanceSettings picks the look: the desktop's, or a fixed one.
func (a *App) appearanceSettings(c *ui.Context, t *ui.Theme) {
	ui.Text(c, "Appearance").Bold()
	ui.Text(c, "Match the desktop, or pick a fixed look.").
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)
	ui.Row(c).Gap(t.Space(3)).Children(func() {
		for _, choice := range []struct{ source, label string }{
			{"system", "System"},
			{"light", "Light"},
			{"dark", "Dark"},
		} {
			a.themeCard(c, t, choice.source, choice.label)
		}
		ui.Spacer(c)
	})
	ui.Text(c, "The window redraws at once, and remembers the choice.").
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
		background := "#101014"
		if !mygo.Theme.IsDark() {
			background = "#f6f6f8"
		}
		a.win.SetBackgroundColor(background)
	}
	if a.Prefs != nil {
		prefs := a.Prefs
		go prefs.Set(settings.Theme, source)
	}
}


// settingRow is one setting: what it is, what it does, and its control on the
// right.
func settingRow(c *ui.Context, t *ui.Theme, title, detail string, control func()) {
	ui.Row(c).Gap(t.Space(3)).Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius).
		BorderWidth(1).BorderColor(t.Border).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(t.FontSize - 1).Bold().SingleLine()
			if detail != "" {
				ui.Text(c, detail).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
			}
		})
		control()
	})
}

// ------------------------------------------------------------ transcription

func (a *App) transcriptionSettings(c *ui.Context, t *ui.Theme) {
	if a.MM == nil {
		ui.Text(c, "No model manager is configured.").TextColor(t.TextMuted)
		return
	}
	if !a.modelLoaded {
		a.loadModels()
	}
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "Speech recognition model").Bold()
			ui.Text(c, "Transcription runs offline, with the model chosen here.").
				FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		})
		if ui.Button(c, "Open folder").Tooltip("Open the folder models are installed in").Clicked() {
			mygo.Shell.OpenPath(a.MM.ModelsDir())
		}
		if ui.Button(c, "Re-check").Tooltip("Look for models again").Clicked() {
			a.loadModels()
		}
	})
	if a.modelLoading {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Label("Looking for models")
			ui.Text(c, "Looking for models…").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		})
	} else if len(a.modelList) == 0 {
		ui.Text(c, fmt.Sprintf("No models in %s yet.", a.MM.ModelsDir())).
			TextColor(t.TextMuted).Selectable()
		ui.Text(c, "Install a streaming zipformer model there — tokens.txt with encoder, decoder and joiner ONNX files — then Re-check.").
			FontSize(t.FontSize - 2).TextColor(t.TextMuted)
	}
	for _, info := range a.modelList {
		a.modelRow(c, t, info)
	}
	ui.Column(c).Gap(1).Children(func() {
		ui.Text(c, "Recognizer binary").FontSize(t.FontSize - 1).Bold()
		ui.Text(c, a.sherpaNote()).FontSize(t.FontSize - 3).TextColor(t.TextMuted).Selectable()
	})
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "FFmpeg and ffprobe").FontSize(t.FontSize - 1).Bold()
			ui.Text(c, a.ffmpegNote).FontSize(t.FontSize - 3).TextColor(t.TextMuted).Selectable()
		})
		if ui.Button(c, "Choose…").Tooltip("Point Ottr at ffmpeg.exe").Clicked() {
			a.pickFFmpeg()
		}
		if ui.Button(c, "Re-check").Clicked() {
			a.loadModels()
		}
	})
}

func (a *App) modelRow(c *ui.Context, t *ui.Theme, info models.Info) {
	settingRow(c, t, info.Name, modelDetail(info), func() {
		if info.Path == a.modelInUse {
			ui.Badge(c, "In use")
			return
		}
		if info.Installed && ui.Button(c, "Use").Label("Use this model").Clicked() {
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

// sherpaNote says where the recognizer binary was found, or why it was not.
func (a *App) sherpaNote() string {
	if a.modelBinErr != "" {
		return a.modelBinErr
	}
	if a.modelBin == "" {
		return "Looking for the binary…"
	}
	return "Found at " + a.modelBin
}

// loadModels reads the models on disk, and which one transcription would use.
func (a *App) loadModels() {
	a.modelLoaded, a.modelLoading = true, true
	if a.MM == nil {
		return
	}
	go func() {
		list, err := a.MM.ListModels()
		bin, binErr := a.MM.FindSherpaBin()
		mdl, _ := a.MM.FindASRModel() // nil when none is usable
		inUse := ""
		if mdl != nil {
			inUse = mdl.Dir
		}
		ffmpeg, ffmpegErr := tools.FFmpeg()
		_, probeErr := tools.FFprobe()
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.modelList, a.modelInUse, a.modelLoading = list, inUse, false
			a.modelBin, a.modelBinErr = bin, ""
			if binErr != nil {
				a.modelBinErr = binErr.Error()
			}
			a.ffmpeg, a.ffmpegNote = ffmpeg, "Found at "+ffmpeg
			switch {
			case ffmpegErr != nil:
				a.ffmpeg, a.ffmpegNote = "", ffmpegErr.Error()
			case probeErr != nil:
				a.ffmpegNote = "Found at " + ffmpeg + ", but ffprobe is missing"
			}
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

// pickFFmpeg lets the user point at the program when Ottr cannot find it — an
// FFmpeg installed while the app runs is not on its PATH.
func (a *App) pickFFmpeg() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: mygo.CallerWindow(context.Background()),
			Title:  "Choose ffmpeg.exe",
			Filters: []mygo.FileFilter{
				{Name: "ffmpeg", Extensions: []string{"exe"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.update(func() { a.useFFmpeg(paths[0]) })
	}()
}

// useFFmpeg points the app at a program or a folder, and remembers it.
func (a *App) useFFmpeg(path string) {
	tools.SetChosen(path)
	if a.Prefs != nil {
		prefs := a.Prefs
		go prefs.Set(settings.FFmpegPath, path)
	}
	a.loadModels() // reads the programs again, so the page shows the result
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

// --------------------------------------------------------------- translation

func (a *App) translationSettings(c *ui.Context, t *ui.Theme) {
	if !a.loadedTrans {
		a.loadTransSettings()
		ui.Text(c, "Loading…").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		return
	}
	ui.Text(c, "Translation provider").Bold()
	ui.Text(c, "Subtitles are translated online, one target language at a time.").
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)

	providers := []string{"Free online", "Custom AI model"}
	chosen := 0
	if a.transCfg.Provider == "custom" {
		chosen = 1
	}
	ui.Row(c).Children(func() {
		ui.Segmented(c, &chosen, providers...).Label("Translation provider").Width(300).Changed()
		ui.Spacer(c)
	})
	if chosen == 1 {
		a.transCfg.Provider = "custom"
	} else {
		a.transCfg.Provider = "mymemory"
	}
	if a.transCfg.Provider == "custom" {
		settingRow(c, t, "API base URL", "Any OpenAI-compatible chat endpoint.", func() {
			ui.TextInput(c, &a.transCfg.BaseURL).Placeholder("https://api.deepseek.com/v1").Grow(1).Label("API base URL")
		})
		settingRow(c, t, "API key", "Kept in the database on this computer.", func() {
			ui.TextInput(c, &a.transCfg.APIKey).Password().Grow(1).Label("API key")
		})
		settingRow(c, t, "Model", "For example deepseek-chat.", func() {
			ui.TextInput(c, &a.transCfg.Model).Placeholder("deepseek-chat").Grow(1).Label("Model")
		})
	}
	a.saveRow(c, t, func() { a.saveTransSettings() })
}

// --------------------------------------------------------------- summary

func (a *App) summarySettings(c *ui.Context, t *ui.Theme) {
	if !a.loadedSum {
		a.loadSumSettings()
		ui.Text(c, "Loading…").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		return
	}
	ui.Text(c, "AI model").Bold()
	ui.Text(c, "Summaries are written by a remote OpenAI-compatible model.").
		FontSize(t.FontSize - 3).TextColor(t.TextMuted)
	settingRow(c, t, "API base URL", "DeepSeek, OpenAI, SiliconFlow or a local Ollama.", func() {
		ui.TextInput(c, &a.sumCfg.BaseURL).Placeholder("https://api.deepseek.com/v1").Grow(1).Label("API base URL")
	})
	settingRow(c, t, "API key", "Kept in the database on this computer.", func() {
		ui.TextInput(c, &a.sumCfg.APIKey).Password().Grow(1).Label("API key")
	})
	settingRow(c, t, "Model", "For example deepseek-chat.", func() {
		ui.TextInput(c, &a.sumCfg.Model).Placeholder("deepseek-chat").Grow(1).Label("Model")
	})
	a.saveRow(c, t, func() { a.saveSumSettings() })
}

// saveRow is the one button that stores what the section above holds.
func (a *App) saveRow(c *ui.Context, t *ui.Theme, save func()) {
	ui.Row(c).Justify(ui.End).Children(func() {
		if ui.PrimaryButton(c, "Save").Clicked() {
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

func (a *App) loadSumSettings() {
	go func() {
		cfg := a.Sum.GetSettings()
		a.update(func() { a.sumCfg, a.loadedSum = cfg, true })
	}()
}

func (a *App) saveSumSettings() {
	cfg := a.sumCfg
	go func() {
		err := a.Sum.SaveSettings(cfg)
		a.update(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.loadedSum = false
		})
	}()
}
