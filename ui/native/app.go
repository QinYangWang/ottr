// Package native draws Ottr's recording, Library and Settings windows with
// MyGo's own toolkit: no web page, no HTML.
package native

import (
	"fmt"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/capture"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/settings"
	"ottr/internal/summary"
	"ottr/internal/translate"
)

// App is the app's state, which the view reads, and the window it draws.
type App struct {
	Lib     *library.Service
	Rec     *capture.Recorder
	ASR     *asr.Service
	Trans   *translate.Service
	Sum     *summary.Service
	Jobs    *jobs.Service
	MM      *models.Manager
	Preview *capture.Previewer // the picture of the source, before recording
	Prefs   *settings.Service  // what the window itself remembers

	win         *mygo.Window
	libraryWin  *mygo.Window
	settingsWin *mygo.Window

	// wide is whether the main window shows the source picker beside the form.
	wide  bool
	theme string // system | light | dark

	// The library.
	media []library.Media
	query string
	sel   int64
	tab   int
	err   string

	// The recording pane.
	sources     []capture.Source
	sourceID    string
	source      string // the label of sourceID
	speakers    []capture.AudioDevice
	microphones []capture.AudioDevice
	speaker     capture.AudioDevice // zero ID records no system audio
	microphone  capture.AudioDevice // zero ID records no microphone
	busy        bool
	previewErr  string

	// What importing does beyond probing: transcription always, translation
	// into trLang and a summary only when asked for.
	importChain bool
	discard     bool

	// The recording in progress, mirrored from the recorder each frame.
	recording  bool
	startedAt  time.Time
	liveText   string
	late       []liveLine
	preview    *ui.Bitmap
	previewW   int
	previewH   int
	previewSeq uint64 // the previewer frame decoded last
	previewAt  time.Time

	// The session shown, and its three tabs.
	transcript   asr.Transcript
	segs         []asr.Segment
	trLoaded     bool
	lang         string // the language the transcript says it is in
	trLang       string
	trans        translate.Translation
	transLang    string
	transLoading bool
	sum          summary.Summary
	sumLoaded    bool
	segQuery     string

	// job is background work over the media shown: what it does, how far it
	// has come, and what became of it.
	job jobs.Job

	// The page beside the library: "" is the session of the media shown,
	// "sources" the picker for a kind of source.
	page string

	// The source picker.
	pickKind  string
	pickQuery string

	// The Settings window and what it shows.
	settingsTab  int
	ffmpeg       string // the program found, or ""
	ffmpegNote   string // where it is, or why it is not
	modelList    []models.Info
	modelInUse   string
	modelBin     string
	modelBinErr  string
	modelLoaded  bool
	modelLoading bool

	// Provider settings, read when the Settings window opens them.
	transCfg    translate.Settings
	loadedTrans bool
	sumCfg      summary.Settings
	loadedSum   bool

	confirmDel bool
	delID      int64

	// orphans are files in the recordings folder that no record holds.
	orphans        []string
	confirmOrphans bool

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
	if a.Prefs != nil {
		if source := a.Prefs.Get(settings.Theme); source != "" {
			a.theme = source
		}
	}
	a.reload()
	a.loadAudioDevices()
	if srcs, err := a.Rec.Sources(); err != nil {
		a.setError(err)
	} else {
		a.sources = srcs
		a.pickSource()
		a.showPreview()
	}
}

// showPreview draws the source chosen, so that what will be recorded is visible
// before it is.
func (a *App) showPreview() {
	if a.Preview == nil || a.sourceID == "" {
		return
	}
	a.previewErr = ""
	a.Preview.Show(a.sourceID)
}

// The room the window takes: the recording form alone, and beside it the page
// of what was chosen.
const (
	CompactWidth  = 420
	CompactHeight = 800
	WideWidth     = 1180
	WideHeight    = 800
)

// openPane shows the source picker beside the recording form.
func (a *App) openPane() {
	if a.wide {
		return
	}
	a.wide = true
	a.resize()
}

// closePane returns the main window to the recording form alone.
func (a *App) closePane() {
	if !a.wide {
		return
	}
	a.wide, a.page = false, ""
	a.resize()
}

func (a *App) resize() {
	if a.win == nil {
		return
	}
	if a.wide {
		a.win.SetSize(WideWidth, WideHeight)
		return
	}
	a.win.SetSize(CompactWidth, CompactHeight)
}

// ---------------------------------------------------------------- the view

// titleBarHeight is the strip the window controls sit in. The window is created
// with a hidden title bar of this height (mygo.WindowOptions), and the view
// draws its own bar there.
const titleBarHeight = 46

// sidebarWidth is the column the library and the recording form take.
const sidebarWidth = 320

func (a *App) view(c *ui.Context) {
	t := c.Theme()
	a.drain()
	a.mirror(c)
	a.poll(c)
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c, t)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, t)
			if a.wide && a.page == "sources" {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					a.sourcesPage(c, t)
				})
			}
		})
	})
	if a.err != "" {
		a.banner(c, t)
	}
}

// titleBar is the window's own title bar, in the strip the window controls
// sit in: the app's name and what it is doing over the library's column, and
// drag room everywhere else. The controls themselves are the system's, at the
// far right, so the bar keeps their room free (ui.TitleBar).
func (a *App) titleBar(c *ui.Context, t *ui.Theme) {
	bar := c.TitleBar()
	height := max(bar.Height, float32(titleBarHeight))

	if !a.wide {
		// The form alone: the whole bar drags the window, its right end left
		// clear for the controls.
		row := ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).
			Padding(0, bar.Right+t.Space(2), 0, t.Space(4)+bar.Left).
			Background(t.Surface).DragWindow()
		row.Children(func() {
			ui.Text(c, "Ottr").FontSize(t.FontSize + 3).Bold()
			a.recordingPill(c, t)
		})
		return
	}

	// Beside a page: the name sits over the library's column, and the rest is
	// drag room up to the controls.
	ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).
		Padding(0, bar.Right, 0, 0).Background(t.Surface).Children(func() {
		name := ui.Row(c).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).
			Gap(t.Space(2.5)).Padding(0, t.Space(2), 0, t.Space(4)+bar.Left).DragWindow()
		name.Width(sidebarWidth - bar.Left).Shrink(0)
		name.Children(func() {
			ui.Text(c, "Ottr").FontSize(t.FontSize + 3).Bold()
			a.recordingPill(c, t)
		})
		ui.Row(c).Grow(1).FillHeight().DragWindow() // drag room beside the controls
	})
}

// windowTitleBar gives secondary windows the same immersive title strip.
func (a *App) windowTitleBar(c *ui.Context, t *ui.Theme, title string) {
	bar := c.TitleBar()
	height := max(bar.Height, float32(titleBarHeight))
	ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).
		Padding(0, bar.Right+t.Space(2), 0, t.Space(4)+bar.Left).
		Background(t.Surface).DragWindow().Children(func() {
		ui.Text(c, title).FontSize(t.FontSize + 3).Bold()
		a.recordingPill(c, t)
	})
}

// recordingPill says a recording runs, and for how long.
func (a *App) recordingPill(c *ui.Context, t *ui.Theme) {
	if !a.recording {
		return
	}
	ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
		dot := ui.Box(c).Size(8, 8).Radius(4).Background(t.Danger)
		dot.Opacity(float32(0.35 + 0.65*dot.Loop("pulse", 1100*time.Millisecond, ui.EaseInOut)))
		ui.Text(c, fmtDuration(time.Since(a.startedAt).Milliseconds())).
			FontSize(t.FontSize - 1).TextColor(t.TextMuted).FontFeatures("tnum")
	})
}

// previewRefresh is how often the idle preview takes a new still frame. It is
// slow on purpose: the desktop is grabbed for each one.
const previewRefresh = 3 * time.Second

// update runs fn in a frame, from any goroutine: through the window when
// there is one, else on the next frame the view builds.
func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(func() {
			fn()
			for _, win := range []*mygo.Window{a.libraryWin, a.settingsWin} {
				if win != nil && !win.IsDestroyed() {
					win.Invalidate()
				}
			}
		})
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
		a.showPreview()
	case st.Recording && !a.recording:
		a.recording, a.startedAt, a.late, a.preview = true, st.StartedAt, nil, nil
	}
	if !a.recording {
		// The source is shown as a still picture, so that what will be
		// recorded is visible before it is. Continuous grabbing keeps the
		// desktop compositor busy enough to make the pointer stutter, so the
		// previewer takes one frame at a time, refreshed every few seconds.
		if a.Preview != nil {
			a.previewErr = ""
			if err := a.Preview.Err(); err != nil {
				a.previewErr = err.Error()
			}
			a.grabPreview()
			switch {
			case a.Preview.Pending():
				c.After(200 * time.Millisecond) // the still is on its way
			case a.preview != nil && time.Since(a.previewAt) > previewRefresh:
				a.showPreview()
				c.After(200 * time.Millisecond)
			}
		}
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

// poll keeps frames coming while something the window shows is still moving,
// and reads what a job did: the services announce theirs to pages, and a
// window of native UI has none.
func (a *App) poll(c *ui.Context) {
	prev := a.job
	a.refreshJob()
	if prev.State != a.job.State && (prev.Running() || a.job.Running()) {
		a.reload()
		if !a.job.Running() {
			a.afterJob(a.job)
		}
	}
	if !a.recording && a.pending() {
		c.After(250 * time.Millisecond)
	}
}

// refreshJob reads the newest job over the media shown.
func (a *App) refreshJob() {
	if a.Jobs == nil || a.sel == 0 {
		a.job = jobs.Job{}
		return
	}
	job, ok := a.Jobs.Latest(a.sel)
	if !ok {
		job = jobs.Job{}
	}
	a.job = job
}

// jobRunning reports whether work of this kind is running over the media shown.
func (a *App) jobRunning(kind string) bool {
	return a.job.Kind == kind && a.job.Running()
}

// afterJob reads what a job that ended left behind, and says why it failed.
func (a *App) afterJob(job jobs.Job) {
	switch job.Kind {
	case jobs.KindTranscribe:
		a.trLoaded = false // the lines may be new ones
	case jobs.KindTranslate:
		a.loadTranslation()
	case jobs.KindSummarize:
		a.sumLoaded = false
		a.loadSummary()
	}
	if job.State == jobs.StateFailed && job.Error != "" {
		a.setError(fmt.Errorf("%s: %s", job.Kind, job.Error))
	}
}

// cancelJob stops the job over the media shown.
func (a *App) cancelJob() {
	if a.Jobs == nil {
		return
	}
	if err := a.Jobs.Cancel(a.job.ID); err != nil {
		a.setError(err)
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
	if a.Rec != nil && a.Rec.Dir != "" {
		if orphans, err := a.Lib.Orphans(a.Rec.Dir); err == nil {
			a.orphans = orphans
		}
	}
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
		a.page = ""
		return
	}
	a.forget()
	a.sel = id
	a.page = ""
}

// forget drops what was loaded for the media shown.
func (a *App) forget() {
	a.sel = 0
	a.transcript, a.segs = asr.Transcript{}, nil
	a.trLoaded, a.sumLoaded = false, false
	a.trans, a.sum = translate.Translation{}, summary.Summary{}
	a.transLang, a.segQuery = "", ""
	a.lang = ""
}

// pending reports whether something the window shows is still being worked
// on, so that the window asks for frames until it settles.
func (a *App) pending() bool {
	if a.recording || a.busy || a.job.Running() {
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
	if a.confirmOrphans {
		ui.Modal(c, &a.confirmOrphans, func() {
			ui.Text(c, fmt.Sprintf("Delete %d recording files?", len(a.orphans))).Bold()
			ui.Text(c, "They were recorded here but no record in the library holds them. Deleting them cannot be undone.").
				TextColor(t.TextMuted).FontSize(t.FontSize - 1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Keep files").Clicked() {
					a.confirmOrphans = false
				}
				if ui.PrimaryButton(c, "Delete").Clicked() {
					a.confirmOrphans = false
					a.deleteOrphans()
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
	// Below the title bar, so that the app's name and the window controls stay
	// reachable while an error is up.
	top := max(c.TitleBar().Height, float32(titleBarHeight)) + t.Space(2)
	ui.Overlay(c, func() {
		ui.Row(c).Attach(ui.AnchorTop, ui.AnchorTop).Top(top).MaxWidth(680).
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
