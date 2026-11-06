package asr

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// ExportFormats are the transcript files Export can write, by extension.
var ExportFormats = []string{"srt", "vtt", "txt", "md", "docx"}

// ExportOptions shape a transcript file beyond its format.
type ExportOptions struct {
	// Speakers prefixes each line with its speaker's name.
	Speakers bool
	// Timestamps prefixes each line of a text, Markdown or Word file with its
	// start time. Subtitle formats are always timed.
	Timestamps bool
	// Translation maps a segment ID to its translated text; a segment that has
	// one gets it on the line after its own.
	Translation map[int64]string
	// Preamble is Markdown put before the transcript of a Markdown file: the
	// summary, when there is one.
	Preamble string
	// Title heads a Markdown or Word file.
	Title string
}

// Render writes a transcript in one of ExportFormats.
func Render(tr Transcript, format string, opts ExportOptions) ([]byte, error) {
	segs := tr.Segments
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "srt":
		return []byte(FormatSRT(exportLines(tr, segs, opts, false))), nil
	case "vtt":
		return []byte(FormatVTT(exportLines(tr, segs, opts, false))), nil
	case "txt":
		var sb strings.Builder
		for _, seg := range exportLines(tr, segs, opts, opts.Timestamps) {
			sb.WriteString(seg.Text)
			sb.WriteString("\n")
		}
		return []byte(sb.String()), nil
	case "md":
		return []byte(renderMarkdown(tr, opts)), nil
	case "docx":
		return renderDocx(tr, opts)
	}
	return nil, fmt.Errorf("unsupported export format: %s", format)
}

// SpeakerName is the name a speaker is exported under: the one given, else
// the numbered default.
func SpeakerName(tr Transcript, speaker int) string {
	if name := tr.SpeakerNames[speaker]; name != "" {
		return name
	}
	return fmt.Sprintf("Speaker %d", speaker+1)
}

// exportLines returns the segments with their text as the options want it:
// a time and a speaker in front, a translation after.
func exportLines(tr Transcript, segs []Segment, opts ExportOptions, timed bool) []Segment {
	out := make([]Segment, len(segs))
	for i, seg := range segs {
		text := seg.Text
		if opts.Speakers {
			text = SpeakerName(tr, seg.Speaker) + ": " + text
		}
		if timed {
			text = "[" + clock(seg.StartMs) + "] " + text
		}
		if translated := opts.Translation[seg.ID]; translated != "" {
			text += "\n" + translated
		}
		seg.Text = text
		out[i] = seg
	}
	return out
}

func renderMarkdown(tr Transcript, opts ExportOptions) string {
	var sb strings.Builder
	if opts.Preamble != "" {
		sb.WriteString(strings.TrimRight(opts.Preamble, "\n"))
		sb.WriteString("\n\n## Transcript\n\n")
	} else if opts.Title != "" {
		sb.WriteString("# " + opts.Title + "\n\n")
	}
	for _, seg := range tr.Segments {
		var head []string
		if opts.Timestamps {
			head = append(head, "`"+clock(seg.StartMs)+"`")
		}
		if opts.Speakers {
			head = append(head, "**"+SpeakerName(tr, seg.Speaker)+"**")
		}
		line := seg.Text
		if len(head) > 0 {
			line = strings.Join(head, " ") + " " + line
		}
		sb.WriteString(line + "\n")
		if translated := opts.Translation[seg.ID]; translated != "" {
			sb.WriteString("> " + translated + "\n")
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// clock is a start time as a reader expects it: m:ss, or h:mm:ss.
func clock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// renderDocx writes a minimal Word document: one paragraph per segment, the
// time and speaker in bold, a translation in its own paragraph.
func renderDocx(tr Transcript, opts ExportOptions) ([]byte, error) {
	var body strings.Builder
	if opts.Title != "" {
		body.WriteString(docxParagraph(docxRun(opts.Title, true, 32)))
	}
	for _, seg := range tr.Segments {
		var head []string
		if opts.Timestamps {
			head = append(head, clock(seg.StartMs))
		}
		if opts.Speakers {
			head = append(head, SpeakerName(tr, seg.Speaker))
		}
		runs := ""
		if len(head) > 0 {
			runs = docxRun(strings.Join(head, " · ")+"  ", true, 0)
		}
		body.WriteString(docxParagraph(runs + docxRun(seg.Text, false, 0)))
		if translated := opts.Translation[seg.ID]; translated != "" {
			body.WriteString(docxParagraph(docxRun(translated, false, 0)))
		}
	}
	files := []struct{ name, data string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
			body.String() + `</w:body></w:document>`},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, fmt.Errorf("write docx: %w", err)
		}
		if _, err := w.Write([]byte(f.data)); err != nil {
			return nil, fmt.Errorf("write docx: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("write docx: %w", err)
	}
	return buf.Bytes(), nil
}

func docxParagraph(runs string) string { return "<w:p>" + runs + "</w:p>" }

// docxRun is one run of text; size is in half-points, 0 for the default.
func docxRun(text string, bold bool, size int) string {
	var props string
	if bold {
		props += "<w:b/>"
	}
	if size > 0 {
		props += fmt.Sprintf(`<w:sz w:val="%d"/>`, size)
	}
	if props != "" {
		props = "<w:rPr>" + props + "</w:rPr>"
	}
	var esc bytes.Buffer
	xml.EscapeText(&esc, []byte(text))
	return `<w:r>` + props + `<w:t xml:space="preserve">` + esc.String() + `</w:t></w:r>`
}
