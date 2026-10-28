//go:build windows

package capture

import (
	"os/exec"
	"sync"

	"ottr/internal/tools"
)

// encoder is a video encoder FFmpeg can open here.
type encoder struct {
	name string // the FFmpeg encoder
	opts []string
}

var (
	encOnce sync.Once
	encBest encoder
)

// videoEncoder picks the encoder a recording uses: the first hardware one that
// can actually open here, else libx264.
//
// Hardware encoders keep the CPU free — measured on a 4K desktop, six seconds of
// 30 fps capture cost 12.55 CPU seconds with libx264 and 6.09 with QSV — but a
// build offering one is no promise it works: h264_nvenc fails outright without
// nvcuda.dll, and h264_amf without amrt64.dll. Each candidate is opened once, on
// a tiny black frame, and the first that succeeds wins.
func videoEncoder() encoder {
	encOnce.Do(func() {
		ffmpeg, err := tools.FFmpeg()
		if err == nil {
			candidates := []encoder{
				{"h264_nvenc", []string{"-preset", "p4", "-rc", "vbr", "-cq", "23", "-b:v", "0"}},
				{"h264_qsv", []string{"-preset", "veryfast", "-global_quality", "23"}},
				{"h264_amf", []string{"-quality", "balanced", "-rc", "cqp", "-qp_i", "23", "-qp_p", "23"}},
			}
			for _, c := range candidates {
				if encoderOpens(ffmpeg, c) {
					encBest = c
					return
				}
			}
		}
		encBest = encoder{"libx264", []string{"-preset", "veryfast", "-crf", "23"}}
	})
	return encBest
}

// encoderOpens encodes one short test frame with c.
func encoderOpens(ffmpeg string, c encoder) bool {
	args := []string{"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=0.1",
		"-c:v", c.name}
	args = append(args, c.opts...)
	args = append(args, "-pix_fmt", "yuv420p", "-f", "null", "-")
	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)
	return cmd.Run() == nil
}
