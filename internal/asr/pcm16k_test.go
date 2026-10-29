package asr

import (
	"math"
	"testing"
)

func TestConverterPassesA16kMonoStreamThrough(t *testing.T) {
	c := NewConverter(16000, 1)
	in := []float32{0.1, -0.2, 0.3, -0.4}
	out := c.Feed(in)
	if len(out) != len(in) {
		t.Fatalf("want %d samples, got %d", len(in), len(out))
	}
	for i := range in {
		if math.Abs(float64(out[i]-in[i])) > 1e-6 {
			t.Errorf("sample %d: want %v, got %v", i, in[i], out[i])
		}
	}
	if rest := c.Flush(); len(rest) != 0 {
		t.Errorf("flush invented %d samples", len(rest))
	}
}

func TestConverterAveragesChannels(t *testing.T) {
	c := NewConverter(16000, 2)
	out := c.Feed([]float32{1, 0, 0, 1, -1, -1})
	want := []float32{0.5, 0.5, -1}
	if len(out) != len(want) {
		t.Fatalf("want %d samples, got %d: %v", len(want), len(out), out)
	}
	for i := range want {
		if math.Abs(float64(out[i]-want[i])) > 1e-6 {
			t.Errorf("sample %d: want %v, got %v", i, want[i], out[i])
		}
	}
}

func TestConverterResamples48kTo16k(t *testing.T) {
	c := NewConverter(48000, 1)
	// 30 ms of a 48 kHz square wave: three input samples per output sample.
	in := make([]float32, 1440)
	for i := range in {
		if i%2 == 0 {
			in[i] = 1
		}
	}
	out := c.Feed(in)
	if len(out) != 480 {
		t.Fatalf("want 480 samples out, got %d", len(out))
	}
}

// A stream resampled in pieces must be the same audio resampled in one go: no
// call may reset the resampler or lose its fractional position.
func TestConverterKeepsItsPositionAcrossSplits(t *testing.T) {
	in := make([]float32, 4410) // 44100 Hz: a non-integer ratio
	for i := range in {
		in[i] = float32(math.Sin(float64(i) * 0.05))
	}

	whole := NewConverter(44100, 1).Feed(in)
	together := append(whole, NewConverter(44100, 1).Flush()...)

	split := NewConverter(44100, 1)
	var pieces []float32
	for i := 0; i < len(in); i += 997 { // ragged splits, on purpose
		end := min(i+997, len(in))
		pieces = append(pieces, split.Feed(in[i:end])...)
	}
	if got, want := len(pieces), len(in)*16000/44100; got != want {
		t.Fatalf("piecewise conversion gave %d samples, a whole one gives %d", got, want)
	}
	for i := range pieces {
		if math.Abs(float64(pieces[i]-together[i])) > 1e-6 {
			t.Fatalf("sample %d differs: piecewise %v, whole %v", i, pieces[i], together[i])
		}
	}
}

func TestConverterKeepsPositionMidBuffer(t *testing.T) {
	// 8 kHz halves the rate, so every other input sample is an output sample
	// and the position lands on an input frame each time; 44.1 kHz does not,
	// which is what this checks: two calls with a fractional boundary lose
	// nothing.
	a, b := NewConverter(44100, 1), NewConverter(44100, 1)
	in := make([]float32, 1000)
	for i := range in {
		in[i] = float32(i)
	}
	one := a.Feed(in)
	one = append(one, a.Flush()...)
	two := b.Feed(in[:501])
	two = append(two, b.Feed(in[501:])...)
	two = append(two, b.Flush()...)
	if len(one) != len(two) {
		t.Fatalf("split conversion gave %d samples, one call gave %d", len(two), len(one))
	}
}

func TestConverterEmptyFeedChangesNothing(t *testing.T) {
	c := NewConverter(44100, 1)
	c.Feed([]float32{0.5, 0.5, 0.5})
	if out := c.Feed(nil); len(out) != 0 {
		t.Fatalf("an empty feed produced %d samples", len(out))
	}
	if out := c.Flush(); len(out) == 0 {
		t.Fatal("flush after an empty feed lost the buffered audio")
	}
}

func TestConverterClips(t *testing.T) {
	c := NewConverter(16000, 2)
	out := c.Feed([]float32{4, 4, -4, -4})
	for i, v := range out {
		if v > 1 || v < -1 {
			t.Errorf("sample %d is %v, outside [-1, 1]", i, v)
		}
	}
}

func TestConverterFlushDoesNotInventAudio(t *testing.T) {
	// 48 kHz: 3 input frames are exactly one output frame, so the flush of the
	// longest stream that fits is one sample, never more.
	c := NewConverter(48000, 1)
	total := len(c.Feed([]float32{0.1, 0.2, 0.3}))
	total += len(c.Flush())
	if total != 1 {
		t.Fatalf("3 input frames gave %d output samples, want 1", total)
	}
	if out := NewConverter(48000, 1).Flush(); len(out) != 0 {
		t.Fatalf("flush of nothing gave %d samples", len(out))
	}
}
