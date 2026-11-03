// Package player decodes media for MyGo's native video and audio players.
package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/tools"
)

const audioRate = 48000

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

// AudioDecoder decodes stereo float PCM for audio.Player.
type AudioDecoder struct {
	path string

	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	out    io.ReadCloser
	stderr *bytes.Buffer
	pos    int64
	length int64
	mu     sync.Mutex
}

func OpenAudio(ctx context.Context, path string, duration time.Duration) (*AudioDecoder, error) {
	ctx, cancel := context.WithCancel(ctx)
	d := &AudioDecoder{path: path, ctx: ctx, cancel: cancel, length: -1}
	if duration > 0 {
		d.length = int64(math.Round(duration.Seconds() * audioRate))
	}
	if err := d.start(0); err != nil {
		cancel()
		return nil, err
	}
	return d, nil
}

func (d *AudioDecoder) start(frame int64) error {
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		return err
	}
	args := []string{"-loglevel", "error"}
	if frame > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", float64(frame)/audioRate))
	}
	args = append(args, "-i", d.path, "-map", "0:a:0", "-vn", "-f", "f32le",
		"-acodec", "pcm_f32le", "-ar", fmt.Sprint(audioRate), "-ac", "2", "pipe:1")
	d.cmd = exec.CommandContext(d.ctx, ffmpeg, args...)
	hideWindow(d.cmd)
	d.stderr = &bytes.Buffer{}
	d.cmd.Stderr = d.stderr
	d.out, err = d.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	d.pos = frame
	return d.cmd.Start()
}

func (d *AudioDecoder) SampleRate() int { return audioRate }
func (d *AudioDecoder) Channels() int   { return 2 }
func (d *AudioDecoder) Len() int64      { return d.length }

func (d *AudioDecoder) Read(buf []float32) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.out == nil {
		return 0, io.EOF
	}
	raw := make([]byte, len(buf)*4)
	n, err := io.ReadFull(d.out, raw)
	n -= n % 8 // whole stereo frames only
	for i := 0; i < n; i += 4 {
		buf[i/4] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i:]))
	}
	d.pos += int64(n / 8)
	if err != nil {
		if err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return n / 4, err
	}
	return n / 4, nil
}

func (d *AudioDecoder) SetPosition(frame int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if frame == d.pos {
		return nil
	}
	d.closeProcess()
	return d.start(max(frame, 0))
}

func (d *AudioDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cancel()
	d.closeProcess()
	return nil
}

func (d *AudioDecoder) closeProcess() {
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
