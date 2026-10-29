package asr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"ottr/internal/tools"
)

// ExtractWav converts any media file's audio stream to 16kHz mono 16-bit WAV
// PCM at wavPath.
//
// shortcut: FFmpeg still does the decoding and never reports its position, so
// the job's progress stays coarse; internal/media's native decoder will replace
// this and hand its PCM straight to recognizePCM.
func ExtractWav(ctx context.Context, mediaPath, wavPath string) error {
	ffmpeg, err := tools.FFmpeg()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-y", "-i", mediaPath,
		"-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", wavPath)
	hideWindowProc(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("audio extraction failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
