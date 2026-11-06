package native

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// The icons are Lucide's (https://lucide.dev, ISC): 24x24 and drawn with
// strokes, so that ui.Icon paints them in the color of the text around them.
// See icons/LICENSE and icons/README.md.
//
//go:embed icons/*.svg
var iconFiles embed.FS

func loadIcon(name string) *ui.SVG {
	data, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic("icons: " + err.Error())
	}
	return ui.MustParseSVG(data)
}

var (
	// monitor: a display, for the screen and each monitor.
	iconScreen = loadIcon("monitor")
	// app-window: a window of another app.
	iconWindow = loadIcon("app-window")
	// mic and volume-2: the two sounds a recording can take.
	iconMic     = loadIcon("mic")
	iconSpeaker = loadIcon("volume-2")
	// sliders-horizontal: settings.
	iconSliders = loadIcon("sliders-horizontal")
	// settings and clapperboard: the round buttons under the recorder.
	iconSettings = loadIcon("settings")
	// Brand marks of the provider cards.
	iconDeepSeek   = loadIcon("deepseek")
	iconOpenRouter = loadIcon("openrouter")
	iconOllama     = loadIcon("ollama")
	iconClapper    = loadIcon("clapperboard")
	// search and x: the transcript toolbar.
	iconSearch = loadIcon("search")
	iconClose  = loadIcon("x")
	// arrow-left and check: going back, and what is chosen.
	iconBack  = loadIcon("arrow-left")
	iconCheck = loadIcon("check")
	// import: adding a file to the library.
	iconImport = loadIcon("import")
	// folder and trash-2: file actions on a chosen media.
	iconFolder = loadIcon("folder")
	iconTrash  = loadIcon("trash-2")
	// download: the export menu of a tab.
	iconDownload = loadIcon("download")
	// play, pause, fullscreen and more: the player in the detail page.
	iconPlay       = loadIcon("play")
	iconPause      = loadIcon("pause")
	iconFullscreen = loadIcon("maximize")
	iconVolumeX    = loadIcon("volume-x")
	iconMore       = loadIcon("more-vertical")
	// Library's empty state, layout switch and right-panel toggle.
	iconLibrary    = loadIcon("library")
	iconGrid       = loadIcon("layout-grid")
	iconList       = loadIcon("list")
	iconPanelClose = loadIcon("panel-left-close")
	// languages and file-text: the two tabs outside Settings' own pages.
	iconLanguages = loadIcon("languages")
	iconEarth     = loadIcon("earth")
	iconSummary   = loadIcon("file-text")
	// server and users: Settings' provider and speaker sections.
	iconProvider = loadIcon("server")
	iconSpeakers = loadIcon("users")
	// clock and circle-alert: a step still queued, and work that failed.
	iconClock = loadIcon("clock")
	iconAlert = loadIcon("circle-alert")
	// copy and refresh-cw: copying a summary, and generating it again.
	iconCopy    = loadIcon("copy")
	iconRefresh = loadIcon("refresh-cw")

	// The redesign's additions (lucide-static 1.70.0).
	iconChevronDown   = loadIcon("chevron-down")
	iconChevronRight  = loadIcon("chevron-right")
	iconArrowUp       = loadIcon("arrow-up")
	iconArrowDown     = loadIcon("arrow-down")
	iconWaveform      = loadIcon("audio-lines")
	iconCamera        = loadIcon("camera")
	iconCaptions      = loadIcon("captions")
	iconCircleCheck   = loadIcon("circle-check")
	iconCPU           = loadIcon("cpu")
	iconFilm          = loadIcon("film")
	iconFolderOpen    = loadIcon("folder-open")
	iconImage         = loadIcon("image")
	iconMicOff        = loadIcon("mic-off")
	iconPalette       = loadIcon("palette")
	iconSkipBack      = loadIcon("skip-back")
	iconSkipForward   = loadIcon("skip-forward")
	iconSparkles      = loadIcon("sparkles")
	iconArea          = loadIcon("square-dashed")
	iconStop          = loadIcon("square")
	iconAlertTriangle = loadIcon("alert-triangle")
	iconType          = loadIcon("type")
	iconMoreH         = loadIcon("more-horizontal")
	iconDisk          = loadIcon("hard-drive")
	iconPencil        = loadIcon("pencil")
	iconUserX         = loadIcon("user-x")
	iconKey           = loadIcon("key-round")
)
