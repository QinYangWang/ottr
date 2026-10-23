package native

import (
	"bytes"
	"context"
	"image"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/capture"
	"ottr/internal/library"
)

// sidebar holds the library and what adds to it. Recording happens here, so
// that what it records and what is imported land in the one list.
func (a *App) sidebar(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(320).Shrink(0).FillHeight().Background(t.Surface).
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
		a.recordPane(c, t)
		ui.Divider(c)
		a.libraryPane(c, t)
	})
}

// recordPane chooses what to record, and starts it; while recording it shows
// how long it has run and how to end it.
func (a *App) recordPane(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Padding(t.Space(4)).Gap(t.Space(2.5)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Ottr").FontSize(t.FontSize + 5).Bold().Grow(1)
			if a.recording {
				ui.Badge(c, "Recording")
			}
		})
		if a.recording {
			a.recordingControls(c, t)
			return
		}
		names, ids := a.sourceOptions()
		if len(names) == 0 {
			ui.Text(c, "Nothing can be recorded on this system yet.").
				FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			return
		}
		if a.source == "" {
			a.source = names[0]
		}
		ui.Select(c, &a.source, names).Label("Record from").Disabled(a.busy)
		for i, name := range names {
			if name == a.source {
				a.sourceID = ids[i]
			}
		}
		ui.Row(c).Gap(t.Space(4)).Children(func() {
			ui.Checkbox(c, &a.sysAudio, "System audio")
			ui.Checkbox(c, &a.micAudio, "Microphone")
		})
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

// recordingControls tells how long the recording has run; the page beside
// the library has the buttons that end it.
func (a *App) recordingControls(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(2.5)).AlignItems(ui.Center).Children(func() {
		dot := ui.Box(c).Size(9, 9).Radius(5).Background(t.Danger)
		dot.Opacity(float32(0.35 + 0.65*dot.Loop("pulse", 1100*time.Millisecond, ui.EaseInOut)))
		ui.Text(c, fmtDuration(time.Since(a.startedAt).Milliseconds())).FontFeatures("tnum").Bold()
		ui.Text(c, "recording").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		ui.Spacer(c)
	})
	ui.Text(c, "Choosing another file is off until it stops.").
		FontSize(t.FontSize - 2).TextColor(t.TextMuted)
}

// libraryPane searches the library and lists it.
func (a *App) libraryPane(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(t.Space(4), t.Space(4), t.Space(2), t.Space(4)).Gap(t.Space(2)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Library").FontSize(t.FontSize - 1).Bold().TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "Import…").Tooltip("Add a media file to the library").
					Disabled(a.recording).Clicked() {
					a.importFiles()
				}
			})
			if ui.SearchField(c, &a.query).Label("Search the library").Changed() {
				a.reload()
			}
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

// sourceOptions returns the recordable sources as labels and IDs, grouped by
// kind. A single display is the screen, so it is left out.
func (a *App) sourceOptions() (names, ids []string) {
	labels := map[string]string{"screen": "Screen", "display": "Display", "window": "Window"}
	for _, kind := range []string{"screen", "display", "window"} {
		var group []capture.Source
		for _, s := range a.sources {
			if s.Kind == kind {
				group = append(group, s)
			}
		}
		if kind == "display" && len(group) < 2 {
			continue
		}
		for _, s := range group {
			names = append(names, labels[kind]+" · "+s.Name)
			ids = append(ids, s.ID)
		}
	}
	return names, ids
}

// pickSource chooses the whole screen when it can, else the first source.
func (a *App) pickSource() {
	names, ids := a.sourceOptions()
	if len(names) == 0 {
		return
	}
	a.source, a.sourceID = names[0], ids[0]
}

func (a *App) startRecording() {
	if a.busy || a.recording {
		return
	}
	a.busy = true
	cfg := capture.Config{SourceID: a.sourceID, SystemAudio: a.sysAudio, Mic: a.micAudio}
	go func() {
		err := a.Rec.Start(cfg)
		a.update(func() {
			a.busy = false
			if err != nil {
				a.setError(err)
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
		})
	}()
}

func (a *App) importFiles() {
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
	}()
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

// grabPreview decodes the recorder's latest preview frame into the bitmap,
// at most five times a second: an MJPEG frame is a JPEG.
func (a *App) grabPreview() {
	if time.Since(a.previewAt) < 200*time.Millisecond {
		return
	}
	frame := a.Rec.Preview()
	if len(frame) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		return
	}
	a.previewAt = time.Now()
	if a.preview == nil {
		a.preview = ui.NewBitmap(img)
		a.previewW, a.previewH = img.Bounds().Dx(), img.Bounds().Dy()
		return
	}
	a.preview.Update(img)
}
