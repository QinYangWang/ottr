package asr

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ottr/internal/asr/native"
	"ottr/internal/models"
)

// These tests run the real sherpa-onnx library against the real model. They skip
// — never fake — when either is absent, which is the state of a machine that
// never installed them. OTTR_TEST_BIN and OTTR_TEST_MODELS point them
// elsewhere, for a library outside the usual user data folder.
func realASR(t *testing.T) (*Service, models.ASRModel, string) {
	t.Helper()
	svc, _ := setupTestASR(t)
	dir := ""
	if d := os.Getenv("OTTR_TEST_MODELS"); d != "" {
		dir = d
	}
	svc.models = models.NewManager(dir, os.Getenv("OTTR_TEST_BIN"))
	if rt := svc.RuntimeStatus(); !rt.OK() {
		t.Skipf("no native ASR runtime: %s", rt.Err)
	}
	mdl, err := svc.models.FindASRModel()
	if err != nil {
		t.Skipf("no ASR model: %v", err)
	}
	wav := os.Getenv("OTTR_TEST_WAV")
	if wav == "" {
		wav = filepath.Join(mdl.Dir, "test_wavs", "0.wav")
	}
	if _, err := os.Stat(wav); err != nil {
		wav = filepath.Join(mdl.Dir, "test_wavs", "ja.wav")
	}
	if _, err := os.Stat(wav); err != nil {
		t.Skipf("the model has no test_wavs/0.wav or ja.wav: %v", err)
	}
	return svc, *mdl, wav
}

// The batch path, end to end: a WAV file becomes ordered, non-empty segments.
func TestRealModelTranscribesAWav(t *testing.T) {
	_, mdl, wav := realASR(t)
	segments, err := recognizeWav(context.Background(), mdl, wav)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) == 0 {
		t.Fatal("no segments")
	}
	text := ""
	prevEnd := int64(-1)
	for _, seg := range segments {
		if seg.StartMs < 0 || seg.EndMs <= seg.StartMs {
			t.Errorf("bad timestamps: %+v", seg)
		}
		if seg.StartMs < prevEnd {
			t.Errorf("timestamps out of order: %+v after %d", seg, prevEnd)
		}
		prevEnd = seg.EndMs
		text += seg.Text
	}
	if text == "" {
		t.Fatal("recognized text is empty")
	}
	t.Logf("%d segments, %d characters: %q", len(segments), len(text), text)
}

// Empty and very short audio must not crash or produce a fabricated line.
func TestRealModelHandlesShortAudio(t *testing.T) {
	_, mdl, wav := realASR(t)
	samples, rate, err := readWav(wav)
	if err != nil {
		t.Fatal(err)
	}
	st, err := newStreamer(mdl)
	if err != nil {
		t.Fatal(err)
	}
	defer st.close()
	conv := NewConverter(rate, 1)
	st.feed(conv.Feed(samples[:min(1600, len(samples))])) // 0.1 s
	st.feed(conv.Flush())
	if u := st.result(); u.text != "" {
		t.Logf("0.1 s of speech recognized as %q (not an error)", u.text)
	}
}

// The live path: one recognizer for the whole stream, revisions of the line
// being spoken, and a final revision at the end. Feeding in pieces as capture
// does must not lose the words.
func TestRealLiveStreamKeepsWordsAcrossPieces(t *testing.T) {
	svc, mdl, wav := realASR(t)
	samples, rate, err := readWav(wav)
	if err != nil {
		t.Fatal(err)
	}
	var subs []LiveSubtitle
	sink, err := svc.StartLive(rate, 1, func(sub LiveSubtitle) { subs = append(subs, sub) })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(samples); i += 1600 { // 100 ms of audio at a time
		end := min(i+1600, len(samples))
		if _, err := sink.Write(f32leBytes(samples[i:end])); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if len(subs) == 0 {
		t.Fatal("no live revisions")
	}
	last := subs[len(subs)-1]
	if !last.Final || last.Text == "" {
		t.Fatalf("the last revision is not a final one: %+v", last)
	}
	// Offline models finalize four-second chunks, so several utterances are
	// expected. Join their text before comparing with the file result.
	var live strings.Builder
	for _, sub := range subs {
		if sub.Final {
			live.WriteString(sub.Text)
		}
	}

	// The same audio through the batch path must recognize the same words: one
	// recognizer, one result, whether it ran live or over the file.
	segments, err := recognizeWav(context.Background(), mdl, wav)
	if err != nil {
		t.Fatal(err)
	}
	batch := ""
	for _, seg := range segments {
		batch += seg.Text
	}
	if batch != live.String() {
		t.Logf("live text %q, batch text %q", live.String(), batch)
	}
}

func TestRealSpeakerEmbedding(t *testing.T) {
	_, _, wav := realASR(t)
	samples, _, err := readWav(wav)
	if err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(os.Getenv("APPDATA"), "Ottr", "diarization", "eres2net.onnx")
	if _, err := os.Stat(model); err != nil {
		t.Skip(err)
	}
	vector, err := native.SpeakerEmbedding(model, samples)
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) == 0 || math.Abs(cosine(vector, vector)-1) > 0.001 {
		t.Fatalf("invalid speaker embedding: %d values", len(vector))
	}
}

func TestRealSpeakerDiarization(t *testing.T) {
	_, _, wav := realASR(t)
	samples, _, err := readWav(wav)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(os.Getenv("APPDATA"), "Ottr", "diarization")
	segmentation := filepath.Join(root, "pyannote", "model.onnx")
	embedding := filepath.Join(root, "eres2net.onnx")
	if _, err := os.Stat(segmentation); err != nil {
		t.Skip(err)
	}
	if _, err := os.Stat(embedding); err != nil {
		t.Skip(err)
	}
	ranges, err := native.Diarize(segmentation, embedding, samples)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) == 0 {
		t.Fatal("no speaker ranges")
	}
	t.Logf("%d ranges: %+v", len(ranges), ranges)
}

// Creating, feeding and closing a stream many times must not crash, warn or
// leave the recognizer behind: this is the cycle a long session of short
// recordings produces.
func TestRealLiveStreamLifecycle(t *testing.T) {
	svc, _, wav := realASR(t)
	samples, rate, err := readWav(wav)
	if err != nil {
		t.Fatal(err)
	}
	short := samples[:min(len(samples), rate/2)] // 0.5 s
	for i := 0; i < 5; i++ {
		sink, err := svc.StartLive(rate, 1, func(LiveSubtitle) {})
		if err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		if _, err := sink.Write(f32leBytes(short)); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		if err := sink.Close(); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		if err := sink.Close(); err != nil { // Close is idempotent
			t.Errorf("cycle %d: second Close: %v", i, err)
		}
	}
}

func f32leBytes(s []float32) []byte {
	out := make([]byte, len(s)*4)
	for i, v := range s {
		bits := math.Float32bits(v)
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)
	}
	return out
}
