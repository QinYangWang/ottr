//go:build !windows

package capture

// encoder is a video encoder FFmpeg can open here.
type encoder struct {
	name string
	opts []string
}

// videoEncoder is libx264 off Windows: no hardware encoder is probed there yet.
func videoEncoder() encoder {
	return encoder{"libx264", []string{"-preset", "veryfast", "-crf", "23"}}
}
