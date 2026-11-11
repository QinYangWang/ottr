package native

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/native/kit"
)

// onboardingPage is the first-run guide. Ottr recognizes speech on this
// computer, so it needs one model before a recording can be transcribed; the
// page is that one question, asked once, with the three models it supports.
func (a *App) onboardingPage(c *ui.Context, t *ui.Theme) {
	p := pal(t)
	if !a.modelLoaded {
		a.loadModels()
	}
	ui.Column(c).Grow(1).FillWidth().FillHeight().Background(p.Paper).Center().Padding(24, 24).Children(func() {
		ui.Column(c).Width(560).Gap(16).Padding(24, 28).Radius(kit.RadiusCard).Background(p.Sheet).
			Border(1, p.Line).Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, a.tr("Set up transcription")).FontSize(20).FontWeight(600).
					LetterSpacing(-0.3).TextColor(p.Ink)
				ui.Text(c, a.tr("Ottr transcribes on this computer. Download one model once; it works offline and stays in the models folder.")).
					FontSize(13).TextColor(p.Ink2)
			})
			kit.Group(c, t, func() {
				if a.modelLoading {
					ui.Row(c).Padding(14, 16).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).Label(a.tr("Looking for models")).Size(14, 14)
						ui.Text(c, a.tr("Looking for models…")).FontSize(12.5).TextColor(p.Ink3)
					})
					return
				}
				for i, offer := range a.modelList {
					a.modelRow(c.Key(offer.ID), t, offer, i == 0)
				}
			})
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				if kit.Button(c, t, kit.Ghost, nil, a.tr("Later"), false).
					Tooltip(a.tr("Record without a transcript for now")).Clicked() {
					a.onboarding, a.onboardLater = false, true
				}
				ui.Spacer(c)
				if a.MM != nil && kit.Button(c, t, kit.Ghost, iconFolderOpen, a.tr("Open folder"), false).
					Tooltip(a.tr("Open the folder models are installed in")).Clicked() {
					mygo.Shell.OpenPath(a.MM.ModelsDir())
				}
			})
		})
	})
}
