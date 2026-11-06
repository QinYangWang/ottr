package native

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/native/kit"
)

// sidebar is the recording form. Beside an expanded library it keeps its
// compact width; otherwise it uses the whole window.
func (a *App) sidebar(c *ui.Context, t *ui.Theme, besideLibrary bool) {
	side := ui.Column(c).FillHeight().Grow(1).Background(t.Background)
	if besideLibrary {
		side.Width(CompactWidth).Grow(0).Shrink(0)
	}
	side.Children(func() {
		// The cards and picture scroll in a short window; the recording
		// controls and window buttons stay in reach.
		ui.Scroll(c).MaxHeight(formHeight).Children(func() {
			a.recordForm(c, t)
		})
		a.audioBar(c, t)
		a.startBar(c, t)
		ui.Spacer(c)
	})
}

// formHeight bounds the scrolling part of the recording form — the cards and
// the picture — which leaves the library its room in a short window.
const formHeight = 480

// recordForm chooses what to record: the sources as the cards of a grid, and the
// picture of what is chosen.
func (a *App) recordForm(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Padding(t.Space(4), t.Space(4), t.Space(2), t.Space(4)).Gap(t.Space(3)).Children(func() {
		if a.recording {
			ui.Text(c, a.tr("Choosing another file is off until it stops.")).
				FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			return
		}
		if len(a.sources) == 0 {
			ui.Text(c, a.tr("Nothing can be recorded on this system yet.")).
				FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			return
		}
		a.caption(c, t, a.tr("Choose a capture source"))
		a.sourceCards(c, t)
		a.sourceSelect(c, t)
		a.sourcePreview(c, t)
	})
}

// audioBar chooses the speaker and microphone a recording takes.
func (a *App) audioBar(c *ui.Context, t *ui.Theme) {
	if a.recording || len(a.sources) == 0 {
		return
	}
	ui.Column(c).Padding(t.Space(2), t.Space(4), 0, t.Space(4)).Gap(t.Space(2)).Children(func() {
		a.audioRow(c, t, &a.speaker, a.speakers, iconSpeaker, "System audio")
		a.audioRow(c, t, &a.microphone, a.microphones, iconMic, "Microphone")
	})
}

// startBar is the button that begins the recording, under the form and always
// in reach: the library and Settings sit beside it, as round buttons.
func (a *App) startBar(c *ui.Context, t *ui.Theme) {
	if a.recording || len(a.sources) == 0 {
		return
	}
	ui.Column(c).Padding(t.Space(2.5), t.Space(4), t.Space(3), t.Space(4)).Gap(t.Space(2)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			start := ui.PrimaryButton(c, "").Label(a.tr("Start recording")).Disabled(a.busy).Grow(1)
			start.Children(func() {
				ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
					if a.busy {
						ui.Spinner(c).Label(a.tr("Starting"))
						ui.Text(c, a.tr("Starting…"))
					} else {
						ui.Text(c, a.tr("Start recording"))
					}
				})
			})
			if start.Clicked() {
				a.startRecording()
			}
			width, _ := c.Size()
			kit.RoundButton(c, t, iconClapper, a.tr("Library"), func() { a.toggleLibrary(width) })
			kit.RoundButton(c, t, iconSettings, a.tr("Settings"), a.openSettings)
		})
	})
}

// openLibrary shows the grid beside the recorder, or as a drawer in a
// narrow window. It never opens another window.
func (a *App) openLibrary() {
	a.libraryOpen, a.librarySet = true, true
	a.closeDetail()
	a.reload()
}

// libraryPanel is the compact, list-shaped library beside the recorder.
func (a *App) libraryPanel(c *ui.Context, t *ui.Theme) {
	kit.SidePanel(c, t, libraryWidth, func() {
		ui.Row(c).Padding(t.Space(4), t.Space(4), t.Space(3), t.Space(4)).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			a.libraryTools(c, t)
			icon, label := iconGrid, "Grid view"
			if a.libraryGridView {
				icon, label = iconList, "List view"
			}
			a.iconButton(c, t, icon, a.tr(label), func() { a.libraryGridView = !a.libraryGridView })
			width, _ := c.Size()
			a.iconButton(c, t, iconPanelClose, a.tr("Close library"), func() { a.toggleLibrary(width) })
		})
		a.libraryGrid(c, t)
	})
}

// detail reports whether one media's page is shown instead of the grid.
func (a *App) detail() bool { return a.sel != 0 }

// sourcePreview shows the source before it is recorded — the frames the
// previewer draws — or what keeps them away.
func (a *App) sourcePreview(c *ui.Context, t *ui.Theme) {
	from, to := mediaBackdrop(t.Dark)
	box := ui.Column(c).FillWidth().AspectRatio(16.0/9.0).Radius(t.Radius+2).
		Clip().Border(1, t.Border).Gradient(from, to, 90)
	box.Children(func() {
		if a.preview != nil {
			ui.Image(c, a.preview).Fit(ui.Cover).Grow(1)
			return
		}
		ui.Column(c).Grow(1).Center().Padding(t.Space(3)).Gap(t.Space(2)).Children(func() {
			ui.Icon(c, iconScreen).FontSize(t.FontSize + 10).TextColor(onMedia()).Opacity(0.75)
			label := a.sourceName()
			if detail := a.sourceDetail(); detail != "" {
				label += " · " + detail
			}
			ui.Text(c, label).FontSize(t.FontSize - 1).TextColor(onMedia()).SingleLine()
			if a.previewErr == "" {
				ui.Text(c, a.tr("Waiting for the first frame…")).FontSize(t.FontSize - 2).
					TextColor(onMedia()).Opacity(0.65).SingleLine().TextAlign(ui.Center)
				return
			}
			// What the capture backend said, whole: it is the only clue
			// there is.
			ui.Text(c, a.previewErr).FontSize(t.FontSize - 2).TextColor(onMedia()).
				Opacity(0.75).MaxWidth(340).TextAlign(ui.Center).Selectable()
			if ui.Button(c, a.tr("Try again")).Clicked() {
				a.showPreview()
			}
		})
	})
}

// caption names what the controls under it choose.
func (a *App) caption(c *ui.Context, t *ui.Theme, text string) {
	ui.Text(c, a.tr(text)).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
}

// iconButton is a button showing only an icon, named by label for assistive
// technology and by its tooltip for the pointer.
func (a *App) iconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) {
	kit.IconButton(c, t, icon, a.tr(label), onClick)
}

// loadAudioDevices selects the default speaker and keeps the microphone off.
// shortcut: endpoints load at startup; add WASAPI notifications for hot-plug refresh.
func (a *App) loadAudioDevices() {
	go func() {
		speakers, microphones, err := a.Rec.AudioDevices()
		a.update(func() {
			a.speakers, a.microphones = speakers, microphones
			if len(speakers) > 0 {
				a.speaker = speakers[0]
			}
			if err != nil {
				a.setError(err)
			}
		})
	}()
}

// audioRow is a full-width device dropdown for one kind of sound.
func (a *App) audioRow(c *ui.Context, t *ui.Theme, selected *capture.AudioDevice, devices []capture.AudioDevice, icon *ui.SVG, label string) {
	options := []kit.SelectOption[capture.AudioDevice]{{Label: a.tr("Off")}}
	for _, device := range devices {
		options = append(options, kit.SelectOption[capture.AudioDevice]{Value: device, Label: device.Name})
	}
	if selected.ID == "" {
		*selected = capture.AudioDevice{}
	}
	kit.Select(c, t, selected, options, a.tr(label), a.tr(label)+a.tr(" device"), icon, func(capture.AudioDevice) {})
}

// sourceCard is one tile of the capture-source grid: the screen and each
// display choose themselves, and a window is chosen below them.
type sourceCard struct {
	label   string
	icon    *ui.SVG
	id      string // the source a click chooses
	chosen  bool
	blocked string // why this source cannot be recorded yet
}

// sourceCardHeight is the height every card of the grid takes, so that the rows
// line up whatever each says.
const sourceCardHeight = 56

// sourceCards lays the sources out two to a row, as large tiles.
func (a *App) sourceCards(c *ui.Context, t *ui.Theme) {
	cards := a.sourceCardsOf(a.sources)
	for i := 0; i < len(cards); i += 2 {
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			for _, card := range cards[i:min(i+2, len(cards))] {
				a.sourceCard(c, t, card)
			}
			if i+2 > len(cards) {
				ui.Spacer(c) // keeps a lone card the width of a column
			}
		})
	}
}

func (a *App) sourceCard(c *ui.Context, t *ui.Theme, card sourceCard) {
	card.label = a.tr(card.label)
	card.blocked = a.tr(card.blocked)
	btn := ui.ButtonBase(c).Label(card.label).Grow(1).MinWidth(0).Height(sourceCardHeight).
		Padding(t.Space(2), t.Space(2)).Radius(t.Radius).BorderWidth(1)
	if card.blocked != "" {
		btn.Disabled(true).Tooltip(card.blocked).TextColor(t.TextMuted)
	} else {
		btn.Cursor(ui.CursorPointer)
	}
	if card.chosen {
		btn.BorderColor(t.Accent).Background(t.Surface)
	} else {
		btn.BorderColor(t.Border)
	}
	btn.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(0.5)).Children(func() {
			picture := ui.Icon(c, card.icon).FontSize(t.FontSize + 5)
			if card.chosen {
				picture.TextColor(t.Accent)
			} else {
				picture.TextColor(t.TextMuted)
			}
			ui.Text(c, card.label).FontSize(t.FontSize - 2).Bold().
				SingleLine().TextAlign(ui.Center)
		})
	})
	if card.blocked == "" && !card.chosen && btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	if card.blocked == "" && btn.Clicked() {
		a.chooseSource(card.id)
	}
}

// sourceSelect chooses a display in full-screen mode or a window in window
// mode. The two modes share one compact dropdown.
func (a *App) sourceSelect(c *ui.Context, t *ui.Theme) {
	if a.sourceMode() == "window" {
		windows := a.sourcesOfKind("window")
		if len(windows) == 0 {
			a.sourceSelectEmpty(c, t, iconWindow, "No visible window to record")
			return
		}
		chosen := a.windowSource()
		options := make([]kit.SelectOption[capture.Source], 0, len(windows))
		for _, window := range windows {
			options = append(options, kit.SelectOption[capture.Source]{Value: window, Label: window.Name})
		}
		kit.Select(c, t, &chosen, options, a.tr("Window"), a.tr("Choose a window"), iconWindow, func(source capture.Source) {
			a.chooseSource(source.ID)
		})
		return
	}
	displays := a.sourcesOfKind("display")
	if len(displays) == 0 {
		displays = a.sourcesOfKind("screen")
	}
	if len(displays) == 0 {
		a.sourceSelectEmpty(c, t, iconScreen, "No display to record")
		return
	}
	chosen := a.displaySource()
	options := make([]kit.SelectOption[capture.Source], 0, len(displays))
	for _, display := range displays {
		options = append(options, kit.SelectOption[capture.Source]{Value: display, Label: display.Name})
	}
	kit.Select(c, t, &chosen, options, a.tr("Display"), a.tr("Choose a display"), iconScreen, func(source capture.Source) {
		a.chooseSource(source.ID)
	})
}

func (a *App) sourceSelectEmpty(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Padding(t.Space(2), t.Space(2.5)).
		Radius(t.Radius).BorderWidth(1).BorderColor(t.Border).Children(func() {
		ui.Icon(c, icon).FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		ui.Text(c, a.tr(label)).FontSize(t.FontSize - 3).TextColor(t.TextMuted).SingleLine()
	})
}

func (a *App) sourceMode() string {
	for _, source := range a.sources {
		if source.ID == a.sourceID && source.Kind == "window" {
			return "window"
		}
	}
	return "screen"
}

// displaySource is the chosen display, or the first available one.
func (a *App) displaySource() capture.Source {
	for _, source := range a.sources {
		if source.ID == a.sourceID && (source.Kind == "display" || source.Kind == "screen") {
			return source
		}
	}
	if displays := a.sourcesOfKind("display"); len(displays) > 0 {
		return displays[0]
	}
	if screens := a.sourcesOfKind("screen"); len(screens) > 0 {
		return screens[0]
	}
	return capture.Source{}
}

// windowSource is the chosen window, or the first available one.
func (a *App) windowSource() capture.Source {
	for _, source := range a.sources {
		if source.ID == a.sourceID && source.Kind == "window" {
			return source
		}
	}
	if windows := a.sourcesOfKind("window"); len(windows) > 0 {
		return windows[0]
	}
	return capture.Source{}
}

// libraryTools are the list's search field and import button.
func (a *App) libraryTools(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
		if ui.SearchField(c, &a.query).Label(a.tr("Search the library")).Height(t.Space(8)).Grow(1).MinWidth(0).Changed() {
			a.reload()
		}
		imp := ui.ButtonBase(c).Label(a.tr("Import")).Tooltip(a.tr("Add a media file to the library")).
			Disabled(a.recording).Size(t.Space(8), t.Space(8)).Radius(t.Radius).Center().TextColor(t.TextMuted)
		imp.Children(func() { ui.Icon(c, iconImport).FontSize(t.FontSize + 4) })
		if imp.Hovered() && !a.recording {
			imp.Background(t.SurfaceHover)
		}
		if imp.Clicked() && !a.recording {
			a.importFiles()
		}
	})
}

// libraryGrid lists media and any orphaned recording files.
func (a *App) libraryGrid(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, t.Space(2), t.Space(4), t.Space(2)).Gap(t.Space(3)).Children(func() {
			if len(a.media) == 0 {
				a.emptyLibrary(c, t)
			} else if a.libraryGridView {
				ui.Row(c).Wrap().Gap(t.Space(3)).AlignItems(ui.Start).Children(func() {
					for _, m := range a.media {
						a.mediaCard(c, t, m)
					}
				})
			} else {
				ui.Column(c).Gap(t.Space(1)).Children(func() {
					for _, m := range a.media {
						a.mediaRow(c, t, m)
					}
				})
			}
			if len(a.orphans) > 0 {
				a.orphanNotice(c, t)
			}
		})
	})
}

// emptyLibrary says the library is empty, or that nothing matched, and offers
// the one thing to do about it.
func (a *App) emptyLibrary(c *ui.Context, t *ui.Theme) {
	searching := strings.TrimSpace(a.query) != ""
	ui.Column(c).FillWidth().Padding(t.Space(12), 0).Center().Gap(t.Space(2)).Children(func() {
		ui.Icon(c, iconLibrary).FontSize(t.FontSize + 18).TextColor(t.TextMuted).Opacity(0.6)
		title := a.tr("No recordings yet")
		detail := a.tr("Record the screen, or import a media file to get started.")
		if searching {
			title = a.trf("No results for “%s”", strings.TrimSpace(a.query))
			detail = a.tr("Try a different search.")
		}
		ui.Text(c, title).FontSize(t.FontSize + 3).Bold()
		ui.Text(c, detail).FontSize(t.FontSize - 1).TextColor(t.TextMuted).TextAlign(ui.Center)
		if !searching {
			if ui.Button(c, a.tr("Import media")).Clicked() {
				a.importFiles()
			}
		}
	})
}

// mediaRow is one recording in the library list.
func (a *App) mediaRow(c *ui.Context, t *ui.Theme, m library.Media) {
	id := m.ID
	card := ui.ButtonBase(c.Key(id)).Label(m.Name).FillWidth().Padding(t.Space(2)).
		Radius(t.Radius).Cursor(ui.CursorPointer).AlignItems(ui.Center)
	if id == a.sel {
		card.Background(t.SurfaceHover)
	} else if card.Hovered() {
		card.Background(t.Surface)
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(3)).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Width(96).Shrink(0).Children(func() { a.mediaThumb(c, t, m, false) })
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
				ui.Text(c, m.Name).FontSize(t.FontSize).Bold().MaxLines(2)
				ui.Text(c, a.cardSubtitle(m)).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine()
			})
		})
	})
	if card.Clicked() {
		a.openDetail(id)
	}
	a.mediaMenu(card, m)
}

const mediaCardWidth = 210

// mediaCard is one tile in grid view.
func (a *App) mediaCard(c *ui.Context, t *ui.Theme, m library.Media) {
	card := ui.ButtonBase(c.Key(m.ID)).Label(m.Name).Width(mediaCardWidth).Padding(t.Space(2)).
		Radius(t.Radius).Cursor(ui.CursorPointer)
	if m.ID == a.sel {
		card.Background(t.SurfaceHover)
	} else if card.Hovered() {
		card.Background(t.Surface)
	}
	card.Children(func() {
		ui.Column(c).Gap(t.Space(2)).Children(func() {
			a.mediaThumb(c, t, m, false)
			ui.Text(c, m.Name).Bold().MaxLines(2)
			ui.Text(c, a.cardSubtitle(m)).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine()
		})
	})
	if card.Clicked() {
		a.openDetail(m.ID)
	}
	a.mediaMenu(card, m)
}

func (a *App) mediaMenu(card ui.Element, m library.Media) {
	card.ContextMenu(func(mn *ui.Menu) {
		if mn.Item(a.tr("Transcript")).Chosen() {
			a.openDetail(m.ID)
			a.tab = 0
		}
		if mn.Item(a.tr("Translate")).Chosen() {
			a.openDetail(m.ID)
			a.tab = 0
			if current, ok := a.current(); ok {
				a.setTranslationVisible(current, true)
			}
		}
		if mn.Item(a.tr("Summary")).Chosen() {
			a.openDetail(m.ID)
			a.tab = 1
		}
		mn.Separator()
		if mn.Item(a.tr("Show the file")).Chosen() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		if mn.Item(a.tr("Delete…")).Chosen() {
			a.delID, a.confirmDel = m.ID, true
		}
	})
}

// mediaThumb shows a decoded video poster or screenshot, with a source glyph
// as a fallback while it loads or when the file cannot be decoded.
func (a *App) mediaThumb(c *ui.Context, t *ui.Theme, m library.Media, chosen bool) {
	thumb := ui.Column(c).FillWidth().AspectRatio(16.0 / 9.0).Radius(t.Radius + 2).
		Clip().Justify(ui.End)
	from, to := mediaBackdrop(t.Dark)
	thumb.Gradient(from, to, 90)
	if chosen {
		thumb.BorderWidth(2).BorderColor(t.Accent)
	}
	thumb.Children(func() {
		if picture := a.thumbs[m.ID]; picture != nil {
			ui.Image(c, picture).Fit(ui.Cover).Grow(1)
		} else {
			switch {
			case m.Source == "screenshot" || !m.HasVideo:
				ui.Column(c).Grow(1).Center().Children(func() {
					ui.Icon(c, iconScreen).FontSize(t.FontSize + 16).TextColor(onMedia()).Opacity(0.7)
				})
			default:
				ui.Column(c).Grow(1).Center().Children(func() {
					ui.Icon(c, iconPlay).FontSize(t.FontSize + 10).TextColor(onMedia()).Opacity(0.8)
				})
			}
		}
		ui.Row(c).Attach(ui.AnchorBottomLeft, ui.AnchorBottomLeft).FillWidth().Gap(t.Space(1)).
			AlignItems(ui.Center).Padding(t.Space(1), t.Space(1.5)).Background(ui.RGBA(10, 13, 20, 0.45)).Children(func() {
			for _, chip := range mediaChips(m) {
				chipLabel(c, t, chip)
			}
			ui.Spacer(c)
			if m.TranscriptStatus == "pending" || m.SummaryStatus == "pending" {
				ui.Text(c, a.tr("Working…")).FontSize(t.FontSize - 3).TextColor(onMedia()).Opacity(0.85)
			} else if m.DurationMs > 0 {
				ui.Text(c, fmtDuration(m.DurationMs)).FontSize(t.FontSize - 3).Bold().
					TextColor(onMedia()).FontFeatures("tnum")
			}
		})
	})
}

// mediaChips say what has been made of a media, as the card's small badges.
func mediaChips(m library.Media) []string {
	var chips []string
	if m.Source == "screenshot" {
		chips = append(chips, "PNG")
	}
	switch m.TranscriptStatus {
	case "done":
		chips = append(chips, "TXT")
	case "error":
		chips = append(chips, "TXT!")
	}
	if m.SummaryStatus == "done" {
		chips = append(chips, "AI")
	}
	return chips
}

// chipLabel is one small badge on a thumbnail: a dark pill with a hairline, so
// that it reads on any picture.
func chipLabel(c *ui.Context, t *ui.Theme, text string) {
	ui.Row(c).Padding(t.Space(0.5), t.Space(1.5)).Radius(t.Space(1.5)).
		Background(ui.RGBA(10, 13, 20, 0.72)).Border(1, ui.RGBA(255, 255, 255, 0.16)).
		Children(func() {
			ui.Text(c, text).FontSize(t.FontSize - 5).Bold().TextColor(onMedia())
		})
}

// cardSubtitle is the line under a card's name: what it is and when it was
// made.
func (a *App) cardSubtitle(m library.Media) string {
	source := a.tr(m.Source)
	if m.Source == "screenshot" {
		source = a.tr("Screenshot")
	}
	return fmt.Sprintf("%s · %s", source, a.dateTime(m.CreatedAt))
}

// orphanNotice says that recordings on disk are not in the library, and offers
// the two things to do about it: add them back, or delete them.
func (a *App) orphanNotice(c *ui.Context, t *ui.Theme) {
	ui.Divider(c)
	ui.Column(c).Padding(t.Space(2.5), t.Space(4), t.Space(3), t.Space(4)).Gap(t.Space(2)).Children(func() {
		n := len(a.orphans)
		said := a.trf("%d recording files are not in the library.", n)
		if n == 1 {
			said = a.tr("1 recording file is not in the library.")
		}
		ui.Text(c, said).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			if ui.Button(c, a.tr("Add back")).Tooltip(a.tr("Add these files to the library again")).Clicked() {
				a.addOrphans()
			}
			if ui.Button(c, a.tr("Delete files…")).Tooltip(a.tr("Delete these files from disk")).Clicked() {
				a.confirmOrphans = true
			}
		})
	})
}

// mediaMeta is the second line of a row: when it was made, how long it runs,
// and what has been done with it.
func mediaMeta(m library.Media) string {
	parts := []string{m.CreatedAt.Local().Format("Jan 2 15:04")}
	if m.Source == "screenshot" {
		parts = append(parts, "screenshot", fmtSize(m.Size))
		return strings.Join(parts, " · ")
	}
	parts = append(parts, fmtDuration(m.DurationMs))
	if m.Source == "recording" {
		parts = append(parts, "recorded")
	}
	switch m.TranscriptStatus {
	case "done":
		parts = append(parts, "transcribed")
	case "pending":
		parts = append(parts, "transcribing")
	case "error":
		parts = append(parts, "transcription failed")
	}
	if m.SummaryStatus == "done" {
		parts = append(parts, "summary")
	}
	return strings.Join(parts, " · ")
}

// sourceCardsOf returns the two capture modes; the dropdown below chooses
// the concrete display or window.
func (a *App) sourceCardsOf(sources []capture.Source) []sourceCard {
	window := a.windowSource()
	windowCard := sourceCard{label: "Window", icon: iconWindow, id: window.ID, chosen: a.sourceMode() == "window"}
	if window.ID == "" {
		windowCard.blocked = "No visible window to record"
	}
	display := a.displaySource()
	displayCard := sourceCard{label: "Full screen", icon: iconScreen, id: display.ID, chosen: a.sourceMode() == "screen"}
	if display.ID == "" {
		displayCard.blocked = "No display to record"
	}
	return []sourceCard{displayCard, windowCard}
}

// sourcesOfKind returns the sources of one kind, in the order the platform
// listed them.
func (a *App) sourcesOfKind(kind string) []capture.Source {
	var list []capture.Source
	for _, s := range a.sources {
		if s.Kind == kind {
			list = append(list, s)
		}
	}
	return list
}

// sourceName is the label of the source chosen, or the kind when it is gone.
func (a *App) sourceName() string {
	for _, s := range a.sources {
		if s.ID == a.sourceID {
			return s.Name
		}
	}
	return a.tr("Window")
}

// sourceDetail is the size the platform reports for the source chosen.
func (a *App) sourceDetail() string {
	for _, s := range a.sources {
		if s.ID == a.sourceID {
			return s.Detail
		}
	}
	return ""
}

// chooseSource makes one source the one recorded, and shows it.
func (a *App) chooseSource(id string) {
	a.sourceID = id
	a.source = a.sourceName()
	a.showPreview()
}

func (a *App) hasSource(id string) bool {
	for _, s := range a.sources {
		if s.ID == id {
			return true
		}
	}
	return false
}

// pickSource chooses a display when one is available, else the first source.
func (a *App) pickSource() {
	if len(a.sources) == 0 {
		a.sourceID = ""
		return
	}
	if display := a.displaySource(); display.ID != "" {
		a.chooseSource(display.ID)
		return
	}
	a.chooseSource(a.sources[0].ID)
}

func (a *App) startRecording() {
	if a.busy || a.recording {
		return
	}
	a.closePlayback()
	a.busy = true
	a.openLibrary() // the live recording stays in this window
	if a.Preview != nil {
		a.Preview.Stop() // the recording draws its own preview
	}
	cfg := capture.Config{
		SourceID: a.sourceID, SystemAudio: a.speaker.ID != "", Mic: a.microphone.ID != "",
		SpeakerID: a.speaker.ID, MicrophoneID: a.microphone.ID,
	}
	go func() {
		err := a.Rec.Start(cfg)
		a.update(func() {
			a.busy = false
			if err != nil {
				a.setError(err)
				a.showPreview()
				return
			}
			st := a.Rec.State()
			a.recording, a.startedAt = st.Recording, st.StartedAt
			a.late, a.preview = nil, nil
			a.err = ""
		})
	}()
}

// stopRecording ends the recording; discard deletes the file it made.
func (a *App) stopRecording(discard bool) {
	if a.busy {
		return
	}
	a.busy = true
	go func() {
		m, err := a.Rec.Stop()
		a.update(func() {
			a.busy = false
			a.recording, a.late, a.preview = false, nil, nil
			if err != nil {
				a.setError(err)
				a.reload()
				return
			}
			if discard {
				a.removeMedia(m.ID, true)
				return
			}
			a.reload()
			a.selectMedia(m.ID)
			a.tab = 0
			a.showPreview() // back to showing the source, not the recording
		})
	}()
}

func (a *App) importFiles() {
	chain, lang := a.importChain, a.trLang
	go func() {
		imported, err := a.Lib.Import(context.Background())
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.reload()
			if len(imported) > 0 {
				a.selectMedia(imported[0].ID)
			}
		})
		for _, m := range imported {
			if m.HasAudio {
				a.startImport(m.ID, chain, lang)
			}
		}
	}()
}

// startImport runs what importing does once the file is in the library: the
// offline transcription recording does too, and when the import asked for it,
// translation into lang and a summary — all as one job, so one Cancel stops
// the chain.
func (a *App) startImport(id int64, chain bool, lang string) {
	if a.Jobs == nil || a.ASR == nil {
		return
	}
	_, err := a.Jobs.Run(id, jobs.KindImport, func(ctx context.Context, progress func(float64)) error {
		share := 1.0
		if chain {
			share = 0.6
		}
		if err := a.ASR.TranscribeNow(ctx, id, func(p float64) { progress(p * share) }); err != nil {
			return err
		}
		if !chain {
			return nil
		}
		progress(0.6)
		// Translating into the language it is already in would spend the
		// provider's quota saying nothing.
		translated := asr.Transcript{Language: lang}
		if tr, err := a.ASR.Get(id); err == nil {
			translated = tr
		}
		if translated.Language != lang {
			if _, err := a.Trans.Translate(ctx, id, lang); err != nil {
				return err
			}
		}
		progress(0.8)
		_, err := a.Sum.Generate(ctx, id)
		return err
	})
	if err != nil {
		a.setError(err)
	}
}

// removeMedia deletes a record, and its file when withFile is set.
func (a *App) removeMedia(id int64, withFile bool) {
	if a.sel == id {
		a.closePlayback()
	}
	go func() {
		err := a.Lib.Remove(id, withFile)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			if a.sel == id {
				a.forget()
			}
			delete(a.thumbs, id)
			a.reload()
		})
	}()
}

// grabPreview decodes the latest frame of what is being recorded, or of what
// will be, into the bitmap — at most five times a second: the frame is a JPEG.
func (a *App) grabPreview() {
	if time.Since(a.previewAt) < 200*time.Millisecond {
		return
	}
	var frame []byte
	if a.recording {
		frame = a.Rec.Preview()
	} else if a.Preview != nil {
		frame = a.Preview.Frame()
	}
	if len(frame) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		return
	}
	a.previewAt = time.Now()
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if a.preview == nil || w != a.previewW || h != a.previewH {
		// A new source, or a new recording: the size may have changed.
		a.preview = ui.NewBitmap(img)
		a.previewW, a.previewH = w, h
		return
	}
	a.preview.Update(img)
}

// addOrphans puts the files Ottr made back in the library.
func (a *App) addOrphans() {
	paths := a.orphans
	go func() {
		_, err := a.Lib.AddFiles(paths)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.reload()
		})
	}()
}

// deleteOrphans deletes the files Ottr made and nothing else; the records are
// already gone.
func (a *App) deleteOrphans() {
	paths := a.orphans
	go func() {
		err := a.Lib.DeleteFiles(paths)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			a.reload()
		})
	}()
}
