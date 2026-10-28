package capture

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ottr/internal/tools"
)

// Previewer shows a source before it is recorded: one still frame, grabbed the
// way a recording grabs video. It is a snapshot rather than a stream because
// grabbing the desktop continuously — even at five frames a second — holds the
// desktop compositor busy enough to make the system pointer stutter.
type Previewer struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	frame  []byte
	seq    uint64 // counts the frames grabbed, so a caller decodes each once
	source string
	err    error
}

// NewPreviewer creates a previewer. Nothing runs until Show.
func NewPreviewer() *Previewer { return &Previewer{} }

// Show draws the source given from now on: the caller gets the picture from
// Frame once FFmpeg has written it, a moment later.
func (p *Previewer) Show(sourceID string) {
	p.mu.Lock()
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill() // the newer picture wins
		p.cmd = nil
	}
	p.source = sourceID
	p.mu.Unlock()
	p.capture(sourceID)
}

func (p *Previewer) capture(sourceID string) {
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
	out := filepath.Join(os.TempDir(), fmt.Sprintf("ottr-preview-%d.jpg", time.Now().UnixNano()))
	defer os.Remove(out)

	// No captured pointer: the desktop's own pointer already sits over this
	// picture, and a second one would flicker as it lags behind.
	args := append([]string{"-hide_banner", "-loglevel", "error", "-draw_mouse", "0"}, video...)
	args = append(args, "-frames:v", "1", "-vf", previewFilter, "-f", "image2", "-y", out)
	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		p.fail(fmt.Errorf("start ffmpeg: %w", err))
		return
	}

	p.mu.Lock()
	p.cmd = cmd
	p.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()
		frame, readErr := os.ReadFile(out)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.cmd != cmd {
			return // a newer Show replaced this one
		}
		p.cmd = nil
		if waitErr != nil || readErr != nil || len(frame) == 0 {
			p.err = previewError(errors.Join(waitErr, readErr), stderr.String())
			return
		}
		p.frame, p.err = frame, nil
		p.seq++
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
		return fmt.Errorf("capture stopped: %w", waitErr)
	default:
		return errors.New("the capture wrote no picture")
	}
}

func (p *Previewer) fail(err error) {
	p.mu.Lock()
	p.err = err
	p.cmd = nil
	p.mu.Unlock()
}

// Frame returns the latest frame, or nil before the first one.
func (p *Previewer) Frame() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frame
}

// Seq counts the frames grabbed so far: a caller that shows the frame decodes
// it once, when the count changes.
func (p *Previewer) Seq() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.seq
}

// Pending reports whether a still frame is being grabbed: the window asks for
// frames until it lands, and then stops.
func (p *Previewer) Pending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil
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
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	p.cmd, p.frame, p.source, p.err = nil, nil, "", nil
	p.seq++
}
