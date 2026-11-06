package native

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// recorderPage uses the same structure as sessionPage: title and media on the
// left, transcript on the right, stacked when the window is compact.
func (a *App) recorderPage(c *ui.Context, t *ui.Theme) {
	width, _ := c.Size()
	if width < 900 {
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(t.Space(3), t.Space(4), 0, t.Space(4)).Gap(t.Space(3)).Children(func() {
				a.recordingHead(c, t)
				a.recordingStage(c, t, true)
			})
			ui.Column(c).Grow(1).MinHeight(0).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
				a.recordingTranscript(c, t)
			})
		})
		return
	}
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
			ui.Column(c).Padding(t.Space(4), t.Space(5), t.Space(5), t.Space(5)).Gap(t.Space(4)).Children(func() {
				a.recordingHead(c, t)
				a.recordingStage(c, t, false)
			})
		})
		ui.Column(c).Width(sideWidth).Shrink(0).FillHeight().MinWidth(0).
			Background(t.Surface).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
			a.recordingTranscript(c, t)
		})
	})
}

func (a *App) recordingHead(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(3)).AlignItems(ui.Center).Children(func() {
		dot := ui.Box(c).Size(10, 10).Radius(5).Background(t.Danger)
		dot.Opacity(float32(0.35 + 0.65*dot.Loop("pulse", 1100*time.Millisecond, ui.EaseInOut)))
		ui.Text(c, a.tr("Recording")).FontSize(t.FontSize + 3).Bold().Grow(1)
		if a.busy {
			ui.Spinner(c).Label(a.tr("Finishing"))
		}
		pauseLabel := a.tr("Pause")
		if a.recordingPaused {
			pauseLabel = a.tr("Resume")
		}
		if ui.Button(c, pauseLabel).Disabled(a.busy).Clicked() {
			a.Rec.SetPaused(!a.recordingPaused)
		}
		if ui.PrimaryButton(c, a.tr("Stop")).Disabled(a.busy).Clicked() {
			a.stopRecording(false)
		}
	})
}

// recordingStage is the recording counterpart of nativePlayerStage, including
// the same square frame and dark transport bar.
func (a *App) recordingStage(c *ui.Context, t *ui.Theme, fitWidth bool) {
	ui.Column(c).FillWidth().Clip().Border(1, t.Border).Children(func() {
		a.previewPane(c, t, fitWidth)
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(3)).AlignItems(ui.Center).
			Background(playerBar(t)).Children(func() {
			ui.Spacer(c)
			ui.Text(c, fmtDuration(a.recordingElapsed().Milliseconds())).FontSize(t.FontSize - 2).
				TextColor(onMedia()).Opacity(0.9).FontFeatures("tnum").SingleLine().Shrink(0)
		})
	})
}

func (a *App) previewPane(c *ui.Context, t *ui.Theme, fitWidth bool) {
	from, to := mediaBackdrop(t.Dark)
	pane := ui.Column(c).Label(a.tr("Recording preview")).MinWidth(0).Clip().
		Border(0, t.Border).Gradient(from, to, 90)
	if fitWidth {
		ratio := float32(16.0 / 9.0)
		if a.previewW > 0 && a.previewH > 0 {
			ratio = float32(a.previewW) / float32(a.previewH)
		}
		pane.FillWidth().AspectRatio(ratio)
	} else {
		pane.Grow(1)
	}
	pane.Children(func() {
		if a.preview == nil {
			ui.Column(c).Fill().Center().Gap(t.Space(2)).Children(func() {
				ui.Spinner(c).Label(a.tr("Waiting for the picture"))
				ui.Text(c, a.tr("Waiting for the first frame…")).FontSize(t.FontSize - 2).TextColor(onMedia())
			})
			return
		}
		ui.Image(c, a.preview).Fit(ui.Contain).Grow(1)
	})
}

func (a *App) recordingTranscript(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(4), t.Space(4), 0, t.Space(4)).Gap(t.Space(2)).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			tab := 0
			ui.Tabs(c, &tab, a.tr("Transcript")).Grow(1)
			if a.canTranslateLive() {
				ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
					if ui.Switch(c, &a.liveTranslate).Label(a.tr("Translate")).Tooltip(a.tr("Translate")).Changed() {
						a.restartLiveTranslation()
					}
					ui.Text(c, a.tr("Translate")).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
				})
			}
			a.subtitleSettingsButton(c, t, nil)
		})
		ui.Column(c).Grow(1).MinHeight(0).Padding(t.Space(3), t.Space(4)).Children(func() {
			a.livePane(c, t)
		})
	})
}

func (a *App) livePane(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Gap(t.Space(2)).Children(func() {
		ui.Text(c, a.tr("Provisional: start transcription after the recording stops.")).
			FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		if a.liveTranslate {
			a.liveTranslationControls(c, t)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(t.Space(2.5)).Children(func() {
				if len(a.late) == 0 {
					ui.Text(c, a.tr("Listening for audio…")).TextColor(t.TextMuted).
						FontSize(t.FontSize - 1)
					return
				}
				for _, line := range a.late {
					a.subtitleContent(c, t, 0, fmtDuration(line.at), line.text, a.liveTrans[line.utterance].translated)
				}
				if a.liveWarn != "" {
					ui.Text(c, a.liveWarn).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
				}
				if a.liveTransErr != "" {
					ui.Text(c, a.liveTransErr).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
				}
			})
		})
	})
}

// canTranslateLive reports whether a provider is there to translate with.
func (a *App) canTranslateLive() bool {
	return a.liveTranslator != nil || a.Trans != nil
}

// liveTranslationControls picks the target language of the live lines. It is
// the same target the saved transcript uses, so switching one switches both.
func (a *App) liveTranslationControls(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconLanguages).FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		name := langName(a.trLang)
		ui.Select(c, &name, langNames()).Label(a.tr("Translate into")).Width(130).Changed()
		for _, l := range langs {
			if l.name == name && l.code != a.trLang {
				a.trLang = l.code
				a.restartLiveTranslation()
			}
		}
		if len(a.liveTransBusy) > 0 {
			ui.Spinner(c).Label(a.tr("Translating"))
		}
	})
}

// restartLiveTranslation drops every translated line and, when the switch is
// on, asks again for the lines that are already final.
func (a *App) restartLiveTranslation() {
	a.resetLiveTranslation()
	if !a.liveTranslate {
		return
	}
	for _, line := range a.late {
		if line.final {
			a.translateLiveLine(line.utterance, line.text)
		}
	}
}

// resetLiveTranslation forgets translated lines and drops results still in
// flight: a different switch state or language makes them stale.
func (a *App) resetLiveTranslation() {
	a.liveTransGen++
	a.liveTrans = nil
	a.liveTransBusy = nil
	a.liveTransErr = ""
}

// translateLiveLine translates one finalized line into trLang. Lines still
// being spoken are not sent, so each utterance costs one request.
func (a *App) translateLiveLine(utterance int64, text string) {
	translate := a.liveTranslator
	if translate == nil && a.Trans != nil {
		translate = a.Trans.TranslateTexts
	}
	if !a.liveTranslate || translate == nil || strings.TrimSpace(text) == "" {
		return
	}
	if prev, ok := a.liveTrans[utterance]; ok && prev.translated != "" && prev.text == text {
		return
	}
	if a.liveTransBusy[utterance] {
		return
	}
	if a.liveTransBusy == nil {
		a.liveTransBusy = map[int64]bool{}
	}
	a.liveTransBusy[utterance] = true

	gen, lang, source := a.liveTransGen, a.trLang, a.lang
	go func() {
		out, err := translate(context.Background(), []string{text}, source, lang)
		a.update(func() {
			delete(a.liveTransBusy, utterance)
			if gen != a.liveTransGen {
				return
			}
			if err != nil {
				a.liveTransErr = err.Error()
				return
			}
			if len(out) == 0 || strings.TrimSpace(out[0]) == "" {
				return
			}
			a.liveTransErr = ""
			if a.liveTrans == nil {
				a.liveTrans = map[int64]liveTranslation{}
			}
			a.liveTrans[utterance] = liveTranslation{text: text, translated: out[0]}
		})
	}()
}
