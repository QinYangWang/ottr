// Package player decodes video poster frames for the Library's thumbnails.
// Playback itself is ui/video's engine player; this remains only for the
// first frame of a video, which has no other in-process source yet.
package player

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os/exec"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/tools"
)

// VideoDecoder decodes RGBA frames at 30 fps for ui.VideoPlayer.
type VideoDecoder struct {
	path          string
	width, height int

	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	out    io.ReadCloser
	stderr *bytes.Buffer
}

func OpenVideo(ctx context.Context, path string, width, height int) (*VideoDecoder, error) {
	return OpenVideoAt(ctx, path, width, height, 0)
}

func OpenVideoAt(ctx context.Context, path string, width, height int, at time.Duration) (*VideoDecoder, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("video dimensions are unknown")
	}
	ctx, cancel := context.WithCancel(ctx)
	d := &VideoDecoder{path: path, width: width, height: height, ctx: ctx, cancel: cancel}
	if err := d.start(at); err != nil {
		cancel()
		return nil, err
	}
	return d, nil
}

func (d *VideoDecoder) start(at time.Duration) error {
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		return err
	}
	args := []string{"-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", at.Seconds()))
	}
	args = append(args, "-i", d.path, "-map", "0:v:0", "-an", "-vf", "fps=30",
		"-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")
	d.cmd = exec.CommandContext(d.ctx, ffmpeg, args...)
	hideWindow(d.cmd)
	d.stderr = &bytes.Buffer{}
	d.cmd.Stderr = d.stderr
	d.out, err = d.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	return d.cmd.Start()
}

func (d *VideoDecoder) Next() (ui.VideoFrame, error) {
	pix := make([]byte, d.width*d.height*4)
	if _, err := io.ReadFull(d.out, pix); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if waitErr := d.cmd.Wait(); waitErr != nil && d.ctx.Err() == nil {
				return ui.VideoFrame{}, fmt.Errorf("video decode: %w (%s)", waitErr, d.stderr.String())
			}
			return ui.VideoFrame{}, io.EOF
		}
		return ui.VideoFrame{}, err
	}
	img := &image.RGBA{Pix: pix, Stride: d.width * 4, Rect: image.Rect(0, 0, d.width, d.height)}
	return ui.VideoFrame{Image: img, Duration: time.Second / 30}, nil
}

func (d *VideoDecoder) Rewind() error {
	d.closeProcess()
	return d.start(0)
}

func (d *VideoDecoder) Close() error {
	d.cancel()
	d.closeProcess()
	return nil
}

func (d *VideoDecoder) closeProcess() {
	if d.cmd == nil {
		return
	}
	if d.out != nil {
		d.out.Close()
	}
	if d.cmd.Process != nil {
		d.cmd.Process.Kill()
	}
	d.cmd.Wait()
	d.cmd, d.out = nil, nil
}
