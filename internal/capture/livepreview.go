package capture

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"ottr/internal/tools"
)

// Previewer shows a source before it is recorded: a short-lived FFmpeg that
// grabs the same input a recording would and writes an MJPEG stream over TCP,
// read the way the recording's own preview is. It is one process at a time and
// stops when the recording starts, which draws its own.
type Previewer struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	hub    *previewHub
	source string
	err    error
}

// NewPreviewer creates a previewer. Nothing runs until Show.
func NewPreviewer() *Previewer { return &Previewer{} }

// Show draws the source given from now on, in the background: the caller gets
// the picture from Frame once FFmpeg has one, a moment later.
func (p *Previewer) Show(sourceID string) {
	p.mu.Lock()
	if p.source == sourceID && p.cmd != nil && p.err == nil {
		p.mu.Unlock()
		return
	}
	p.stopLocked()
	p.source = sourceID
	p.mu.Unlock()

	p.start(sourceID)
}

func (p *Previewer) start(sourceID string) {
	video, err := resolveSource(sourceID)
	if err != nil {
		p.fail(err)
		return
	}
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		p.fail(err)
		return
	}
	hub, err := newPreviewHub()
	if err != nil {
		p.fail(err)
		return
	}

	// The same preview output a recording writes, at a size that keeps the
	// stream small: five frames a second, 480 pixels wide.
	args := append(video, "-map", "0:v", "-vf", "fps=5,scale=480:-2",
		"-f", "mpjpeg", "tcp://"+hub.addr())
	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		hub.Close()
		p.fail(fmt.Errorf("start ffmpeg: %w", err))
		return
	}

	p.mu.Lock()
	// A newer Show may have won while this one started: keep only the newest.
	if p.source != sourceID {
		p.mu.Unlock()
		cmd.Process.Kill()
		cmd.Wait()
		hub.Close()
		return
	}
	p.cmd, p.hub, p.err = cmd, hub, nil
	p.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()
		p.mu.Lock()
		if p.cmd == cmd {
			// It ended on its own — bad arguments, or the source went away.
			// Nothing runs now, so the next Show starts again, and what FFmpeg
			// said is the error the window shows.
			p.cmd, p.hub, p.source = nil, nil, ""
			p.err = previewError(waitErr, stderr.String())
		}
		p.mu.Unlock()
		hub.Close()
	}()
}

// previewError is what FFmpeg said, in one line.
func previewError(waitErr error, stderr string) error {
	said := strings.TrimSpace(stderr)
	if i := strings.IndexByte(said, '\n'); i >= 0 {
		said = said[:i]
	}
	switch {
	case said != "":
		return errors.New("ffmpeg: " + said)
	case waitErr != nil:
		return fmt.Errorf("ffmpeg stopped: %w", waitErr)
	default:
		return errors.New("the preview stopped")
	}
}

func (p *Previewer) fail(err error) {
	p.mu.Lock()
	p.err = err
	p.cmd, p.hub = nil, nil
	p.mu.Unlock()
}

// Frame returns the latest JPEG frame, or nil before the first one.
func (p *Previewer) Frame() []byte {
	p.mu.Lock()
	hub := p.hub
	p.mu.Unlock()
	if hub == nil {
		return nil
	}
	return hub.latest()
}

// Err reports why no picture comes, when it is not simply "not yet".
func (p *Previewer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Stop ends the preview.
func (p *Previewer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Previewer) stopLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	if p.hub != nil {
		p.hub.Close()
	}
	p.cmd, p.hub, p.source, p.err = nil, nil, "", nil
}
