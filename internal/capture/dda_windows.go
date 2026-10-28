//go:build windows

package capture

import (
	"os/exec"
	"sync"

	"ottr/internal/tools"
)

var (
	ddaOnce sync.Once
	ddaOK   bool
)

// ddaAvailable reports whether Desktop Duplication can be read right now.
//
// It is asked once per run: the answer is a property of the FFmpeg build and of
// the desktop session. Windows refuses duplication while the session sits on a
// secure or disconnected desktop — a locked screen, fast user switching, or a
// remote session that was dropped — and answers the same way the interactive
// screen would not be there at all.
func ddaAvailable() bool {
	ddaOnce.Do(func() {
		ffmpeg, err := tools.FFmpeg()
		if err != nil {
			return
		}
		// One frame, decoded to nothing: this succeeds only when the desktop can
		// be duplicated.
		cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "ddagrab=output_idx=0:framerate=1:draw_mouse=0,hwdownload,format=bgra",
			"-frames:v", "1", "-f", "null", "-")
		hideWindow(cmd)
		ddaOK = cmd.Run() == nil
	})
	return ddaOK
}
