package asr

import (
	"fmt"
	"strings"
)

// FormatSRT converts segments into SubRip (.srt) subtitle format.
func FormatSRT(segments []Segment) string {
	var sb strings.Builder
	for i, s := range segments {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatSRTTime(s.StartMs), formatSRTTime(s.EndMs)))
		sb.WriteString(s.Text)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FormatVTT converts segments into WebVTT (.vtt) format.
func FormatVTT(segments []Segment) string {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")
	for _, s := range segments {
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatVTTTime(s.StartMs), formatVTTTime(s.EndMs)))
		sb.WriteString(s.Text)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FormatTXT converts segments into plain text with one line per subtitle.
func FormatTXT(segments []Segment) string {
	var sb strings.Builder
	for _, s := range segments {
		sb.WriteString(s.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatSRTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, millis)
}

func formatVTTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, millis)
}
