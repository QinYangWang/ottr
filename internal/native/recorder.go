package native

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"ottr/internal/native/kit"
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
				a.recordingFacts(c, t)
			})
		})
		ui.Column(c).Width(sideWidth).Shrink(0).FillHeight().MinWidth(0).
			Background(t.Surface).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
			a.recordingTranscript(c, t)
		})
	})
}

// recordingFacts is the strip under the preview: where the recording comes
// from, what it hears, and which model is writing it.
func (a *App) recordingFacts(c *ui.Context, t *ui.Theme) {
	source := a.sourceName()
	if detail := a.sourceDetail(); detail != "" {
		source += " · " + detail
	}
	systemAudio := a.tr("Off")
	if a.speaker.Name != "" {
		systemAudio = a.speaker.Name
	}
	microphone := a.tr("Off")
	if a.microphone.Name != "" {
		microphone = a.microphone.Name
	}
	model := ""
	if a.MM != nil {
		model = a.MM.CurrentName()
	}
	facts := [][2]string{
		{a.tr("Source"), source},
		{a.tr("System audio"), systemAudio},
		{a.tr("Microphone"), microphone},
		{a.tr("Model"), model},
	}
	ui.Row(c).FillWidth().Radius(t.Radius).Border(1, t.Border).Clip().
		AlignItems(ui.Stretch).Children(func() {
		for i, fact := range facts {
			if i > 0 {
				ui.Column(c).Width(1).Shrink(0).Background(t.Border)
			}
			ui.Column(c).Grow(1).MinWidth(0).Padding(t.Space(2.5), t.Space(3.5)).Gap(t.Space(1)).Children(func() {
				ui.Text(c, fact[0]).FontSize(t.FontSize - 3).Bold().TextColor(t.TextMuted).SingleLine()
				ui.Text(c, fact[1]).FontSize(t.FontSize - 1).Bold().SingleLine()
			})
		}
	})
}

// recordingHead is the title, the running clock and the two transport buttons.
func (a *App) recordingHead(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(2.5)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.tr("Recording")).FontSize(t.FontSize + 3).Bold()
		ui.Column(c).Padding(t.Space(0.5), t.Space(2)).Radius(9999).
			Background(t.Danger.Alpha(0.12)).Children(func() {
			ui.Text(c, fmtDuration(a.recordingElapsed().Milliseconds())).FontSize(t.FontSize - 3).
				Bold().TextColor(t.Danger).FontFeatures("tnum")
		})
		ui.Spacer(c)
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
		if kit.DangerButton(c, t, a.tr("Stop")).Disabled(a.busy).Clicked() {
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
			ui.Text(c, a.tr("Recording to disk")).FontSize(t.FontSize - 2).
				TextColor(onMedia()).Opacity(0.9).SingleLine().Shrink(0)
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
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			a.transcriptSearchButton(c, t)
			a.liveLanguageSelect(c, t)
		})
		ui.Text(c, a.tr("Provisional: start transcription after the recording stops.")).
			FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		if a.liveTranslate {
			a.liveTranslationControls(c, t)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(t.Space(2.5)).Children(func() {
				shown := 0
				for _, line := range a.late {
					if !a.liveMatches(line.text) {
						continue
					}
					shown++
					a.subtitleContent(c, t, 0, fmtDuration(line.at), line.text, a.liveTrans[line.utterance].translated)
				}
				if len(a.late) == 0 {
					ui.Text(c, a.tr("Listening for audio…")).TextColor(t.TextMuted).
						FontSize(t.FontSize - 1)
				} else if shown == 0 {
					ui.Text(c, a.tr("Nothing matches that search.")).TextColor(t.TextMuted).
						FontSize(t.FontSize - 1)
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

// liveLanguageSelect sets the spoken language of the recording in progress, as
// the same control does on a saved transcript.
func (a *App) liveLanguageSelect(c *ui.Context, t *ui.Theme) {
	names, codes := a.langChoices()
	lang := a.langLabel(a.defaultLanguage)
	options := make([]kit.SelectOption[string], len(names))
	for i, name := range names {
		options[i] = kit.SelectOption[string]{Value: name, Label: name}
	}
	kit.CompactSelect(c, t, &lang, options, a.tr("Spoken language"), 120, iconEarth, func(choice string) {
		for i, name := range names {
			if name == choice && codes[i] != a.defaultLanguage {
				a.setDefaultLanguage(codes[i], nil)
			}
		}
	})
}

// liveMatches filters live text by the transcript search query.
func (a *App) liveMatches(text string) bool {
	q := strings.ToLower(strings.TrimSpace(a.segQuery))
	return q == "" || strings.Contains(strings.ToLower(text), q)
}

// canTranslateLive reports whether a provider is there to translate with.
func (a *App) canTranslateLive() bool {
	return a.liveTranslator != nil || a.Trans != nil
}

// liveTranslationControls picks the target language of the live lines. It is
// the same target the saved transcript uses, so switching one switches both.
func (a *App) liveTranslationControls(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconEarth).FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		name := langName(a.trLang)
		options := make([]kit.SelectOption[string], len(langs))
		for i, l := range langs {
			options[i] = kit.SelectOption[string]{Value: l.name, Label: l.name}
		}
		kit.CompactSelect(c, t, &name, options, a.tr("Translate into"), 130, nil, func(choice string) {
			for _, l := range langs {
				if l.name == choice && l.code != a.trLang {
					a.trLang = l.code
					a.restartLiveTranslation()
				}
			}
		})
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
