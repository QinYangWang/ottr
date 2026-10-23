package translate

import (
	"fmt"
	"strings"
)

// FormatSRT outputs single-language translated SRT subtitles.
func FormatSRT(segments []TranslationSegment) string {
	var sb strings.Builder
	for i, s := range segments {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatSRTTime(s.StartMs), formatSRTTime(s.EndMs)))
		sb.WriteString(s.TranslatedText)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FormatBilingualSRT outputs bilingual SRT subtitles with original and translated text.
func FormatBilingualSRT(segments []TranslationSegment) string {
	var sb strings.Builder
	for i, s := range segments {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatSRTTime(s.StartMs), formatSRTTime(s.EndMs)))
		sb.WriteString(s.TranslatedText)
		sb.WriteString("\n")
		sb.WriteString(s.OriginalText)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FormatVTT outputs WebVTT format translated subtitles.
func FormatVTT(segments []TranslationSegment) string {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")
	for _, s := range segments {
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatVTTTime(s.StartMs), formatVTTTime(s.EndMs)))
		sb.WriteString(s.TranslatedText)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FormatTXT outputs plain text translated subtitles.
func FormatTXT(segments []TranslationSegment) string {
	var sb strings.Builder
	for _, s := range segments {
		sb.WriteString(s.TranslatedText)
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
