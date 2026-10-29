// Package capture records the screen and audio into media files.
//
// Video comes from the platform's native capture — Windows Graphics Capture on
// Windows — as raw BGRA frames. The preview is encoded in-process; recordings
// are still encoded and muxed by FFmpeg until internal/media replaces it, and
// audio is captured in-process from WASAPI (system loopback and microphone).
// That same PCM goes to a Live sink, which recognizes speech while the
// recording runs.
package capture

import (
	"bytes"
	"context"
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

// AudioDevice is an active speaker or microphone endpoint.
type AudioDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Config selects what to record.
type Config struct {
	SourceID     string `json:"sourceId"`
	SystemAudio  bool   `json:"systemAudio"`
	Mic          bool   `json:"mic"`
	SpeakerID    string `json:"speakerId,omitempty"`
	MicrophoneID string `json:"microphoneId,omitempty"`
}

// State is the recording status, broadcast on StateChanged.
type State struct {
	Recording bool      `json:"recording"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	Error     string    `json:"error,omitempty"`
	// Live is the current line of live text, as the live sink last revised
	// it. Nil when there is none.
	Live *LiveLine `json:"live,omitempty"`
}

// LiveLine is one revision of the line being spoken, mirrored from the live
// sink so that a window polling State sees the same line it would see from the
// event: the utterance number says which line it is, and Final says whether it
// will change again.
type LiveLine struct {
	Utterance int64  `json:"utterance"`
	StartMs   int64  `json:"startMs"`
	Text      string `json:"text"`
	Final     bool   `json:"final"`

	// Warn explains, when it is set, why live text is behind or stopped.
	Warn string `json:"warn,omitempty"`
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
	Backend Backend          // the platform's capture; Sources reads it
	Dir     string           // where recordings are written
	Lib     *library.Service // finished recordings land here
	ASR     Transcriber      // optional auto-transcriber

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
	preview  *preview
	video    *videoInput
	cancel   context.CancelFunc
	errMu    sync.Mutex
	errCause error
	done     chan error
	sink     *liveFeed
	live     *LiveLine
}

// SetLiveText records the current live subtitle for State, called by the live
// sink as it recognizes one. An empty text with a warning replaces the line
// with that warning instead.
func (r *Recorder) SetLiveText(line *LiveLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		r.current.live = line
	}
}

// Sources lists what can be recorded.
func (r *Recorder) Sources() ([]Source, error) {
	if r.Backend == nil {
		return nil, errors.New("no capture backend is configured")
	}
	return r.Backend.Sources(context.Background())
}

// AudioDevices lists active speakers and microphones, default device first.
func (r *Recorder) AudioDevices() (speakers, microphones []AudioDevice, err error) {
	return audioDevices()
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
	return State{Recording: true, StartedAt: r.current.started, Live: r.current.live}
}

// Start begins a recording. It fails without side effects when a device is
// unavailable or another recording is active.
func (r *Recorder) Start(cfg Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		return errors.New("a recording is already in progress")
	}
	if r.Backend == nil {
		return errors.New("no capture backend is configured")
	}
	// The stream is opened before anything else, so that a permission denial
	// or a disappeared source fails without side effects.
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := r.Backend.Open(ctx, StreamConfig{SourceID: cfg.SourceID, FPS: recordFPS, Cursor: true})
	if err != nil {
		cancel()
		return err
	}
	frame, err := stream.Next(ctx)
	if err != nil {
		cancel()
		stream.Close()
		return fmt.Errorf("capture: %w", err)
	}
	width, height := evenSize(frame.Width, frame.Height)
	video, err := newVideoInput(width, height)
	if err != nil {
		cancel()
		stream.Close()
		return err
	}
	stopCapture := func() {
		cancel()
		stream.Close()
		video.close()
	}
	video.offer(fitFrame(frame, width, height))

	sink := newLiveFeed(r.Live)
	var audio []*audioStream
	if cfg.SystemAudio {
		as, err := startAudioStream(true, cfg.SpeakerID, sink)
		if err != nil {
			stopAudio(audio)
			stopCapture()
			return fmt.Errorf("system audio: %w", err)
		}
		audio = append(audio, as)
	}
	if cfg.Mic {
		as, err := startAudioStream(false, cfg.MicrophoneID, sink)
		if err != nil {
			stopAudio(audio)
			stopCapture()
			return fmt.Errorf("microphone: %w", err)
		}
		audio = append(audio, as)
	}
	preview := r.startPreview(cfg.SourceID)
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		preview.close()
		stopAudio(audio)
		stopCapture()
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
		preview.close()
		stopAudio(audio)
		stopCapture()
		return err
	}
	cmd := exec.Command(ffmpeg, buildArgs(video.args(width, height), specs, file)...)
	hideWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		preview.close()
		stopAudio(audio)
		stopCapture()
		return err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		preview.close()
		stopAudio(audio)
		stopCapture()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	sess := &session{
		file: file, started: time.Now(), cmd: cmd, stdin: stdin, stderr: stderr,
		audio: audio, preview: preview, cancel: cancel, video: video,
		done: make(chan error, 1), sink: sink,
	}
	r.current = sess
	// The stream feeds the encoder from a goroutine of its own: a frame the
	// encoder has no room for is dropped, not waited on. A source that dies
	// ends the session the same way a dying encoder does — one owner, one
	// outcome, never a partial file reported as a success.
	go func() {
		defer stream.Close()
		for {
			f, err := stream.Next(ctx)
			if err != nil {
				if ctx.Err() == nil && !errors.Is(err, io.EOF) {
					sess.fail(fmt.Errorf("capture stopped: %w", err))
				}
				return
			}
			video.offer(fitFrame(f, width, height))
		}
	}()
	go func() {
		err := cmd.Wait()
		r.mu.Lock()
		orphaned := r.current == sess
		if orphaned {
			r.current = nil
		}
		r.mu.Unlock()
		if orphaned {
			// The encoder or the source died on its own: discard the
			// partial file.
			sess.cleanup()
			os.Remove(sess.file)
			cause := waitError(err, stderr)
			if cause == "" || err == nil {
				cause = sess.errText()
			}
			StateChanged.Broadcast(State{Error: "recording stopped unexpectedly: " + cause})
		}
		sess.done <- err
	}()
	StateChanged.Broadcast(State{Recording: true, StartedAt: sess.started})
	return nil
}

// startPreview runs the recording's own preview: a second native stream of the
// same source, encoded at the preview's slow rate. It is a picture only, so a
// source that cannot be previewed does not stop the recording.
func (r *Recorder) startPreview(sourceID string) *preview {
	pv := newPreview()
	if r.Backend == nil || sourceID == "" {
		return pv
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := r.Backend.Open(ctx, StreamConfig{SourceID: sourceID, FPS: previewFPS, Cursor: true})
	if err != nil {
		cancel()
		return pv
	}
	pv.own(cancel)
	go func() {
		defer stream.Close()
		for {
			f, err := stream.Next(ctx)
			if err != nil {
				return
			}
			pv.offer(f)
		}
	}()
	return pv
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
	sess.cancel()
	sess.video.close()

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

// fail records why the session ended and stops the encoder, whose exit the
// watchdog then reports.
func (s *session) fail(cause error) {
	s.errMu.Lock()
	if s.errCause == nil {
		s.errCause = cause
	}
	s.errMu.Unlock()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}

// errText is the cause recorded by fail, if there is one.
func (s *session) errText() string {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.errCause == nil {
		return "the encoder exited"
	}
	return s.errCause.Error()
}

func (s *session) cleanup() {
	stopAudio(s.audio)
	s.cancel()
	s.video.close()
	s.preview.close()
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

// Preview returns the latest JPEG frame of the active recording's preview, nil
// before the first frame or when nothing records.
func (r *Recorder) Preview() []byte {
	pv := r.previewOf()
	if pv == nil {
		return nil
	}
	return pv.Frame()
}

// previewOf returns the preview of the active recording, if any.
func (r *Recorder) previewOf() *preview {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return nil
	}
	return r.current.preview
}
