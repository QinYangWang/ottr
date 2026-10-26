// Package capture records the screen and audio into media files.
//
// Video comes from the platform's native capture (GDI on Windows) through
// FFmpeg; audio is captured in-process from WASAPI (system loopback and
// microphone) and streamed to FFmpeg over named pipes. FFmpeg encodes and
// muxes the result. The same PCM goes to a Live sink, which recognizes speech
// while the recording runs.
package capture

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"ottr/internal/library"
	"ottr/internal/tools"
)

// Source is a recordable screen, display or window.
type Source struct {
	ID     string `json:"id"`   // screen | display:N | window:<title>
	Kind   string `json:"kind"` // screen | display | window
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"` // size, when the platform knows it
}

// Config selects what to record.
type Config struct {
	SourceID    string `json:"sourceId"`
	SystemAudio bool   `json:"systemAudio"`
	Mic         bool   `json:"mic"`
}

// State is the recording status, broadcast on StateChanged.
type State struct {
	Recording bool      `json:"recording"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	Error     string    `json:"error,omitempty"`
	LiveText  string    `json:"liveText,omitempty"` // the last live subtitle
}

// StateChanged is broadcast when recording starts, stops or fails.
var StateChanged = mygo.NewEvent[State]("recording:state")

// Transcriber starts speech recognition on a media record and returns the
// job that runs it.
type Transcriber interface {
	Transcribe(mediaID int64) (int64, error)
}

// Recorder records one session at a time. It is bound to the frontend as
// "Recorder".
type Recorder struct {
	Dir string           // where recordings are written
	Lib *library.Service // finished recordings land here
	ASR Transcriber      // optional auto-transcriber

	// Live opens the sink that transcribes the audio as it records, or
	// returns nil when there is none. It is called when a recording starts;
	// nil keeps live text off.
	Live LiveStarter

	mu      sync.Mutex
	current *session
}

type session struct {
	file     string
	started  time.Time
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stderr   *bytes.Buffer
	audio    []*audioStream
	preview  *previewHub
	done     chan error
	sink     *liveFeed
	liveText string
}

// SetLiveText records the last live subtitle for State, called by the live
// sink as it recognizes one.
func (r *Recorder) SetLiveText(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		r.current.liveText = text
	}
}

// Sources lists what can be recorded.
func (r *Recorder) Sources() ([]Source, error) {
	return listSources()
}

// State returns the current recording status.
func (r *Recorder) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stateLocked()
}

func (r *Recorder) stateLocked() State {
	if r.current == nil {
		return State{}
	}
	return State{Recording: true, StartedAt: r.current.started, LiveText: r.current.liveText}
}

// Start begins a recording. It fails without side effects when a device is
// unavailable or another recording is active.
func (r *Recorder) Start(cfg Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		return errors.New("a recording is already in progress")
	}
	video, err := resolveSource(cfg.SourceID)
	if err != nil {
		return err
	}
	sink := newLiveFeed(r.Live)
	var audio []*audioStream
	if cfg.SystemAudio {
		as, err := startAudioStream(true, sink)
		if err != nil {
			stopAudio(audio)
			return fmt.Errorf("system audio: %w", err)
		}
		audio = append(audio, as)
	}
	if cfg.Mic {
		as, err := startAudioStream(false, sink)
		if err != nil {
			stopAudio(audio)
			return fmt.Errorf("microphone: %w", err)
		}
		audio = append(audio, as)
	}
	preview, err := newPreviewHub()
	if err != nil {
		stopAudio(audio)
		return err
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		preview.Close()
		stopAudio(audio)
		return err
	}
	name := "Recording " + time.Now().Format("2006-01-02 15.04.05") + ".mp4"
	file := filepath.Join(r.Dir, name)

	specs := make([]audioSpec, len(audio))
	for i, a := range audio {
		specs[i] = audioSpec{rate: a.rate, channels: a.channels, pipe: a.pipe}
	}
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		preview.Close()
		stopAudio(audio)
		return err
	}
	cmd := exec.Command(ffmpeg, buildArgs(video, specs, preview.addr(), file)...)
	hideWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		preview.Close()
		stopAudio(audio)
		return err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		preview.Close()
		stopAudio(audio)
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	sess := &session{
		file: file, started: time.Now(), cmd: cmd, stdin: stdin, stderr: stderr,
		audio: audio, preview: preview, done: make(chan error, 1), sink: sink,
	}
	r.current = sess
	go func() {
		err := cmd.Wait()
		r.mu.Lock()
		orphaned := r.current == sess
		if orphaned {
			r.current = nil
		}
		r.mu.Unlock()
		if orphaned {
			// ffmpeg died on its own: discard the partial file.
			sess.cleanup()
			os.Remove(sess.file)
			StateChanged.Broadcast(State{Error: "recording stopped unexpectedly: " + waitError(err, stderr)})
		}
		sess.done <- err
	}()
	StateChanged.Broadcast(State{Recording: true, StartedAt: sess.started})
	return nil
}

// Stop finishes the recording, verifies the file and adds it to the
// library.
func (r *Recorder) Stop() (library.Media, error) {
	r.mu.Lock()
	sess := r.current
	r.current = nil
	r.mu.Unlock()
	if sess == nil {
		return library.Media{}, errors.New("not recording")
	}
	defer sess.cleanup()

	fmt.Fprintln(sess.stdin, "q") // ask ffmpeg to finalize the file
	sess.stdin.Close()
	stopAudio(sess.audio)
	sess.preview.Close()

	select {
	case err := <-sess.done:
		if err != nil {
			os.Remove(sess.file)
			return library.Media{}, errors.New(waitError(err, sess.stderr))
		}
	case <-time.After(15 * time.Second):
		sess.cmd.Process.Kill()
		<-sess.done
		os.Remove(sess.file)
		return library.Media{}, errors.New("ffmpeg did not stop in time; recording discarded")
	}
	st, err := os.Stat(sess.file)
	if err != nil || st.Size() == 0 {
		os.Remove(sess.file)
		return library.Media{}, errors.New("recording produced no playable file")
	}
	m, err := r.Lib.AddRecording(sess.file)
	if err != nil {
		os.Remove(sess.file)
		return library.Media{}, err
	}
	if r.ASR != nil && m.HasAudio {
		go r.ASR.Transcribe(m.ID)
	}
	StateChanged.Broadcast(State{})
	return m, nil
}

func (s *session) cleanup() {
	stopAudio(s.audio)
	s.preview.Close()
	s.stdin.Close()
	s.sink.close()
}

func stopAudio(streams []*audioStream) {
	for _, a := range streams {
		a.stop()
	}
}

func waitError(err error, stderr *bytes.Buffer) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if stderr.Len() > 0 {
		msg += ": " + stderr.String()
	}
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return msg
}

// Preview returns the latest frame of the active recording's preview, nil
// before the first frame or when nothing records.
func (r *Recorder) Preview() []byte {
	hub := r.previewHubOf()
	if hub == nil {
		return nil
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return hub.frame
}

// previewHubOf returns the preview of the active recording, if any.
func (r *Recorder) previewHubOf() *previewHub {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return nil
	}
	return r.current.preview
}
