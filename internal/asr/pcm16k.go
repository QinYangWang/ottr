package asr

// Converter turns interleaved float32 PCM at any rate and channel count into
// the mono 16 kHz float32 that the recognizer takes. It keeps the fractional
// read position between calls, so a stream resampled in pieces sounds exactly
// like the same audio resampled in one go; no call resets it, and empty input
// changes nothing.
type Converter struct {
	rate  int
	chans int
	ratio float64 // input frames per output frame
	buf   []float32
	head  int64   // absolute input index of buf[0]
	pos   float64 // absolute input position of the next output frame
}

// NewConverter makes a converter for interleaved samples at rate with the
// given channel count.
func NewConverter(rate, channels int) *Converter {
	if rate <= 0 {
		rate = 16000
	}
	if channels <= 0 {
		channels = 1
	}
	return &Converter{
		rate:  rate,
		chans: channels,
		ratio: float64(rate) / 16000,
	}
}

// Rate is the input sample rate the converter was made for.
func (c *Converter) Rate() int { return c.rate }

// Feed converts one buffer of interleaved samples and returns the mono 16 kHz
// samples that are now complete.
func (c *Converter) Feed(interleaved []float32) []float32 {
	frames := len(interleaved) / c.chans
	if frames == 0 {
		return nil
	}
	if c.chans == 1 {
		c.buf = append(c.buf, interleaved[:frames]...)
	} else {
		for i := 0; i < frames; i++ {
			var sum float32
			for ch := 0; ch < c.chans; ch++ {
				sum += interleaved[i*c.chans+ch]
			}
			c.buf = append(c.buf, sum/float32(c.chans))
		}
	}
	return c.read(false)
}

// Flush returns the samples still owed for the audio fed so far, so that the
// end of a stream is not cut short. It invents nothing: only positions between
// frames that were really fed produce output.
func (c *Converter) Flush() []float32 {
	out := c.read(true)
	c.buf = nil
	c.head = int64(c.pos)
	return out
}

// read produces every output frame the buffered input covers.
func (c *Converter) read(final bool) []float32 {
	if c.buf == nil {
		return nil
	}
	last := c.head + int64(len(c.buf)) - 1
	var out []float32
	for {
		frac := c.pos - float64(int64(c.pos))
		i := int64(c.pos)
		switch {
		case i < c.head:
			// The position was consumed already; resume at the first frame held.
			c.pos = float64(c.head)
			continue
		case frac == 0 && i <= last:
			// On a frame: no interpolation is needed, so 16 kHz in, 16 kHz out
			// costs no latency, and every rate keeps its exact samples.
			out = append(out, clip(c.buf[i-c.head]))
			c.pos += c.ratio
		case i+1 <= last:
			a, b := c.buf[i-c.head], c.buf[i+1-c.head]
			out = append(out, clip(a+float32(frac)*(b-a)))
			c.pos += c.ratio
		case final && i == last:
			// The stream ended between two frames: use the last one rather than
			// leave the tail unconverted.
			out = append(out, clip(c.buf[i-c.head]))
			c.pos += c.ratio
		default:
			// The next output needs a frame that has not been fed yet.
			c.trim()
			return out
		}
	}
}

// trim drops the input frames no later output can reach, keeping the one frame
// an interpolation may still need.
func (c *Converter) trim() {
	keep := int64(c.pos)
	if keep > c.head {
		drop := min(keep-c.head, int64(len(c.buf)))
		if drop > 0 {
			c.buf = append(c.buf[:0], c.buf[drop:]...)
			c.head += drop
		}
	}
}

func clip(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
