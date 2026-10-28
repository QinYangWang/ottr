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
)

// sidebar is the recording form, with Library and Settings at the bottom. It is
// the whole main window.
func (a *App) sidebar(c *ui.Context, t *ui.Theme) {
	side := ui.Column(c).FillHeight().Grow(1).Background(t.Surface)
	side.Children(func() {
		// The cards and picture scroll in a short window; the recording
		// controls and window buttons stay in reach.
		ui.Scroll(c).MaxHeight(formHeight).Children(func() {
			a.recordForm(c, t)
		})
		a.audioBar(c, t)
		a.startBar(c, t)
		ui.Spacer(c)
		ui.Divider(c)
		a.sidebarFooter(c, t)
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
			ui.Text(c, "Choosing another file is off until it stops.").
				FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			return
		}
		if len(a.sources) == 0 {
			ui.Text(c, "Nothing can be recorded on this system yet.").
				FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			return
		}
		a.caption(c, t, "Choose a capture source")
		a.sourceCards(c, t)
		a.windowSelect(c, t)
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
// in reach.
func (a *App) startBar(c *ui.Context, t *ui.Theme) {
	if a.recording || len(a.sources) == 0 {
		return
	}
	ui.Column(c).Padding(t.Space(2.5), t.Space(4), t.Space(3), t.Space(4)).Gap(t.Space(2)).Children(func() {
		if ui.PrimaryButton(c, "Start recording").Disabled(a.busy).Clicked() {
			a.startRecording()
		}
		if a.busy {
			ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Label("Starting")
				ui.Text(c, "Starting the recorder…").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			})
		}
	})
}

// sidebarFooter opens the secondary windows and toggles the global theme.
func (a *App) sidebarFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(1)).Justify(ui.End).Children(func() {
		dark := a.themeSource() == "dark" || a.themeSource() == "system" && t.Dark
		if dark {
			a.iconButton(c, t, iconSun, "Light theme", func() { a.setTheme("light") })
		} else {
			a.iconButton(c, t, iconMoon, "Dark theme", func() { a.setTheme("dark") })
		}
		a.iconButton(c, t, iconLibrary, "Library", a.openLibrary)
		a.iconButton(c, t, iconSliders, "Settings", a.openSettings)
	})
}

// openLibrary opens the media list and its details independently of the main
// recording form. Recording also lands here.
func (a *App) openLibrary() {
	if a.libraryWin != nil && !a.libraryWin.IsDestroyed() {
		a.libraryWin.Focus()
		return
	}
	a.reload()
	a.libraryWin = mygo.NewWindow(mygo.WindowOptions{
		Title: "Ottr Library", Width: WideWidth, Height: WideHeight,
		MinWidth: 900, MinHeight: 600, Parent: a.win,
		TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: titleBarHeight,
		BackgroundColor: themeBackground(), Content: ui.View(a.libraryView),
	})
	a.libraryWin.OnClosed(func() { a.libraryWin = nil })
}

// libraryView puts recordings on the left and the selected recording, or the
// live recording, on the right.
func (a *App) libraryView(c *ui.Context) {
	t := c.Theme()
	a.drain()
	a.mirror(c)
	a.poll(c)
	ui.Column(c).Fill().Children(func() {
		a.windowTitleBar(c, t, "Ottr")
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Background(t.Surface).
				BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
				a.libraryPane(c, t)
			})
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				switch {
				case a.recording:
					a.recorderPage(c, t)
				default:
					if m, ok := a.current(); ok {
						a.sessionPage(c, t, m)
					} else {
						a.emptyPage(c, t)
					}
				}
			})
		})
	})
	if a.err != "" {
		a.banner(c, t)
	}
	a.dialogs(c, t)
}

// sourcePreviewHeight gives the source room enough to read its contents.
const sourcePreviewHeight = 192

// sourcePreview shows the source before it is recorded — the frames the
// previewer draws — or what keeps them away.
func (a *App) sourcePreview(c *ui.Context, t *ui.Theme) {
	height := float32(sourcePreviewHeight)
	if a.previewErr != "" {
		height = 168 // room for what FFmpeg said
	}
	box := ui.Column(c).Height(height).Radius(t.Radius).
		BorderWidth(1).BorderColor(t.Border).Background(t.Background).Clip()
	box.Children(func() {
		if a.preview != nil {
			ui.Image(c, a.preview).Fit(ui.Cover).Grow(1)
			return
		}
		ui.Column(c).Grow(1).Center().Padding(t.Space(2)).Gap(t.Space(1)).Children(func() {
			ui.Icon(c, iconScreen).FontSize(t.FontSize + 5).TextColor(t.TextMuted)
			label := a.sourceName()
			if detail := a.sourceDetail(); detail != "" {
				label += " · " + detail
			}
			ui.Text(c, label).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine()
			if a.previewErr == "" {
				ui.Text(c, "Waiting for the first frame…").FontSize(t.FontSize - 3).
					TextColor(t.TextMuted).SingleLine().TextAlign(ui.Center)
				return
			}
			// What FFmpeg said, whole: it is the only clue there is.
			ui.Text(c, a.previewErr).FontSize(t.FontSize - 3).TextColor(t.TextMuted).
				MaxWidth(300).TextAlign(ui.Center).Selectable()
			if ui.Button(c, "Try again").Clicked() {
				a.showPreview()
			}
		})
	})
}

// caption names what the controls under it choose.
func (a *App) caption(c *ui.Context, t *ui.Theme, text string) {
	ui.Text(c, text).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
}

// iconButton is a button showing only an icon, named by label for assistive
// technology and by its tooltip for the pointer.
func (a *App) iconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, onClick func()) {
	btn := ui.ButtonBase(c).Label(label).Tooltip(label).TextColor(t.TextMuted).
		Padding(t.Space(1.5)).Radius(t.Radius)
	btn.Children(func() { ui.Icon(c, icon).FontSize(t.FontSize + 3) })
	if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	if btn.Clicked() {
		onClick()
	}
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
	sel := ui.SelectBase(c, selected)
	sel.Trigger.Label(label+" device").Tooltip(label).Grow(1).MinWidth(0).Padding(t.Space(2), t.Space(2.5)).
		Radius(t.Radius).Border(1, t.Border).Background(t.Surface).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		picture := ui.Icon(c, icon).FontSize(t.FontSize + 2).Shrink(0)
		if selected.ID == "" {
			picture.TextColor(t.TextMuted)
		}
		ui.Text(c, label).FontSize(t.FontSize - 3).Shrink(0)
		name := selected.Name
		if selected.ID == "" {
			name = "Off"
		}
		ui.Text(c, name).FontSize(t.FontSize - 3).TextColor(t.TextMuted).
			SingleLine().Grow(1).MinWidth(0).TextAlign(ui.End)
		ui.Text(c, "▾").TextColor(t.TextMuted).Shrink(0)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(t.Space(1)).Radius(t.Radius+2).Background(t.Background).
			Border(1, t.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		options := append([]capture.AudioDevice{{Name: "Off"}}, devices...)
		for _, device := range options {
			item := sel.Item(device).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			} else if device == *selected {
				item.Background(t.Surface)
			}
			item.Children(func() { ui.Text(c, device.Name).FontSize(t.FontSize - 2).SingleLine() })
		}
	})
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

// windowSelect chooses one of the open windows. Windows come and go, so the
// list is read again each time the dropdown opens.
func (a *App) windowSelect(c *ui.Context, t *ui.Theme) {
	windows := a.sourcesOfKind("window")
	if len(windows) == 0 {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Padding(t.Space(2), t.Space(2.5)).
			Radius(t.Radius).BorderWidth(1).BorderColor(t.Border).Children(func() {
			ui.Icon(c, iconWindow).FontSize(t.FontSize + 2).TextColor(t.TextMuted)
			ui.Text(c, "No visible window to record").FontSize(t.FontSize - 3).
				TextColor(t.TextMuted).SingleLine()
		})
		return
	}
	chosen := a.windowSource()
	sel := ui.SelectBase(c, &chosen)
	sel.Trigger.Label("Window").Tooltip("Record one window").Grow(1).MinWidth(0).
		Padding(t.Space(2), t.Space(2.5)).Radius(t.Radius).Border(1, t.Border).
		Background(t.Surface).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Icon(c, iconWindow).FontSize(t.FontSize + 2).Shrink(0).TextColor(t.TextMuted)
		ui.Text(c, "Window").FontSize(t.FontSize - 3).Shrink(0)
		ui.Text(c, chosen.Name).FontSize(t.FontSize - 3).TextColor(t.TextMuted).
			SingleLine().Grow(1).MinWidth(0).TextAlign(ui.End)
		ui.Text(c, "▾").TextColor(t.TextMuted).Shrink(0)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(t.Space(1)).Radius(t.Radius+2).Background(t.Background).
			Border(1, t.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		for _, window := range windows {
			item := sel.Item(window).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			} else if window.ID == chosen.ID {
				item.Background(t.Surface)
			}
			item.Children(func() { ui.Text(c, window.Name).FontSize(t.FontSize - 2).SingleLine() })
		}
	})
	if sel.Trigger.Changed() {
		a.chooseSource(chosen.ID)
	}
}

// windowSource is the window chosen, or the first one where none is yet.
func (a *App) windowSource() capture.Source {
	windows := a.sourcesOfKind("window")
	for _, w := range windows {
		if w.ID == a.sourceID {
			return w
		}
	}
	return windows[0]
}

// libraryPane searches the library and lists it, in the room the form leaves.
func (a *App) libraryPane(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(t.Space(4), t.Space(4), t.Space(2), t.Space(4)).Gap(t.Space(2)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Text(c, "Library").FontSize(t.FontSize - 1).Bold().TextColor(t.TextMuted).Grow(1)
				// The one action of the header is a button with an icon alone:
				// the name is in its label and tooltip.
				imp := ui.ButtonBase(c).Label("Import").Tooltip("Add a media file to the library").
					Disabled(a.recording).Padding(t.Space(1.5)).Radius(t.Radius).TextColor(t.TextMuted)
				imp.Children(func() { ui.Icon(c, iconImport).FontSize(t.FontSize + 3) })
				if imp.Hovered() && !a.recording {
					imp.Background(t.SurfaceHover)
				}
				if imp.Clicked() && !a.recording {
					a.importFiles()
				}
			})
			if ui.SearchField(c, &a.query).Label("Search the library").Changed() {
				a.reload()
			}
			ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Translate + summarize imports").FontSize(t.FontSize - 3).
					TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine()
				ui.Switch(c, &a.importChain).Label("Translate and summarize imports").Changed()
			})
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(0, t.Space(2), t.Space(3), t.Space(2)).Gap(1).Children(func() {
				if len(a.media) == 0 {
					ui.Text(c, a.emptyList()).FontSize(t.FontSize-1).TextColor(t.TextMuted).
						Padding(t.Space(2), t.Space(3))
				}
				for _, m := range a.media {
					a.mediaRow(c, t, m)
				}
			})
		})
		if len(a.orphans) > 0 {
			a.orphanNotice(c, t)
		}
	})
}

// orphanNotice says that recordings on disk are not in the library, and offers
// the two things to do about it: add them back, or delete them.
func (a *App) orphanNotice(c *ui.Context, t *ui.Theme) {
	ui.Divider(c)
	ui.Column(c).Padding(t.Space(2.5), t.Space(4), t.Space(3), t.Space(4)).Gap(t.Space(2)).Children(func() {
		n := len(a.orphans)
		said := fmt.Sprintf("%d recording files are not in the library.", n)
		if n == 1 {
			said = "1 recording file is not in the library."
		}
		ui.Text(c, said).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			if ui.Button(c, "Add back").Tooltip("Add these files to the library again").Clicked() {
				a.addOrphans()
			}
			if ui.Button(c, "Delete files…").Tooltip("Delete these files from disk").Clicked() {
				a.confirmOrphans = true
			}
		})
	})
}

func (a *App) emptyList() string {
	if strings.TrimSpace(a.query) != "" {
		return "Nothing matches that search."
	}
	return "Nothing yet: record the screen, or import a file."
}

func (a *App) mediaRow(c *ui.Context, t *ui.Theme, m library.Media) {
	id := m.ID
	row := ui.Row(c.Key(id)).Gap(t.Space(2.5)).Padding(t.Space(2), t.Space(3)).
		Radius(t.Radius).Cursor(ui.CursorPointer)
	chosen := id == a.sel && !a.recording
	switch {
	case chosen:
		row.Background(t.Accent).TextColor(t.AccentText)
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, m.Name).SingleLine().FontSize(t.FontSize - 1)
			meta := ui.Text(c, mediaMeta(m)).SingleLine().FontSize(t.FontSize - 3)
			if chosen {
				meta.TextColor(t.AccentText.Alpha(0.75))
			} else {
				meta.TextColor(t.TextMuted)
			}
		})
		if m.TranscriptStatus == "pending" || m.SummaryStatus == "pending" {
			ui.Spinner(c).Label("Working")
		} else if chosen {
			ui.Badge(c, fmtDuration(m.DurationMs))
		}
	})
	if row.Clicked() {
		a.selectMedia(id)
	}
	row.ContextMenu(func(mn *ui.Menu) {
		if mn.Item("Transcript").Chosen() {
			a.selectMedia(id)
			a.tab = 0
		}
		if mn.Item("Translate").Chosen() {
			a.selectMedia(id)
			a.tab = 1
		}
		if mn.Item("Summary").Chosen() {
			a.selectMedia(id)
			a.tab = 2
		}
		mn.Separator()
		if mn.Item("Show the file").Chosen() {
			mygo.Shell.ShowItemInFolder(m.Path)
		}
		if mn.Item("Delete…").Chosen() {
			a.delID, a.confirmDel = id, true
		}
	})
}

// mediaMeta is the second line of a row: when it was made, how long it runs,
// and what has been done with it.
func mediaMeta(m library.Media) string {
	parts := []string{m.CreatedAt.Local().Format("Jan 2 15:04"), fmtDuration(m.DurationMs)}
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

// sourceCardsOf is the grid over the sources: the screen, each display, and
// the two kinds that are not implemented yet. A single window is chosen in the
// dropdown under the grid instead.
func (a *App) sourceCardsOf(sources []capture.Source) []sourceCard {
	var cards []sourceCard
	for _, s := range sources {
		switch s.Kind {
		case "screen":
			cards = append(cards, sourceCard{
				label: "Full screen", icon: iconScreen,
				id: s.ID, chosen: a.sourceID == s.ID,
			})
		case "display":
			cards = append(cards, sourceCard{
				label: s.Name, icon: iconScreen,
				id: s.ID, chosen: a.sourceID == s.ID,
			})
		}
	}
	cards = append(cards,
		sourceCard{label: "Area", icon: iconArea,
			blocked: "Recording part of the screen is not implemented yet"},
		sourceCard{label: "Camera only", icon: iconCamera,
			blocked: "Recording a camera is not implemented yet"},
	)
	return cards
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
	return "Window"
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

// pickSource chooses the whole screen when it can, else the first source.
func (a *App) pickSource() {
	if len(a.sources) == 0 {
		a.sourceID = ""
		return
	}
	chosen := a.sources[0]
	for _, s := range a.sources {
		if s.Kind == "screen" {
			chosen = s
			break
		}
	}
	a.chooseSource(chosen.ID)
}

func (a *App) startRecording() {
	if a.busy || a.recording {
		return
	}
	a.busy = true
	a.openLibrary() // the live recording and its transcript land here
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
	go func() {
		err := a.Lib.Remove(id, withFile)
		a.update(func() {
			if err != nil {
				a.setError(err)
			}
			if a.sel == id {
				a.forget()
			}
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
