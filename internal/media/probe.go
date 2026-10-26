// Package media probes media files with ffprobe.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"ottr/internal/tools"
)

// Info is what ffprobe reports about a media file.
type Info struct {
	DurationMs int64
	Width      int
	Height     int
	Size       int64
	HasAudio   bool
	HasVideo   bool
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

// Probe runs ffprobe on path. It fails with a clear error when ffprobe is
// not installed.
func Probe(ctx context.Context, path string) (Info, error) {
	ffprobe, err := tools.FFprobe()
	if err != nil {
		return Info{}, err
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-show_format", "-show_streams", "-of", "json", path)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("ffprobe failed on %s: %w", path, err)
	}
	var po probeOutput
	if err := json.Unmarshal(out, &po); err != nil {
		return Info{}, fmt.Errorf("ffprobe output: %w", err)
	}
	var info Info
	if secs, err := strconv.ParseFloat(po.Format.Duration, 64); err == nil {
		info.DurationMs = int64(secs * 1000)
	}
	info.Size, _ = strconv.ParseInt(po.Format.Size, 10, 64)
	if info.Size == 0 {
		if st, err := os.Stat(path); err == nil {
			info.Size = st.Size()
		}
	}
	for _, s := range po.Streams {
		switch s.CodecType {
		case "video":
			if !info.HasVideo {
				info.Width, info.Height = s.Width, s.Height
			}
			info.HasVideo = true
		case "audio":
			info.HasAudio = true
		}
	}
	return info, nil
}
