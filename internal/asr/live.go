package asr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"

	"ottr/internal/asr/native"
	"ottr/internal/models"
)

const (
	// liveQueueAudio is how much audio may wait for the recognizer before live
	// audio is dropped; liveWarnAudio is how much must be dropped before the
	// window says the live text is behind. The recording itself keeps every
	// sample: this only ever costs preview text.
	liveQueueAudio = 20 * 1000 // ms
	liveWarnAudio  = 4 * 1000  // ms

	// utteranceTailMs keeps a line on screen long enough to read after its
	// last word.
	utteranceTailMs = 250
)

// livestream recognizes the audio of one recording with one recognizer whose
// state runs the whole recording, so words are not cut at chunk boundaries.
type livestream struct {
	emit func(LiveSubtitle)

	// queue holds converted 16 kHz mono audio for the owner goroutine.
	queue chan []float32
	done  chan struct{}

	mu      sync.Mutex
	closed  bool
	queued  int64 // ms of audio waiting for the owner
	dropped int64 // ms of audio dropped for being behind
	warned  bool
}

// StartLive returns a sink for the PCM of a recording, which recognizes it into
// subtitles as it comes and calls emit for each revision, provisional ones
// included. Close the sink when the recording ends.
//
// The sink never blocks the capture thread: audio the recognizer cannot keep up
// with is dropped after a warning, and the post-recording pass over the file is
// what the transcript is built from.
func (s *Service) StartLive(rate, channels int, emit func(LiveSubtitle)) (io.WriteCloser, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	mdl, err := s.models.FindASRModel()
	if err != nil {
		return nil, err
	}
	l := &livestream{emit: emit, queue: make(chan []float32, 64), done: make(chan struct{})}
	// Conversion happens on the capture thread, so the owner only ever sees
	// 16 kHz mono samples, and its Flush in Close flushes the last of them.
	sink := &liveSink{l: l, conv: NewConverter(rate, channels)}
	go l.work(mdl)
	return sink, nil
}

type liveSink struct {
	l    *livestream
	conv *Converter
}

// Write takes interleaved f32le samples at the sink's rate. It never blocks on
// recognition.
func (s *liveSink) Write(pcm []byte) (int, error) {
	s.l.push(s.conv.Feed(f32le(pcm)))
	return len(pcm), nil
}

// Close flushes the converter, publishes the last revision and waits for the
// recognizer to be freed.
func (s *liveSink) Close() error {
	s.l.push(s.conv.Flush())
	s.l.close()
	return nil
}

// push hands samples to the owner goroutine, or drops them when live text is
// already behind. It never waits: the queue is bounded by audio duration and by
// capacity, and either bound being hit drops the buffer. Samples are copied
// because the caller reuses its buffer.
func (l *livestream) push(samples []float32) {
	if len(samples) == 0 {
		return
	}
	ms := int64(len(samples)) * 1000 / 16000
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	fits := l.queued+ms <= liveQueueAudio
	if fits {
		l.queued += ms
	}
	l.mu.Unlock()
	if fits {
		select {
		case l.queue <- samples:
			return
		default:
		}
		l.mu.Lock()
		l.queued -= ms
		l.mu.Unlock()
	}
	l.mu.Lock()
	l.dropped += ms
	warn := !l.warned && l.dropped >= liveWarnAudio
	if warn {
		l.warned = true
	}
	l.mu.Unlock()
	if warn && l.emit != nil {
		l.emit(LiveSubtitle{Warn: "Live text is behind the recording; it catches up when the audio pauses"})
	}
}

// close marks the input finished once and waits for the recognizer to go.
func (l *livestream) close() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		<-l.done
		return
	}
	l.closed = true
	l.mu.Unlock()
	close(l.queue)
	<-l.done
}

// work owns the recognizer and the stream: every native call happens here.
func (l *livestream) work(mdl *models.ASRModel) {
	defer close(l.done)
	if err := l.run(mdl); err != nil && l.emit != nil {
		l.emit(LiveSubtitle{Error: err.Error()})
	}
}

func (l *livestream) run(mdl *models.ASRModel) error {
	st, err := newStreamer(*mdl)
	if err != nil {
		return err
	}
	defer st.destroy()
	for samples := range l.queue {
		l.mu.Lock()
		l.queued -= int64(len(samples)) * 1000 / 16000
		l.mu.Unlock()
		st.feed(samples)
		if st.endpoint() {
			l.revise(st, true)
			st.reset()
			continue
		}
		l.revise(st, false)
	}
	// The queue was closed after the sink flushed its converter. Decode the last
	// of the audio before reading the result, so the final line has every word.
	st.finish()
	l.revise(st, true)
	return nil
}

// revise publishes the stream's current utterance as the current line: a
// provisional revision while it goes on, a final one at its end. A revision
// that changes nothing is not published.
func (l *livestream) revise(st *streamer, final bool) {
	u := st.result()
	if u.text == "" {
		return // nothing recognized in this utterance yet
	}
	sub := LiveSubtitle{
		Utterance: st.utterance,
		StartMs:   u.startMs,
		EndMs:     u.endMs,
		Text:      u.text,
		Final:     final,
	}
	if sub == st.published {
		return
	}
	st.published = sub
	if l.emit != nil {
		l.emit(sub)
	}
}

// BroadcastLive sends a live subtitle to the pages of the app.
func BroadcastLive(sub LiveSubtitle) { LiveEvent.Broadcast(sub) }

// f32le decodes interleaved little-endian float samples.
func f32le(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// streamer owns one recognizer and its stream, and turns what it recognizes
// into utterances on the recording's timeline: after an endpoint the stream's
// own timestamps start again, so the audio fed so far moves them back onto the
// recording.
type streamer struct {
	rec    *native.Recognizer
	stream *native.Stream

	fed       int64 // samples fed to the stream
	baseMs    int64 // recording time where the current utterance began
	utterance int64 // 1 for the first utterance, then counting up
	published LiveSubtitle
}

func newStreamer(mdl models.ASRModel) (*streamer, error) {
	rec, err := native.NewRecognizer(mdl.Tokens, mdl.Encoder, mdl.Decoder, mdl.Joiner, 2, true, 2.4, 20)
	if err != nil {
		return nil, fmt.Errorf("sherpa-onnx could not load the model in %s: %w", mdl.Dir, err)
	}
	stream := rec.Stream()
	if stream == nil {
		rec.Destroy()
		return nil, errors.New("sherpa-onnx could not open a recognition stream")
	}
	s := &streamer{rec: rec, stream: stream}
	s.startUtterance()
	return s, nil
}

// feed takes mono 16 kHz samples and decodes everything they make ready.
func (s *streamer) feed(samples []float32) {
	s.stream.Accept(16000, samples)
	s.fed += int64(len(samples))
	s.stream.Decode()
}

// endpoint reports whether the recognizer found the end of the utterance.
func (s *streamer) endpoint() bool { return s.stream.Endpoint() }

// reset starts a new utterance without losing the recording's timeline.
func (s *streamer) reset() {
	s.stream.Reset()
	s.startUtterance()
}

func (s *streamer) startUtterance() {
	s.utterance++
	s.baseMs = s.fed * 1000 / 16000
	s.published = LiveSubtitle{}
}

// finish decodes the audio still held after the last feed, which is what makes
// the final result complete.
func (s *streamer) finish() {
	s.stream.Finish()
	s.stream.Decode()
}

// destroy frees the stream, then the recognizer it belongs to.
func (s *streamer) destroy() {
	s.stream.Destroy()
	s.rec.Destroy()
}

// close flushes the last of the audio, then frees both native objects.
func (s *streamer) close() {
	s.finish()
	s.destroy()
}

// utterance is what the current utterance has been recognized as.
type utterance struct {
	text           string
	startMs, endMs int64
	segments       []Segment // grouped lines, for a transcript
}

// result reads the current utterance, on the recording's timeline and grouped
// into lines with the same rules a stored transcript uses.
func (s *streamer) result() utterance {
	text, tokens, stamps := s.stream.Result()
	u := utterance{text: strings.TrimSpace(text), startMs: s.baseMs, endMs: s.baseMs}
	if len(stamps) > 0 {
		u.startMs = s.baseMs + int64(stamps[0]*1000)
		u.endMs = s.baseMs + int64(stamps[len(stamps)-1]*1000) + utteranceTailMs
	}
	if u.endMs < u.startMs+utteranceTailMs {
		u.endMs = u.startMs + utteranceTailMs
	}
	if len(tokens) > 0 {
		shifted := make([]float64, len(stamps))
		for i, t := range stamps {
			shifted[i] = t + float64(s.baseMs)/1000
		}
		for _, seg := range GroupTokens(tokens, shifted) {
			u.segments = append(u.segments, seg)
		}
	}
	return u
}
