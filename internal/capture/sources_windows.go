//go:build windows

package capture

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")

type rect struct {
	Left, Top, Right, Bottom int32
}

// monitorRects returns the virtual-screen rectangles of every display.
func monitorRects() ([]rect, error) {
	var rects []rect
	cb := syscall.NewCallback(func(hmon, hdc uintptr, r *rect, lparam uintptr) uintptr {
		rects = append(rects, *r)
		return 1
	})
	ret, _, err := user32.NewProc("EnumDisplayMonitors").Call(0, 0, cb, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors: %w", err)
	}
	return rects, nil
}

// windowSource is one capturable window: its title and, when Windows reports
// a sane rectangle for it, its size.
type windowSource struct {
	title string
	w, h  int32
}

// windowSources lists the visible top-level windows of other processes.
func windowSources() ([]windowSource, error) {
	self := uint32(syscall.Getpid())
	var windows []windowSource
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		ret, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
		if ret == 0 {
			return 1
		}
		var pid uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == self {
			return 1
		}
		n, _, _ := user32.NewProc("GetWindowTextLengthW").Call(hwnd)
		if n == 0 {
			return 1
		}
		buf := make([]uint16, n+1)
		user32.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
		w := windowSource{title: syscall.UTF16ToString(buf)}
		var r rect
		if ok, _, _ := user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r))); ok != 0 {
			// A minimized window sits far off screen; its size says nothing
			// about what capturing it would show.
			if r.Right > r.Left && r.Bottom > r.Top && r.Left > -30000 {
				w.w, w.h = r.Right-r.Left, r.Bottom-r.Top
			}
		}
		windows = append(windows, w)
		return 1
	})
	ret, _, err := user32.NewProc("EnumWindows").Call(cb, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumWindows: %w", err)
	}
	return windows, nil
}

func listSources() ([]Source, error) {
	screen := Source{ID: "screen", Kind: "screen", Name: "Full screen"}
	rects, err := monitorRects()
	if err != nil {
		rects = nil
	}
	if len(rects) > 0 {
		// The whole screen is the box around every display.
		left, top := rects[0].Left, rects[0].Top
		right, bottom := rects[0].Right, rects[0].Bottom
		for _, r := range rects[1:] {
			left, top = min(left, r.Left), min(top, r.Top)
			right, bottom = max(right, r.Right), max(bottom, r.Bottom)
		}
		screen.Detail = fmt.Sprintf("%dx%d", right-left, bottom-top)
	}
	sources := []Source{screen}
	if len(rects) > 1 {
		for i, r := range rects {
			sources = append(sources, Source{
				ID:     fmt.Sprintf("display:%d", i),
				Kind:   "display",
				Name:   fmt.Sprintf("Display %d", i+1),
				Detail: fmt.Sprintf("%dx%d", r.Right-r.Left, r.Bottom-r.Top),
			})
		}
	}
	if windows, err := windowSources(); err == nil {
		for _, w := range windows {
			src := Source{ID: "window:" + w.title, Kind: "window", Name: w.title}
			if w.w > 0 {
				src.Detail = fmt.Sprintf("%dx%d", w.w, w.h)
			}
			sources = append(sources, src)
		}
	}
	return sources, nil
}

// resolveSource turns a Source.ID into ffmpeg capture arguments.
func resolveSource(id string) ([]string, error) {
	return resolveSourceFPS(id, 30)
}

// resolveSourceFPS is resolveSource at another capture rate.
//
// A screen or display is taken through Desktop Duplication (ddagrab) when it is
// available: it reads what the compositor already drew instead of forcing a
// fresh GDI copy, so it costs far less and does not disturb the desktop. Windows
// cannot lend the desktop out while it is on a secure or disconnected desktop,
// so anything that is not a plain screen is left to gdigrab, which still has the
// only way to capture a single window.
func resolveSourceFPS(id string, fps int) ([]string, error) {
	base := []string{"-thread_queue_size", "512", "-f", "gdigrab", "-framerate", fmt.Sprint(fps)}
	switch {
	case id == "screen":
		if ddaAvailable() {
			return ddaArgs(0, 0, 0, 0, 0, fps), nil
		}
		return append(base, "-i", "desktop"), nil
	case strings.HasPrefix(id, "display:"):
		var n int
		if _, err := fmt.Sscanf(id, "display:%d", &n); err != nil {
			return nil, fmt.Errorf("bad source %q", id)
		}
		rects, err := monitorRects()
		if err != nil {
			return nil, err
		}
		if n < 0 || n >= len(rects) {
			return nil, fmt.Errorf("display %d no longer exists", n+1)
		}
		if ddaAvailable() {
			r := rects[n]
			return ddaArgs(n, int(r.Left), int(r.Top), int(r.Right-r.Left), int(r.Bottom-r.Top), fps), nil
		}
		r := rects[n]
		return append(base,
			"-offset_x", fmt.Sprint(r.Left), "-offset_y", fmt.Sprint(r.Top),
			"-video_size", fmt.Sprintf("%dx%d", r.Right-r.Left, r.Bottom-r.Top),
			"-i", "desktop"), nil
	case strings.HasPrefix(id, "window:"):
		title := strings.TrimPrefix(id, "window:")
		if title == "" {
			return nil, fmt.Errorf("empty window title")
		}
		return append(base, "-i", "title="+title), nil
	}
	return nil, fmt.Errorf("unknown source %q", id)
}

// ddaArgs takes a display out of the Desktop Duplication API. The frames come
// back on the GPU, so hwdownload brings them to memory for the encoders that
// read them.
func ddaArgs(output, x, y, w, h, fps int) []string {
	grab := fmt.Sprintf("ddagrab=output_idx=%d:framerate=%d:draw_mouse=0", output, fps)
	if w > 0 && h > 0 {
		grab += fmt.Sprintf(":offset_x=%d:offset_y=%d:video_size=%dx%d", x, y, w, h)
	}
	return []string{
		"-thread_queue_size", "512",
		"-f", "lavfi", "-i", grab + ",hwdownload,format=bgra",
	}
}
