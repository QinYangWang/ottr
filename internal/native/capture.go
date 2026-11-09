package native

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/capture"
	"ottr/internal/library"
	"ottr/internal/native/kit"
)

// capturePanel is the 360px panel docked at the right of the wide window: a
// new capture, which becomes the live panel while recording.
func (a *App) capturePanel(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	ui.Column(c).Width(captureWidth).Shrink(0).FillHeight().MinHeight(0).
		Background(p.Sheet).BorderWidth(0, 0, 0, 1).BorderColor(p.Line).Children(func() {
		if a.recording {
			a.livePanel(c, t, false)
			return
		}
		a.captureHead(c, t, false)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(16, 20, 20, 20).Gap(12).Children(func() {
				a.sourceSection(c, t)
				a.audioSection(c, t)
			})
		})
		a.startFooter(c, t, true)
	})
	a.recordShortcut(c)
}

// recordShortcut starts a recording with Ctrl Shift R, and stops one.
func (a *App) recordShortcut(c *ui.Context) {
	if !c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyR) {
		return
	}
	if a.recording {
		a.stopRecording(false)
		return
	}
	a.startRecording()
}

// captureHead is "New capture" and its shortcut; the narrow window adds the
// way to the library.
func (a *App) captureHead(c *ui.Context, t *ui.Theme, withLibrary bool) {
	p := pal(t)
	ui.Row(c).FillWidth().Height(52).Shrink(0).Padding(0, 20).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Line).Children(func() {
		ui.Text(c, a.tr("New capture")).FontSize(15).FontWeight(600).TextColor(p.Ink).Grow(1)
		for _, key := range []string{"Ctrl", "Shift", "R"} {
			kit.Kbd(c, t, key)
		}
		if withLibrary {
			ui.Box(c).Width(8)
			width, _ := c.Size()
			kit.IconButton(c, t, iconLibrary, a.tr("Library"), func() { a.toggleLibrary(width) })
		}
	})
}

// sourceSection is what to record: the four source tiles, the picture of
// what is chosen, and the display or window select.
func (a *App) sourceSection(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if len(a.sources) == 0 {
		ui.Text(c, a.tr("Nothing can be recorded on this system yet.")).FontSize(13).TextColor(p.Ink3)
		return
	}
	a.sourceCards(c, t)
	a.sourcePreview(c, t)
	a.sourceSelect(c, t)
}

// audioSection is the sounds a recording takes and what happens while it
// runs.
func (a *App) audioSection(c *ui.Context, t *ui.Theme) {
	if len(a.sources) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(8).Padding(6, 0, 0, 0).Children(func() {
		kit.Label(c, t, a.tr("Audio"))
		a.audioRow(c, t, &a.speaker, a.speakers, iconSpeaker, "System audio")
		a.audioRow(c, t, &a.microphone, a.microphones, iconMic, "Microphone")
	})
	ui.Column(c).FillWidth().Gap(8).Padding(6, 0, 0, 0).Children(func() {
		kit.Label(c, t, a.tr("While recording"))
		a.liveRow(c, t)
		if a.canTranslateLive() {
			a.translateRow(c, t)
		}
	})
}

// liveRow chooses the live transcript of the next recording: the model, or
// none. It is offered whenever the build can recognize speech while recording.
func (a *App) liveRow(c *ui.Context, t *ui.Theme) {
	if a.Rec == nil || a.Rec.Live == nil && a.liveStarter == nil {
		return
	}
	model := ""
	if a.MM != nil {
		model = shortModel(a.MM.CurrentName())
	}
	if model == "" {
		model = a.tr("On")
	}
	on := !a.liveOff
	options := []kit.SelectOption[bool]{{Value: false, Label: a.tr("Disabled")}, {Value: true, Label: model}}
	kit.Select(c, t, &on, options, a.tr("Live transcript"), a.tr("Live transcript"), iconCaptions, a.setLive)
}

// translateRow chooses the language live speech is translated into, or none.
func (a *App) translateRow(c *ui.Context, t *ui.Theme) {
	on := a.liveTranslate
	options := []kit.SelectOption[bool]{{Value: false, Label: a.tr("Disabled")}, {Value: true, Label: langName(a.trLang)}}
	kit.Select(c, t, &on, options, a.tr("Translate"), a.tr("Translate"), iconLanguages, func(on bool) {
		a.liveTranslate = on
		a.restartLiveTranslation()
	})
}

// setLive keeps or drops the recorder's live sink for the next recording.
func (a *App) setLive(on bool) {
	if a.Rec == nil || a.recording {
		return
	}
	if !on && a.Rec.Live != nil {
		a.liveStarter, a.Rec.Live = a.Rec.Live, nil
	}
	if on && a.Rec.Live == nil {
		a.Rec.Live = a.liveStarter
	}
	a.liveOff = !on
}

// shortModel is the first word of a model's name: "SenseVoice".
func shortModel(name string) string {
	if i := strings.IndexAny(name, " ·("); i > 0 {
		return name[:i]
	}
	return name
}

// audioRow is one sound a recording can take: the device to listen to, or
// none.
func (a *App) audioRow(c *ui.Context, t *ui.Theme, selected *capture.AudioDevice, devices []capture.AudioDevice, icon *ui.SVG, label string) {
	options := make([]kit.SelectOption[string], 0, len(devices)+1)
	options = append(options, kit.SelectOption[string]{Value: "", Label: a.tr("Disabled")})
	for _, device := range devices {
		options = append(options, kit.SelectOption[string]{Value: device.ID, Label: device.Name})
	}
	id := selected.ID
	kit.Select(c, t, &id, options, a.tr(label), a.tr(label)+a.tr(" device"), icon, func(id string) {
		*selected = capture.AudioDevice{}
		for _, device := range devices {
			if device.ID == id {
				*selected = device
			}
		}
	})
}

// sourceCard is one tile of the capture-source row: Full screen and Window
// choose; Area and Camera are not built yet and say so.
type sourceCard struct {
	label   string
	icon    *ui.SVG
	id      string // the source a click chooses
	chosen  bool
	blocked string // why this source cannot be recorded
	soon    bool   // not built yet
}

// sourceCards lays the four tiles out in one row.
func (a *App) sourceCards(c *ui.Context, t *ui.Theme) {
	cards := a.sourceCardsOf(a.sources)
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		for _, card := range cards {
			a.sourceCard(c, t, card)
		}
	})
}

// sourceCardHeight is the height every tile takes, so that the row lines up.
const sourceCardHeight = 58

func (a *App) sourceCard(c *ui.Context, t *ui.Theme, card sourceCard) {
	p := pal(t)
	card.label = a.tr(card.label)
	card.blocked = a.tr(card.blocked)
	btn := ui.ButtonBase(c.Key(card.label)).Label(card.label).Grow(1).Width(0).MinWidth(0).
		Height(sourceCardHeight).Radius(kit.RadiusCard).Center().Background(p.Sheet)
	switch {
	case card.soon:
		btn.Disabled(true).Tooltip(card.blocked).BorderStyle(ui.BorderDashed).Border(1, p.Line2)
	case card.blocked != "":
		btn.Disabled(true).Tooltip(card.blocked).Border(1, p.Line)
	case card.chosen:
		btn.Border(1, p.Ink).InsetShadow(0, 0, 0, 1, p.Ink).Cursor(ui.CursorPointer)
	default:
		btn.Border(1, p.Line2).Cursor(ui.CursorPointer)
		if btn.Hovered() {
			btn.Background(p.Hover)
		}
	}
	btn.Children(func() {
		ink := p.Ink2
		switch {
		case card.chosen:
			ink = p.Ink
		case card.soon || card.blocked != "":
			ink = p.Ink4
		}
		ui.Column(c).AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Icon(c, card.icon).FontSize(18).TextColor(ink)
			text := ui.Text(c, card.label).FontSize(11.5).TextColor(ink).SingleLine()
			if card.chosen {
				text.FontWeight(500)
			}
		})
		if card.soon {
			mono(ui.Text(c, a.tr("SOON"))).FontSize(8.5).LetterSpacing(0.4).TextColor(p.Ink3).
				Attach(ui.AnchorTopRight, ui.AnchorTopRight).Top(3).Right(5)
		}
	})
	if !card.soon && card.blocked == "" && btn.Clicked() {
		a.chooseSource(card.id)
	}
}

// sourcePreview shows the source before it is recorded — the frames the
// previewer draws — or what keeps them away, with the source named in a chip.
func (a *App) sourcePreview(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	label := a.sourceName()
	if detail := a.sourceDetail(); detail != "" {
		label += " · " + strings.ReplaceAll(detail, "x", "×")
	}
	box := ui.Column(c).Label(a.tr("Source preview")).FillWidth().AspectRatio(16.0 / 9.0).
		Radius(kit.RadiusCard).Clip().Background(p.Media)
	box.Children(func() {
		if a.preview != nil {
			ui.Image(c, a.preview).Fit(ui.Contain).Grow(1)
		} else {
			ui.Column(c).Grow(1).Center().Padding(12).Gap(8).Children(func() {
				ui.Icon(c, iconScreen).FontSize(26).TextColor(onMedia()).Opacity(0.6)
				if a.previewErr == "" {
					ui.Text(c, a.tr("Waiting for the first frame…")).FontSize(12).
						TextColor(onMedia()).Opacity(0.65).SingleLine().TextAlign(ui.Center)
					return
				}
				// What the capture backend said, whole: it is the only clue
				// there is.
				ui.Text(c, a.previewErr).FontSize(12).TextColor(onMedia()).Opacity(0.75).
					MaxWidth(300).TextAlign(ui.Center).Selectable()
				if kit.Button(c, t, kit.Secondary, nil, a.tr("Try again"), false).Clicked() {
					a.showPreview()
				}
			})
		}
		ui.Row(c).Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).Top(8).Left(8).Height(20).Padding(0, 7).
			Radius(9999).Background(ui.RGBA(255, 255, 255, 0.92)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(ui.Hex("#121417")).SingleLine()
		})
	})
}

// startFooter is the button that begins a recording, always in reach at the
// bottom of the panel.
func (a *App) startFooter(c *ui.Context, t *ui.Theme, wide bool) {
	p := pal(t)
	if len(a.sources) == 0 {
		return
	}
	pad := float32(20)
	if wide {
		pad = 20
	}
	ui.Row(c).FillWidth().Shrink(0).Padding(14, pad).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Background(p.Sheet).Children(func() {
		start := ui.ButtonBase(c).Label(a.tr("Start recording")).Disabled(a.busy).Grow(1).Height(44).
			Radius(kit.RadiusControl).Background(p.Btn).Center().Cursor(ui.CursorPointer)
		if start.Hovered() && !a.busy {
			start.Background(p.Btn.Mix(ui.RGB(128, 128, 128), 0.14))
		}
		start.Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if a.busy {
					ui.Spinner(c).Label(a.tr("Starting")).Size(14, 14)
					ui.Text(c, a.tr("Starting…")).FontSize(14).FontWeight(600).TextColor(p.BtnInk)
					return
				}
				ui.Box(c).Size(10, 10).Radius(5).Background(p.Red)
				ui.Text(c, a.tr("Start recording")).FontSize(14).FontWeight(600).TextColor(p.BtnInk)
			})
		})
		if start.Clicked() {
			a.startRecording()
		}
		if !wide {
			kit.RoundButton(c, t, iconSettings, a.tr("Settings"), func() {
				a.settingsTab = 0
				a.openSettings()
			})
		}
	})
}

// recorderPage is the recorder where the window has room for one page: the
// capture form, or the live panel while recording.
func (a *App) recorderPage(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	width, _ := c.Size()
	narrow := layoutOf(width) == "narrow"
	ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.Sheet).Children(func() {
		if a.recording {
			if narrow {
				a.livePanel(c, t, true)
				return
			}
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
					ui.Column(c).Padding(20, 24).Gap(12).Children(func() {
						kit.Label(c, t, a.tr("Recording"))
						a.recordingStage(c, t)
					})
				})
				ui.Column(c).Width(captureWidth).Shrink(0).FillHeight().BorderWidth(0, 0, 0, 1).
					BorderColor(p.Line).Children(func() { a.livePanel(c, t, false) })
			})
			return
		}
		a.captureHead(c, t, narrow)
		if narrow {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(14, 20, 20, 20).Gap(12).Children(func() {
					a.sourceSection(c, t)
					a.audioSection(c, t)
				})
			})
		} else {
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				ui.Scroll(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					ui.Column(c).Padding(16, 24, 20, 24).Gap(12).Children(func() {
						a.sourceSection(c, t)
					})
				})
				ui.Scroll(c).Grow(1).Basis(0).MinWidth(0).BorderWidth(0, 0, 0, 1).BorderColor(p.Line).Children(func() {
					ui.Column(c).Padding(16, 24, 20, 24).Gap(12).Children(func() {
						a.audioSection(c, t)
						a.lastRecording(c, t)
					})
				})
			})
		}
		a.startFooter(c, t, false)
	})
	a.recordShortcut(c)
}

// lastRecording is the newest recording, a step away from the recorder.
func (a *App) lastRecording(c *ui.Context, t *ui.Theme) {
	var last library.Media
	for _, m := range a.media {
		if m.Source == "recording" {
			last = m
			break
		}
	}
	if last.ID == 0 {
		return
	}
	p := pal(t)
	ui.Column(c).FillWidth().Gap(8).Padding(6, 0, 0, 0).Children(func() {
		kit.Label(c, t, a.tr("Last recording"))
		row := ui.ButtonBase(c).Label(last.Name).FillWidth().Padding(10, 12).Gap(12).AlignItems(ui.Center).
			Radius(kit.RadiusCard).Border(1, p.Line).Background(p.Sheet).Cursor(ui.CursorPointer)
		if row.Hovered() {
			row.Background(p.Sunk)
		}
		row.Children(func() {
			ui.Column(c).Width(72).Shrink(0).Children(func() { a.mediaThumb(c, t, last) })
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, last.Name).FontSize(13).FontWeight(500).SingleLine()
				ui.Text(c, a.dayGroup(time.Now(), last.CreatedAt)+" "+last.CreatedAt.Local().Format("15:04")+
					" · "+durationOrDash(last)).FontSize(11.5).TextColor(p.Ink3).SingleLine()
			})
			ui.Icon(c, iconChevronRight).FontSize(16).TextColor(p.Ink3)
		})
		if row.Clicked() {
			a.openDetail(last.ID)
		}
	})
}
