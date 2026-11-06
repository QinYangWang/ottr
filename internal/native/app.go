// Package native draws Ottr's recording, Library and Settings windows with
// MyGo's own toolkit: no web page, no HTML.
package native

import (
	"context"
	"fmt"
	"strconv"
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
	settingsWin *mygo.Window

	theme    string // system | light | dark
	language string // en | zh
	font     string // installed family, or the system default

	libraryOpen     bool
	librarySet      bool // a manual choice overrides the responsive default
	libraryResized  bool // this toggle widened a compact window for both panes
	libraryGridView bool
	thumbs          map[int64]*ui.Bitmap
	thumbLoading    map[int64]bool

	// The library.
	media             []library.Media
	query             string
	sel               int64
	tab               int
	err               string
	renameID          int64
	renameText        string
	renameSpeakerID   int
	renameSpeakerText string

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
	recording       bool
	recordingPaused bool
	startedAt       time.Time
	pauseStarted    time.Time
	pausedTotal     time.Duration
	liveWarn        string // why live text is behind, when it is
	late            []liveLine

	// Translation of the provisional transcript: what each finalized line
	// became in trLang, the requests in flight, and a generation that drops
	// results a switch or a language change made stale.
	liveTranslate  bool
	liveTrans      map[int64]liveTranslation
	liveTransBusy  map[int64]bool
	liveTransErr   string
	liveTransGen   int64
	liveTranslator func(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error)
	preview        *ui.Bitmap
	previewW       int
	previewH       int
	previewAt      time.Time

	// Playback belongs to the selected media and is closed before switching.
	playback    playback
	playbackGen int64
	muted       bool

	// The session shown, and its three tabs.
	transcript            asr.Transcript
	segs                  []asr.Segment
	trLoaded              bool
	lang                  string // the language the transcript says it is in
	trLang                string
	trans                 translate.Translation
	transLang             string
	transLoading          bool
	showTranslation       bool
	segSearchOpen         bool
	subtitleSettingsOpen  bool
	languageMenuOpen      bool
	transcriptionMenuOpen bool
	translationMenuOpen   bool
	styleMenuOpen         bool
	defaultLanguage       string
	subtitleFontSize      float32
	sum                   summary.Summary
	sumLoaded             bool
	segQuery              string

	// job is background work over the media shown: what it does, how far it
	// has come, and what became of it.
	job jobs.Job

	// The main window shows the recorder, its library, or one media's details.
	page string

	// The Settings window and what it shows.
	settingsTab  int
	modelList    []models.Info
	modelInUse   string
	modelLoaded  bool
	modelLoading bool

	// Provider settings, read when the Settings window opens them. The page
	// edits what translation and summaries share; the transcript menu still
	// picks between the free and the AI translator.
	transCfg       translate.Settings
	loadedTrans    bool
	provider       providerSettings
	loadedProvider bool

	// The speaker library: voices heard across recordings, and the row being
	// renamed.
	speakerLib      []asr.Speaker
	speakerQuery    string
	loadedSpeakers  bool
	speakerEditID   int
	speakerEditText string

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

// liveLine is one line of live text the recording pane shows: a finalized
// utterance, or the one still being spoken.
type liveLine struct {
	at        int64 // ms into the recording where the line starts
	utterance int64 // which utterance it belongs to
	text      string
	final     bool
}

// liveTranslation is a translated live line and the text it was made from, so
// that a revised final line is translated again instead of kept stale.
type liveTranslation struct {
	text       string
	translated string
}

// View returns the window's content.
func View(a *App) *ui.Content { return ui.View(a.view) }

// Begin loads what the window shows.
func (a *App) Begin(win *mygo.Window) {
	a.win = win
	a.trLang = "en"
	a.renameSpeakerID = -1
	a.speakerEditID = -1
	if a.Prefs != nil {
		if source := a.Prefs.Get(settings.Theme); source != "" {
			a.theme = source
		}
		a.language = a.Prefs.Get(settings.Language)
		a.font = a.Prefs.Get(settings.Font)
		a.defaultLanguage = a.Prefs.Get(settings.TranscriptionLanguage)
		a.subtitleFontSize = 14
		if size, err := strconv.ParseFloat(a.Prefs.Get(settings.SubtitleFontSize), 32); err == nil && size >= 10 && size <= 24 {
			a.subtitleFontSize = float32(size)
		}
	}
	a.lang = a.defaultLanguage
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

// The recording window grows a library beside it when both panes fit.
const (
	CompactWidth        = 420
	CompactHeight       = 800
	WideWidth           = 1180
	WideHeight          = 800
	libraryWidth        = 360
	libraryAutoMinWidth = CompactWidth + libraryWidth + 60
)

func (a *App) libraryVisible(width float32) bool {
	if a.librarySet {
		return a.libraryOpen
	}
	return width >= libraryAutoMinWidth
}

func (a *App) toggleLibrary(width float32) {
	a.libraryOpen = !a.libraryVisible(width)
	a.librarySet = true
	if a.libraryOpen {
		a.closeDetail()
		a.reload()
		if a.win != nil && width < libraryAutoMinWidth {
			_, height := a.win.Size()
			a.libraryResized = true
			a.win.SetMinimumSize(libraryAutoMinWidth, 560)
			a.win.SetSize(libraryAutoMinWidth, height)
		}
	} else {
		// Collapsing the panel is a normal-window action: a maximized window
		// would keep the wide layout the panel left behind.
		if a.win != nil && a.win.IsMaximized() {
			a.win.Unmaximize()
		}
		if a.libraryResized {
			a.libraryResized = false
			if a.win != nil {
				_, height := a.win.Size()
				a.win.SetMinimumSize(380, 560)
				a.win.SetSize(CompactWidth, height)
			}
		}
	}
}

func (a *App) resize() {
	if a.win == nil {
		return
	}
	a.win.SetSize(CompactWidth, CompactHeight)
}

// ---------------------------------------------------------------- the view

// titleBarHeight is the strip the window controls sit in. The window is created
// with a hidden title bar of this height (mygo.WindowOptions), and the view
// draws its own bar there.
const titleBarHeight = 46

func (a *App) view(c *ui.Context) {
	t := a.applyTheme(c)
	a.drain()
	a.mirror(c)
	a.poll(c)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.titleBar(c, t)
		width, _ := c.Size()
		switch {
		case a.recording:
			a.recorderPage(c, t)
		case a.detail():
			if m, ok := a.current(); ok {
				a.sessionPage(c, t, m)
			}
		default:
			showLibrary := a.libraryVisible(width)
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				a.sidebar(c, t, showLibrary)
				if showLibrary {
					a.libraryPanel(c, t)
				}
			})
		}
	})
	if a.err != "" {
		a.banner(c, t)
	}
	a.dialogs(c, t)
}

// titleBar is the window's own title bar, in the strip the window controls
// sit in: the app's name and what it is doing, and drag room everywhere else.
// The controls themselves are the system's, at the far right, so the bar keeps
// their room free (ui.TitleBar).
func (a *App) titleBar(c *ui.Context, t *ui.Theme) {
	bar := c.TitleBar()
	height := max(bar.Height, float32(titleBarHeight))

	// The whole bar drags the window, its right end left clear for the
	// controls.
	row := ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).
		Padding(0, bar.Right+t.Space(2), 0, t.Space(4)+bar.Left).
		Background(t.Surface).DragWindow()
	row.Children(func() {
		ui.Text(c, "Ottr").FontSize(t.FontSize + 3).Bold()
	})
}

// windowTitleBar gives secondary windows the same immersive title strip, and
// carries the controls of the page shown at its right, when it has any.
func (a *App) windowTitleBar(c *ui.Context, t *ui.Theme, title string, tools func()) {
	bar := c.TitleBar()
	height := max(bar.Height, float32(titleBarHeight))
	ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).
		Padding(0, bar.Right+t.Space(2), 0, t.Space(4)+bar.Left).
		Background(t.Surface).DragWindow().Children(func() {
		ui.Text(c, title).FontSize(t.FontSize + 2).Bold().Shrink(0)
		a.recordingPill(c, t)
		if tools != nil {
			ui.Spacer(c)
			tools()
		}
	})
}

// backButton is the way out of an inner page: an arrow alone when label is
// empty, else the arrow and the page it goes to.
func (a *App) backButton(c *ui.Context, t *ui.Theme, label string) bool {
	name := a.tr("Back")
	if label != "" {
		name = a.tr("Back to ") + a.tr(label)
	}
	btn := ui.ButtonBase(c).Label(a.tr("Back")).Tooltip(name).
		Padding(t.Space(1.5)).Radius(t.Radius).TextColor(t.TextMuted)
	if btn.Hovered() {
		btn.Background(t.SurfaceHover).TextColor(t.Text)
	}
	btn.Children(func() {
		if label == "" {
			ui.Icon(c, iconBack).FontSize(t.FontSize + 2)
			return
		}
		ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBack).FontSize(t.FontSize + 1)
			ui.Text(c, label).FontSize(t.FontSize - 1)
		})
	})
	return btn.Clicked()
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

// update runs fn in a frame, from any goroutine: through the window when
// there is one, else on the next frame the view builds.
func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(func() {
			fn()
			for _, win := range []*mygo.Window{a.settingsWin} {
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
		a.resetLiveTranslation()
		a.reload()
		a.showLatest()
		a.showPreview()
	case st.Recording && !a.recording:
		a.recording, a.startedAt, a.late, a.preview = true, st.StartedAt, nil, nil
		a.resetLiveTranslation()
		a.recordingPaused, a.pauseStarted, a.pausedTotal = false, time.Time{}, 0
	}
	if !a.recording {
		// The source is live, so that what will be recorded is visible before
		// it is. The previewer captures a handful of frames a second itself,
		// and draws no pointer of its own.
		if a.Preview != nil {
			a.previewErr = ""
			if err := a.Preview.Err(); err != nil {
				a.previewErr = err.Error()
			}
			a.grabPreview()
			c.After(200 * time.Millisecond)
		}
		return
	}
	a.startedAt = st.StartedAt
	if st.Paused && !a.recordingPaused {
		a.pauseStarted = time.Now()
	} else if !st.Paused && a.recordingPaused {
		a.pausedTotal += time.Since(a.pauseStarted)
	}
	a.recordingPaused = st.Paused
	if st.Live != nil {
		a.reviseLive(asr.LiveSubtitle{
			Utterance: st.Live.Utterance, StartMs: st.Live.StartMs,
			Text: st.Live.Text, Final: st.Live.Final, Warn: st.Live.Warn,
		})
	}
	a.grabPreview()
	c.After(150 * time.Millisecond)
}

// reviseLive puts a live subtitle revision on the pane: a revision of the same
// utterance updates that line in place, a final one freezes it, and the next
// utterance becomes a new line. Only the newest six finalized lines are kept,
// plus the one being spoken.
func (a *App) recordingElapsed() time.Duration {
	elapsed := time.Since(a.startedAt) - a.pausedTotal
	if a.recordingPaused {
		elapsed -= time.Since(a.pauseStarted)
	}
	return max(elapsed, 0)
}

func (a *App) reviseLive(sub asr.LiveSubtitle) {
	switch {
	case sub.Text == "" && sub.Warn != "":
		a.liveWarn = sub.Warn
		return
	case sub.Error != "":
		a.liveWarn = sub.Error
		return
	case sub.Text == "":
		return
	}
	a.liveWarn = ""
	at := sub.StartMs
	if i := a.liveIndex(sub.Utterance); i >= 0 {
		a.late[i].text, a.late[i].at, a.late[i].final = sub.Text, at, sub.Final
	} else {
		a.late = append(a.late, liveLine{at: at, utterance: sub.Utterance, text: sub.Text, final: sub.Final})
	}
	if sub.Final {
		a.translateLiveLine(sub.Utterance, sub.Text)
	}
	// Keep the newest six finalized lines. Provisional ones do not count, so
	// the line being spoken is never scrolled away.
	finals := 0
	for i := len(a.late) - 1; i >= 0; i-- {
		if !a.late[i].final {
			continue
		}
		finals++
		if finals > 6 {
			a.late = a.late[i+1:]
			break
		}
	}
}

// liveIndex finds the line of an utterance, or -1.
func (a *App) liveIndex(utterance int64) int {
	for i := len(a.late) - 1; i >= 0; i-- {
		if a.late[i].utterance == utterance {
			return i
		}
	}
	return -1
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
	a.ensureThumbnails()
	if a.Rec != nil && a.Rec.Dir != "" {
		if orphans, err := a.Lib.Orphans(a.Rec.Dir); err == nil {
			a.orphans = orphans
		}
	}
	if _, ok := a.current(); !ok && a.sel != 0 {
		// The media shown is gone: back to the grid.
		a.forget()
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

// selectMedia shows one media's page; the same one again keeps it as it is.
func (a *App) selectMedia(id int64) {
	if a.sel == id {
		return
	}
	a.forget()
	a.sel = id
}

// closeDetail goes back to the grid, and drops what was loaded for the media.
func (a *App) closeDetail() {
	a.forget()
}

// openDetail shows one media's page in the main window.
func (a *App) openDetail(id int64) {
	a.selectMedia(id)
}

// forget drops what was loaded for the media shown.
func (a *App) forget() {
	a.closePlayback()
	a.sel = 0
	a.renameID, a.renameText = 0, ""
	a.renameSpeakerID, a.renameSpeakerText = -1, ""
	a.transcript, a.segs = asr.Transcript{}, nil
	a.trLoaded, a.sumLoaded = false, false
	a.trans, a.sum = translate.Translation{}, summary.Summary{}
	a.transLang, a.segQuery, a.segSearchOpen = "", "", false
	a.showTranslation = false
	a.subtitleSettingsOpen = false
	a.languageMenuOpen, a.transcriptionMenuOpen = false, false
	a.translationMenuOpen, a.styleMenuOpen = false, false
	a.lang = a.defaultLanguage
}

// pending reports whether something the window shows is still being worked
// on, so that the window asks for frames until it settles.
func (a *App) pending() bool {
	if a.recording || a.busy || a.job.Running() || a.playbackPlaying() {
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
	if a.tab == 1 && !a.sumLoaded && m.SummaryStatus == "done" {
		return true
	}
	return false
}

func (a *App) dialogs(c *ui.Context, t *ui.Theme) {
	if a.discard {
		ui.Modal(c, &a.discard, func() {
			ui.Text(c, a.tr("Delete this recording?")).Bold()
			ui.Text(c, a.tr("The file is deleted from disk, and the library keeps nothing.")).
				TextColor(t.TextMuted).FontSize(t.FontSize - 1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, a.tr("Keep recording")).Clicked() {
					a.discard = false
				}
				if ui.PrimaryButton(c, a.tr("Delete")).Clicked() {
					a.discard = false
					a.stopRecording(true)
				}
			})
		})
	}
	if a.confirmOrphans {
		ui.Modal(c, &a.confirmOrphans, func() {
			ui.Text(c, a.trf("Delete %d recording files?", len(a.orphans))).Bold()
			ui.Text(c, a.tr("They were recorded here but no record in the library holds them. Deleting them cannot be undone.")).
				TextColor(t.TextMuted).FontSize(t.FontSize - 1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, a.tr("Keep files")).Clicked() {
					a.confirmOrphans = false
				}
				if ui.PrimaryButton(c, a.tr("Delete")).Clicked() {
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
			ui.Text(c, a.tr("Delete ")+name+a.tr("?")).Bold()
			keep := true
			ui.Checkbox(c, &keep, a.tr("Keep the file on disk")).Changed()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, a.tr("Cancel")).Clicked() {
					a.confirmDel = false
				}
				if ui.PrimaryButton(c, a.tr("Delete")).Clicked() {
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
			if ui.Button(c, a.tr("Dismiss")).Clicked() {
				a.err = ""
			}
		})
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
