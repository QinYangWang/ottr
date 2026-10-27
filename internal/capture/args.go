package capture

import "strconv"

const previewFilter = "fps=5,scale=960:-2"

// audioSpec describes one PCM stream fed to ffmpeg over a pipe.
type audioSpec struct {
	rate     int
	channels int
	pipe     string
}

// buildArgs assembles the ffmpeg command: one video input, one f32le PCM
// pipe per audio stream, an mp4 output and an MJPEG preview output.
func buildArgs(video []string, audios []audioSpec, previewAddr, out string) []string {
	args := []string{"-y", "-loglevel", "error"}
	args = append(args, video...)
	for _, a := range audios {
		args = append(args,
			"-thread_queue_size", "512",
			"-f", "f32le", "-ar", strconv.Itoa(a.rate), "-ac", strconv.Itoa(a.channels),
			"-i", a.pipe)
	}
	switch len(audios) {
	case 0:
		args = append(args, "-map", "0:v")
	case 1:
		args = append(args, "-map", "0:v", "-map", "1:a")
	default:
		args = append(args,
			"-filter_complex",
			"[1:a]aresample=48000:first_pts=0[a1];[2:a]aresample=48000:first_pts=0[a2];[a1][a2]amix=inputs=2:normalize=0[a]",
			"-map", "0:v", "-map", "[a]")
	}
	args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p")
	if len(audios) > 0 {
		args = append(args, "-c:a", "aac", "-b:a", "160k")
	}
	args = append(args, out)
	if previewAddr != "" {
		args = append(args,
			"-map", "0:v", "-vf", previewFilter,
			"-f", "mpjpeg", "tcp://"+previewAddr)
	}
	return args
}
