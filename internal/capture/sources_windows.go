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

// windowTitles lists the titles of visible top-level windows, excluding
// this process.
func windowTitles() ([]string, error) {
	self := uint32(syscall.Getpid())
	var titles []string
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
		titles = append(titles, syscall.UTF16ToString(buf))
		return 1
	})
	ret, _, err := user32.NewProc("EnumWindows").Call(cb, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumWindows: %w", err)
	}
	return titles, nil
}

func listSources() ([]Source, error) {
	sources := []Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}
	if rects, err := monitorRects(); err == nil && len(rects) > 1 {
		for i, r := range rects {
			sources = append(sources, Source{
				ID:   fmt.Sprintf("display:%d", i),
				Kind: "display",
				Name: fmt.Sprintf("Display %d (%dx%d)", i+1, r.Right-r.Left, r.Bottom-r.Top),
			})
		}
	}
	if titles, err := windowTitles(); err == nil {
		for _, t := range titles {
			sources = append(sources, Source{ID: "window:" + t, Kind: "window", Name: t})
		}
	}
	return sources, nil
}

// resolveSource turns a Source.ID into ffmpeg gdigrab input arguments.
func resolveSource(id string) ([]string, error) {
	base := []string{"-thread_queue_size", "512", "-f", "gdigrab", "-framerate", "30"}
	switch {
	case id == "screen":
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
