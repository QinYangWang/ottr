package native

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// recorderPage is the recording shown in place of the grid: the picture as it
// is recorded, the live transcript beside it, and what ends the recording
// above.
func (a *App) recorderPage(c *ui.Context, t *ui.Theme) {
	elapsed := time.Since(a.startedAt).Milliseconds()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Gap(t.Space(3)).Padding(t.Space(3.5), t.Space(5)).AlignItems(ui.Center).
			Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			dot := ui.Box(c).Size(10, 10).Radius(5).Background(t.Danger)
			dot.Opacity(float32(0.35 + 0.65*dot.Loop("pulse", 1100*time.Millisecond, ui.EaseInOut)))
			ui.Text(c, "Recording").Bold()
			ui.Text(c, fmtDuration(elapsed)).FontSize(t.FontSize - 1).TextColor(t.TextMuted).
				FontFeatures("tnum")
			ui.Spacer(c)
			if ui.Button(c, "Delete…").Disabled(a.busy).Clicked() {
				a.discard = true
			}
			if ui.PrimaryButton(c, "Stop and transcribe").Disabled(a.busy).Clicked() {
				a.stopRecording(false)
			}
		})
		ui.Row(c).Grow(1).MinHeight(0).Padding(t.Space(4), t.Space(5)).
			Gap(t.Space(4)).AlignItems(ui.Stretch).Children(func() {
			a.previewPane(c, t)
			ui.Column(c).Width(sideWidth).Shrink(0).MinWidth(0).FillHeight().
				Background(t.Surface).Radius(t.Radius).Border(1, t.Border).Clip().
				Padding(t.Space(3), t.Space(4)).Children(func() {
				a.livePane(c, t)
			})
		})
		if a.busy {
			ui.Row(c).Gap(t.Space(2)).Padding(0, t.Space(5), t.Space(4)).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Label("Finishing")
				ui.Text(c, "Finishing the file…").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			})
		}
	})
}

func (a *App) previewPane(c *ui.Context, t *ui.Theme) {
	from, to := mediaBackdrop(t.Dark)
	ui.Column(c).Grow(1).MinWidth(0).Radius(t.Radius+2).Clip().
		Border(1, t.Border).Gradient(from, to, 90).Children(func() {
		if a.preview == nil {
			ui.Column(c).Fill().Center().Gap(t.Space(2)).Children(func() {
				ui.Spinner(c).Label("Waiting for the picture")
				ui.Text(c, "Waiting for the first frame…").FontSize(t.FontSize - 2).TextColor(onMedia())
			})
			return
		}
		ui.Image(c, a.preview).Fit(ui.Contain).Grow(1)
	})
}

func (a *App) livePane(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Live transcript").Bold().Grow(1)
			if a.busy {
				ui.Spinner(c).Label("Finishing")
			}
		})
		ui.Text(c, "Provisional: the file is transcribed again when the recording stops.").
			FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(t.Space(2.5)).Children(func() {
				if len(a.late) == 0 {
					ui.Text(c, "Listening for audio…").TextColor(t.TextMuted).
						FontSize(t.FontSize - 1)
					return
				}
				for _, line := range a.late {
					ui.Column(c).Gap(t.Space(1)).Children(func() {
						ui.Text(c, fmtDuration(line.at)).FontSize(t.FontSize - 3).
							TextColor(t.TextMuted).FontFeatures("tnum")
						text := ui.Text(c, line.text).FontSize(t.FontSize - 1)
						if line.final {
							text.TextColor(t.TextMuted)
						}
					})
				}
				if a.liveWarn != "" {
					ui.Text(c, a.liveWarn).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
				}
			})
		})
	})
}
