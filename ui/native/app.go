// Package native is Ottr's window: the library of recordings and imported
// files on the left, and beside it what the app makes of the one chosen —
// its transcript, its translation and its summary — or the recording in
// progress, live. It is drawn by MyGo's own toolkit: no web page, no HTML.
package native

import (
	"fmt"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/library"
	"ottr/internal/summary"
	"ottr/internal/translate"
)

// App is the app's state, which the view reads, and the window it draws.
type App struct {
	Lib   *library.Service
	Rec   *capture.Recorder
	ASR   *asr.Service
	Trans *translate.Service
	Sum   *summary.Service

	win *mygo.Window

	// The library.
	media []library.Media
	query string
	sel   int64
	tab   int
	err   string

	// The recording pane.
	sources  []capture.Source
	sourceID string
	source   string // the label of sourceID
	sysAudio bool
	micAudio bool
	busy     bool
	discard  bool

	// The recording in progress, mirrored from the recorder each frame.
	recording bool
	startedAt time.Time
	liveText  string
	late      []liveLine
	preview   *ui.Bitmap
	previewW  int
	previewH  int
	previewAt time.Time

	// The session shown, and its three tabs.
	transcript   asr.Transcript
	segs         []asr.Segment
	trLoaded     bool
	trLang       string
	trans        translate.Translation
	transLang    string
	transLoading bool
	transPending bool
	sum          summary.Summary
	sumLoaded    bool
	sumPending   bool
	segQuery     string

	// Settings drawers, read when opened.
	transOpen   bool
	transCfg    translate.Settings
	loadedTrans bool
	sumOpen     bool
	sumCfg      summary.Settings
	loadedSum   bool

	confirmDel bool
	delID      int64

	// queued functions run on the next frame, for work done elsewhere: the
	// window's own Update does that for a window, and a test has none.
	queueMu sync.Mutex
	queue   []func()
}

// liveLine is one live subtitle the recording pane shows.
type liveLine struct {
	at   int64
	text string
}

// View returns the window's content.
func View(a *App) *ui.Content { return ui.View(a.view) }

// Begin loads what the window shows.
func (a *App) Begin(win *mygo.Window) {
	a.win = win
	a.trLang = "en"
	a.sysAudio = true
	a.reload()
	if srcs, err := a.Rec.Sources(); err != nil {
		a.setError(err)
	} else {
		a.sources = srcs
		a.pickSource()
	}
}

// ---------------------------------------------------------------- the view

func (a *App) view(c *ui.Context) {
	t := c.Theme()
	a.drain()
	a.mirror(c)
	a.poll(c)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, t)
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
	if a.err != "" {
		a.banner(c, t)
	}
	a.dialogs(c, t)
}

// update runs fn in a frame, from any goroutine: through the window when
// there is one, else on the next frame the view builds.
func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
		return
	}
	a.queueMu.Lock()
	a.queue = append(a.queue, fn)
	a.queueMu.Unlock()
}

// drain runs what other goroutines queued for this frame.
func (a *App) drain() {
	for {
		a.queueMu.Lock()
		if len(a.queue) == 0 {
			a.queueMu.Unlock()
			return
		}
		fn := a.queue[0]
		a.queue = a.queue[1:]
		a.queueMu.Unlock()
		fn()
	}
}

// mirror copies what the recorder and the services did elsewhere into the
// view's state, and keeps frames coming while something moves.
func (a *App) mirror(c *ui.Context) {
	st := a.Rec.State()
	switch {
	case a.recording && !st.Recording && !a.busy:
		// Stopped by something other than this window's Stop button.
		a.recording, a.late, a.preview = false, nil, nil
		a.reload()
		a.showLatest()
	case st.Recording && !a.recording:
		a.recording, a.startedAt, a.late, a.preview = true, st.StartedAt, nil, nil
	}
	if !a.recording {
		return
	}
	a.startedAt = st.StartedAt
	if st.LiveText != "" && (len(a.late) == 0 || a.late[len(a.late)-1].text != st.LiveText) {
		a.late = append(a.late, liveLine{at: time.Since(st.StartedAt).Milliseconds(), text: st.LiveText})
		if len(a.late) > 6 {
			a.late = a.late[len(a.late)-6:]
		}
	}
	a.grabPreview()
	c.After(150 * time.Millisecond)
}

// poll keeps frames coming while a job the window shows is still running, so
// that its end is seen: the services announce theirs to pages, and a window
// of native UI has none.
func (a *App) poll(c *ui.Context) {
	if !a.recording && a.pending() {
		c.After(400 * time.Millisecond)
	}
}

// showLatest shows the newest recording once the recorder stopped.
func (a *App) showLatest() {
	if len(a.media) > 0 {
		a.selectMedia(a.media[0].ID)
	}
}

func (a *App) current() (library.Media, bool) { return a.byID(a.sel) }

func (a *App) byID(id int64) (library.Media, bool) {
	for _, m := range a.media {
		if m.ID == id {
			return m, true
		}
	}
	return library.Media{}, false
}

func (a *App) setError(err error) {
	if err == nil {
		a.err = ""
		return
	}
	a.err = err.Error()
}

// reload reads the library as the search asks, and follows the media shown:
// a transcript that finished, a summary that changed.
func (a *App) reload() {
	if a.Lib == nil {
		return
	}
	items, err := a.Lib.List(a.query)
	if err != nil {
		a.setError(err)
		return
	}
	a.media = items
	if _, ok := a.current(); !ok {
		a.forget()
	}
	if a.sel == 0 && len(items) > 0 {
		a.selectMedia(items[0].ID)
	}
	if m, ok := a.current(); ok {
		if m.TranscriptStatus == "done" && a.trLoaded && len(a.segs) == 0 {
			a.trLoaded = false // a transcript arrived: read it again
		}
		if m.SummaryStatus == "done" && a.sumLoaded && a.sum.ContentMarkdown == "" {
			a.sumLoaded = false
		}
	}
}

func (a *App) selectMedia(id int64) {
	if a.sel == id {
		return
	}
	a.forget()
	a.sel = id
}

// forget drops what was loaded for the media shown.
func (a *App) forget() {
	a.sel = 0
	a.transcript, a.segs = asr.Transcript{}, nil
	a.trLoaded, a.sumLoaded = false, false
	a.trans, a.sum = translate.Translation{}, summary.Summary{}
	a.transLang, a.segQuery = "", ""
	a.transPending, a.sumPending = false, false
}

// pending reports whether something the window shows is still being worked
// on, so that the window asks for frames until it settles.
func (a *App) pending() bool {
	if a.recording || a.transPending || a.sumPending || a.busy {
		return true
	}
	m, ok := a.current()
	if !ok {
		return false
	}
	if m.TranscriptStatus == "pending" || m.SummaryStatus == "pending" {
		return true
	}
	if a.tab == 0 && !a.trLoaded && m.TranscriptStatus == "done" {
		return true
	}
	if a.tab == 2 && !a.sumLoaded && m.SummaryStatus == "done" {
		return true
	}
	return false
}

func (a *App) dialogs(c *ui.Context, t *ui.Theme) {
	if a.discard {
		ui.Modal(c, &a.discard, func() {
			ui.Text(c, "Delete this recording?").Bold()
			ui.Text(c, "The file is deleted from disk, and the library keeps nothing.").
				TextColor(t.TextMuted).FontSize(t.FontSize - 1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Keep recording").Clicked() {
					a.discard = false
				}
				if ui.PrimaryButton(c, "Delete").Clicked() {
					a.discard = false
					a.stopRecording(true)
				}
			})
		})
	}
	if a.confirmDel {
		ui.Modal(c, &a.confirmDel, func() {
			name := "this file"
			if m, ok := a.byID(a.delID); ok {
				name = fmt.Sprintf("%q", m.Name)
			}
			ui.Text(c, "Delete "+name+"?").Bold()
			keep := true
			ui.Checkbox(c, &keep, "Keep the file on disk").Changed()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.confirmDel = false
				}
				if ui.PrimaryButton(c, "Delete").Clicked() {
					a.confirmDel = false
					a.removeMedia(a.delID, !keep)
				}
			})
		})
	}
}

func (a *App) banner(c *ui.Context, t *ui.Theme) {
	ui.Overlay(c, func() {
		ui.Row(c).Attach(ui.AnchorTop, ui.AnchorTop).Top(t.Space(3)).MaxWidth(680).
			Padding(t.Space(2.5), t.Space(4)).Radius(t.Radius).Gap(t.Space(3)).
			AlignItems(ui.Center).Background(t.Danger).TextColor(t.AccentText).Children(func() {
			ui.Text(c, a.err).FontSize(t.FontSize - 1).Grow(1).Selectable()
			if ui.Button(c, "Dismiss").Clicked() {
				a.err = ""
			}
		})
	})
}

func (a *App) emptyPage(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Center().Gap(t.Space(2)).Children(func() {
		ui.Text(c, "Nothing chosen").FontSize(t.FontSize + 4).Bold()
		ui.Text(c, "Record the screen, import a file, or choose one on the left.").
			TextColor(t.TextMuted)
		if len(a.sources) > 0 && !a.recording {
			if ui.PrimaryButton(c, "Start recording").Clicked() {
				a.startRecording()
			}
		}
	})
}

func fmtDuration(ms int64) string {
	if ms <= 0 {
		return "0:00"
	}
	s := int(ms / 1000)
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}
