package capture

import "strconv"

// audioSpec describes one PCM stream fed to ffmpeg over a pipe.
type audioSpec struct {
	rate     int
	channels int
	pipe     string
}

// buildArgs assembles the ffmpeg command: one video input, one f32le PCM
// pipe per audio stream, and a fragmented mp4 output.
func buildArgs(video []string, audios []audioSpec, out string) []string {
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
	enc := videoEncoder()
	args = append(args, "-c:v", enc.name)
	args = append(args, enc.opts...)
	// BGRA in, yuv420p out: the encoder reads the pipe's format and this is
	// what every decoder expects. Captured frames are physical pixels, so the
	// size is always even in practice.
	args = append(args, "-pix_fmt", "yuv420p")
	if len(audios) > 0 {
		args = append(args, "-c:a", "aac", "-b:a", "160k")
	}
	// Fragmented mp4 writes each keyframe's fragment as it finishes instead of
	// holding the index until the end: a recording that dies without stopping
	// cleanly — a killed process, a lost power — is still playable up to its
	// last keyframe. A plain mp4 of the same recording holds nothing readable.
	args = append(args, "-g", "30", "-movflags", "+frag_keyframe+empty_moov+default_base_moof")
	args = append(args, out)
	return args
}
