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
	// arrow-left and check: going back, and what is chosen.
	iconBack  = loadIcon("arrow-left")
	iconCheck = loadIcon("check")
	// scan and video: an area of the screen, and a camera.
	iconArea   = loadIcon("scan")
	iconCamera = loadIcon("video")
	// import: adding a file to the library.
	iconImport = loadIcon("import")
	// library and sun/moon: the footer windows and theme toggle.
	iconLibrary = loadIcon("library")
	iconSun     = loadIcon("sun")
	iconMoon    = loadIcon("moon")
	// languages and file-text: the Settings window's last two sections.
	iconLanguages = loadIcon("languages")
	iconSummary   = loadIcon("file-text")
)
