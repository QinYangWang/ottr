package native

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"ottr/internal/capture"
	"ottr/internal/native/kit"
)

// livePanel is the capture panel while recording: the clock, what is heard,
// Pause and Stop, and the live transcript with its translation under each
// final line. full lays it out as the whole window (the narrow layout), with
// the picture of what is recorded on top.
func (a *App) livePanel(c *ui.Context, t *ui.Theme, full bool) {
	p := pal(t)
	ui.Column(c).Fill().MinHeight(0).Background(p.Sheet).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(18, 20, 16, 20).Gap(12).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := a.tr("Recording")
				if a.recordingPaused {
					label = a.tr("Paused")
				}
				kit.Chip(c, t, kit.ChipRec, label)
				ui.Text(c, a.liveSourceLine()).FontSize(12).TextColor(p.Ink3).SingleLine().MinWidth(0)
				ui.Spacer(c)
				if a.busy {
					ui.Spinner(c).Label(a.tr("Finishing")).Size(14, 14)
				}
			})
			ui.Row(c).AlignItems(ui.End).Gap(8).Children(func() {
				mono(ui.Text(c, fmtClock(a.recordingElapsed().Milliseconds()))).FontSize(34).
					FontWeight(500).LetterSpacing(-0.5).TextColor(p.Ink).Grow(1)
				if a.previewW > 0 {
					mono(ui.Text(c, a.trf("%dp", a.previewH))).FontSize(11.5).TextColor(p.Ink3).
						Padding(0, 0, 6, 0)
				}
			})
			if full {
				a.recordingStage(c, t)
			}
			ui.Column(c).Gap(4).Children(func() {
				a.liveSoundRow(c, t, iconSpeaker, iconSpeaker, "System audio", a.speaker)
				a.liveSoundRow(c, t, iconMic, iconMicOff, "Microphone", a.microphone)
			})
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				pauseLabel, pauseIcon := a.tr("Pause"), iconPause
				if a.recordingPaused {
					pauseLabel, pauseIcon = a.tr("Resume"), iconPlay
				}
				if kit.Button(c, t, kit.Secondary, pauseIcon, pauseLabel, true).Grow(1).Basis(0).
					Disabled(a.busy).Clicked() && !a.busy {
					a.Rec.SetPaused(!a.recordingPaused)
				}
				if kit.Button(c, t, kit.Danger, iconStop, a.tr("Stop"), true).Grow(1).Basis(0).
					Disabled(a.busy).Clicked() && !a.busy {
					a.stopRecording(false)
				}
			})
		})
		a.liveTranscript(c, t)
	})
}

// liveSourceLine is what is recorded, said briefly: "Display 1 · System audio".
func (a *App) liveSourceLine() string {
	parts := []string{a.sourceName()}
	if a.speaker.ID != "" {
		parts = append(parts, a.tr("System audio"))
	}
	if a.microphone.ID != "" {
		parts = append(parts, a.tr("Microphone"))
	}
	return strings.Join(parts, " · ")
}

// liveSoundRow is one sound the recording takes, or Off.
func (a *App) liveSoundRow(c *ui.Context, t *ui.Theme, icon, offIcon *ui.SVG, label string, device capture.AudioDevice) {
	p := pal(t)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		glyph, value := icon, device.Name
		if device.ID == "" {
			glyph, value = offIcon, a.tr("Off")
		}
		ui.Icon(c, glyph).FontSize(14).TextColor(p.Ink3)
		ui.Text(c, a.tr(label)).FontSize(11.5).TextColor(p.Ink3).Width(84).SingleLine()
		ui.Text(c, value).FontSize(11.5).TextColor(p.Ink2).SingleLine().MinWidth(0)
	})
}

// recordingStage is the picture of what is being recorded.
func (a *App) recordingStage(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ratio := float32(16.0 / 9.0)
	if a.previewW > 0 && a.previewH > 0 {
		ratio = float32(a.previewW) / float32(a.previewH)
	}
	pane := ui.Column(c).Label(a.tr("Recording preview")).FillWidth().AspectRatio(ratio).MinWidth(0).
		Radius(kit.RadiusCard).Clip().Background(p.Media)
	pane.Children(func() {
		if a.preview == nil {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Spinner(c).Label(a.tr("Waiting for the picture"))
				ui.Text(c, a.tr("Waiting for the first frame…")).FontSize(12).TextColor(onMedia()).Opacity(0.7)
			})
			return
		}
		ui.Image(c, a.preview).Fit(ui.Contain).Grow(1)
	})
}

// liveTranscript is the live text: its controls, the lines, and what the text
// is worth.
func (a *App) liveTranscript(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Padding(14, 20, 6, 20).Gap(6).AlignItems(ui.Center).Children(func() {
			kit.Label(c, t, a.tr("Live transcript")).Grow(1)
			if a.canTranslateLive() {
				a.liveTranslateChip(c, t)
			}
			a.liveLanguageSelect(c, t)
		})
		if a.liveTranslate && a.canTranslateLive() {
			a.liveTranslationControls(c, t)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(6, 20, 16, 20).Gap(14).Children(func() {
				shown := 0
				for _, line := range a.late {
					if !a.liveMatches(line.text) {
						continue
					}
					shown++
					a.liveLineRow(c.Key(line.utterance), t, line, a.liveTrans[line.utterance].translated)
				}
				if len(a.late) == 0 {
					ui.Text(c, a.tr("Listening for audio…")).FontSize(13).TextColor(p.Ink3)
				} else if shown == 0 {
					ui.Text(c, a.tr("Nothing matches that search.")).FontSize(13).TextColor(p.Ink3)
				}
				if a.liveWarn != "" {
					ui.Text(c, a.liveWarn).FontSize(11.5).TextColor(p.Ink3)
				}
				if a.liveTransErr != "" {
					ui.Text(c, a.liveTransErr).FontSize(11.5).TextColor(p.Ink3)
				}
			})
		})
		ui.Row(c).FillWidth().Shrink(0).Padding(12, 20).Gap(8).AlignItems(ui.Start).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
			ui.Icon(c, iconCPU).FontSize(14).TextColor(p.Ink3).Shrink(0)
			ui.Text(c, a.tr("Provisional. When you stop, the full pass on this computer replaces it and adds speakers.")).
				FontSize(11.5).TextColor(p.Ink3)
		})
	})
}

// liveTranslateChip turns the translation of final lines on and off: amber
// when on.
func (a *App) liveTranslateChip(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	label := a.trf("%s on", langName(a.trLang))
	if !a.liveTranslate {
		label = a.trf("%s off", langName(a.trLang))
	}
	chip := ui.ButtonBase(c).Label(a.tr("Translate")).Tooltip(a.tr("Translate")).Role(ui.RoleSwitch).
		Checked(a.liveTranslate).Height(22).Padding(0, 8).Radius(9999).Center().Cursor(ui.CursorPointer)
	if a.liveTranslate {
		chip.Background(p.AmberSoft).TextColor(p.AmberInk)
	} else {
		chip.Background(p.Hover).TextColor(p.Ink2)
	}
	chip.Children(func() { ui.Text(c, label).FontSize(11).FontWeight(500).SingleLine() })
	if chip.Clicked() {
		a.liveTranslate = !a.liveTranslate
		a.restartLiveTranslation()
	}
}

// liveLineRow is one live line: its time, the text, and the translation under
// a final line. The line still being spoken is dimmed, with an amber caret.
func (a *App) liveLineRow(c *ui.Context, t *ui.Theme, line liveLine, translated string) {
	p := pal(t)
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		mono(ui.Text(c, fmtDuration(line.at))).FontSize(11.5).TextColor(p.Ink3).Width(36).Shrink(0).
			Padding(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.End).Wrap().Children(func() {
				text := ui.Text(c, line.text).FontSize(a.subtitleSize() - 0.5).Selectable().MinWidth(0)
				if line.final {
					text.TextColor(p.Ink)
				} else {
					text.TextColor(p.Ink3)
					ui.Box(c).Size(2, 15).Background(p.Amber).Margin(0, 0, 2, 2)
				}
			})
			if translated != "" && line.final {
				ui.Text(c, translated).FontSize(max(a.subtitleSize()-1.5, 10)).TextColor(p.Ink3).Selectable()
			}
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
	kit.CompactSelect(c, t, &lang, options, a.tr("Spoken language"), 96, nil, func(choice string) {
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
	p := pal(t)
	ui.Row(c).Padding(0, 20, 4, 20).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.tr("Translate into")).FontSize(11.5).TextColor(p.Ink3)
		name := langName(a.trLang)
		options := make([]kit.SelectOption[string], len(langs))
		for i, l := range langs {
			options[i] = kit.SelectOption[string]{Value: l.name, Label: l.name}
		}
		kit.CompactSelect(c, t, &name, options, a.tr("Translate into"), 120, nil, func(choice string) {
			for _, l := range langs {
				if l.name == choice && l.code != a.trLang {
					a.trLang = l.code
					a.restartLiveTranslation()
				}
			}
		})
		if len(a.liveTransBusy) > 0 {
			ui.Spinner(c).Label(a.tr("Translating")).Size(12, 12)
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
