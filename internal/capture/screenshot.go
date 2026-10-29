package capture

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ottr/internal/library"
)

// Snapshotter takes one PNG of one source and puts it in the library. It is
// bound to the frontend as "Screenshot".
type Snapshotter struct {
	Backend Backend
	Dir     string           // where screenshots are written
	Lib     *library.Service // finished screenshots land here
}

// Screenshot captures one frame of a source at its native size and returns the
// record it made. The file is written to a temporary name and renamed, so a
// crash leaves either a whole PNG or none.
func (s *Snapshotter) Screenshot(ctx context.Context, sourceID string) (library.Media, error) {
	if s.Backend == nil || s.Lib == nil {
		return library.Media{}, errors.New("screenshots need a capture backend and the library")
	}
	frame, err := s.Backend.Snapshot(ctx, SnapshotConfig{SourceID: sourceID, Cursor: true})
	if err != nil {
		return library.Media{}, err
	}
	if frame.Width <= 0 || frame.Height <= 0 || len(frame.Data) < frame.Width*frame.Height*4 {
		return library.Media{}, fmt.Errorf("%w: the frame is empty", ErrNoFrame)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return library.Media{}, err
	}
	name := "Screenshot " + time.Now().Format("2006-01-02 15.04.05") + ".png"
	file := filepath.Join(s.Dir, name)

	tmp, err := os.CreateTemp(s.Dir, ".screenshot-*.png")
	if err != nil {
		return library.Media{}, err
	}
	if err := png.Encode(tmp, rgbaOf(frame)); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return library.Media{}, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return library.Media{}, err
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		os.Remove(tmp.Name())
		return library.Media{}, err
	}
	st, err := os.Stat(file)
	if err != nil {
		os.Remove(file)
		return library.Media{}, err
	}
	return s.Lib.AddScreenshot(library.Record{
		Name:     strings.TrimSuffix(name, ".png"),
		Path:     file,
		Width:    frame.Width,
		Height:   frame.Height,
		Size:     st.Size(),
		HasVideo: true,
	})
}
