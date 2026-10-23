package asr

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"ottr/internal/models"
)

// liveChunker turns the audio of a recording into subtitles while it records.
//
// sherpa-onnx's binary cannot read a WAV stream from a pipe (its reader
// seeks), so audio is buffered and decoded a chunk at a time, cut at a quiet
// point so words are not split. Each chunk is recognized on its own, which is
// coarser than the pass over the finished file: the live text is a
// provisional preview, and the post-recording transcript replaces it.
type liveChunker struct {
	bin      string
	model    *models.ASRModel
	rate     int
	channels int
	emit     func(LiveSubtitle)

	mu     sync.Mutex
	buf    []int16 // mono samples not yet decoded
	offset int64   // ms of audio decoded before buf
	closed bool

	queue chan chunkJob
	done  chan struct{}
}

const (
	// liveChunkMs is the audio that waits before a chunk is decoded.
	liveChunkMs = 3000
	// liveMaxChunkMs bounds a chunk when no quiet point comes.
	liveMaxChunkMs = 5500
)

// StartLive returns a sink for the PCM of a recording, which decodes it into
// subtitles as it comes and calls emit, when set, for each new subtitle.
// Close the sink when the recording ends.
func (s *Service) StartLive(rate, channels int, emit func(LiveSubtitle)) (io.WriteCloser, error) {
	bin, err := s.models.FindSherpaBin()
	if err != nil {
		return nil, err
	}
	mdl, err := s.models.FindASRModel()
	if err != nil {
		return nil, err
	}
	if rate <= 0 {
		rate = 48000
	}
	if channels <= 0 {
		channels = 1
	}
	l := &liveChunker{
		bin: bin, model: mdl, rate: rate, channels: channels, emit: emit,
		queue: make(chan chunkJob, 2), done: make(chan struct{}),
	}
	go l.work()
	return l, nil
}

// Write takes interleaved f32le samples at the sink's rate.
func (l *liveChunker) Write(pcm []byte) (int, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return len(pcm), nil
	}
	frames := len(pcm) / 4 / l.channels
	for i := 0; i < frames; i++ {
		var sum float32
		for c := 0; c < l.channels; c++ {
			off := (i*l.channels + c) * 4
			sum += math.Float32frombits(binary.LittleEndian.Uint32(pcm[off:]))
		}
		v := sum / float32(l.channels) * 32767
		l.buf = append(l.buf, int16(max(-32768, min(32767, v))))
	}
	var chunk []int16
	if l.ms(len(l.buf)) >= liveChunkMs {
		cut := l.cutPoint()
		chunk, l.buf = l.buf[:cut], append([]int16(nil), l.buf[cut:]...)
	}
	offset := l.offset
	l.offset += l.ms(len(chunk))
	l.mu.Unlock()
	if len(chunk) > 0 {
		l.decode(chunk, offset)
	}
	return len(pcm), nil
}

// Close decodes what is still buffered and waits for it.
func (l *liveChunker) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	rest, offset := l.buf, l.offset
	l.buf = nil
	l.mu.Unlock()
	if len(rest) > 0 {
		l.decode(rest, offset)
	}
	close(l.queue)
	<-l.done
	return nil
}

func (l *liveChunker) ms(samples int) int64 {
	return int64(samples) * 1000 / int64(l.rate)
}

// cutPoint ends the chunk at the quietest 20 ms in its last third, so that a
// word is not cut in half, and never past liveMaxChunkMs.
func (l *liveChunker) cutPoint() int {
	window := max(l.rate/50, 1)
	limit := min(int(int64(liveMaxChunkMs)*int64(l.rate)/1000), len(l.buf))
	best, bestEnergy := limit, int64(math.MaxInt64)
	for start := len(l.buf) / 3; start+window <= limit; start += window {
		var energy int64
		for _, s := range l.buf[start : start+window] {
			energy += int64(max(s, -s))
		}
		if energy < bestEnergy {
			best, bestEnergy = start, energy
		}
	}
	return best
}

// decode queues one chunk for the worker, which recognizes chunks in order
// on a goroutine of its own so that recording never waits. A chunk arriving
// while the queue is full is dropped: the live text is a preview.
func (l *liveChunker) decode(chunk []int16, offsetMs int64) {
	select {
	case l.queue <- chunkJob{chunk, offsetMs}:
	default:
	}
}

func (l *liveChunker) work() {
	defer close(l.done)
	for job := range l.queue {
		for _, seg := range l.recognize(job.chunk) {
			if l.emit != nil {
				l.emit(LiveSubtitle{
					StartMs: job.offsetMs + seg.StartMs,
					EndMs:   job.offsetMs + seg.EndMs,
					Text:    seg.Text,
				})
			}
		}
	}
}

type chunkJob struct {
	chunk    []int16
	offsetMs int64
}

func (l *liveChunker) recognize(chunk []int16) []Segment {
	f, err := os.CreateTemp("", "ottr-live-*.wav")
	if err != nil {
		return nil
	}
	path := f.Name()
	defer os.Remove(path)
	err = writeWAV(f, l.rate, chunk)
	f.Close()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	segs, err := RunSherpa(ctx, l.bin, l.model, path)
	if err != nil {
		return nil
	}
	return segs
}

// writeWAV writes mono 16-bit PCM as a WAV file sherpa-onnx reads.
func writeWAV(w io.Writer, rate int, samples []int16) error {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // mono
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	if _, err := w.Write(h); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// BroadcastLive sends a live subtitle to the pages of the app.
func BroadcastLive(sub LiveSubtitle) { LiveEvent.Broadcast(sub) }
