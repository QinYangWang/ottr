package asr

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func exportFixture() Transcript {
	return Transcript{
		Segments: []Segment{
			{ID: 1, StartMs: 0, EndMs: 2000, Text: "Hello there.", Speaker: 0},
			{ID: 2, StartMs: 65000, EndMs: 67000, Text: "A & B <ok>", Speaker: 1},
		},
		SpeakerNames: map[int]string{1: "Ana"},
	}
}

func TestRender(t *testing.T) {
	tr := exportFixture()
	translated := map[int64]string{1: "你好。"}
	tests := []struct {
		name, format string
		opts         ExportOptions
		want         []string
		not          []string
	}{
		{"srt with speakers", "srt", ExportOptions{Speakers: true},
			[]string{"00:00:00,000 --> 00:00:02,000\nSpeaker 1: Hello there.", "00:01:05,000 --> 00:01:07,000\nAna: A & B <ok>"}, nil},
		{"srt ignores timestamps", "srt", ExportOptions{Timestamps: true}, []string{"\nHello there.\n"}, []string{"[0:00]"}},
		{"srt with translation", "srt", ExportOptions{Translation: translated}, []string{"Hello there.\n你好。\n"}, nil},
		{"plain text", "txt", ExportOptions{}, []string{"Hello there.\nA & B <ok>\n"}, []string{"Speaker"}},
		{"text with time and speaker", "txt", ExportOptions{Speakers: true, Timestamps: true},
			[]string{"[0:00] Speaker 1: Hello there.\n", "[1:05] Ana: A & B <ok>\n"}, nil},
		{"markdown with summary", "md", ExportOptions{Preamble: "# Standup\n\nOverview.\n", Speakers: true, Timestamps: true},
			[]string{"# Standup\n\nOverview.\n\n## Transcript\n\n", "`1:05` **Ana** A & B <ok>"}, nil},
		{"markdown alone", "md", ExportOptions{Title: "Standup", Translation: translated},
			[]string{"# Standup\n\nHello there.\n> 你好。\n"}, []string{"## Transcript"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(tr, tc.format, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(string(out), w) {
					t.Errorf("missing %q in\n%s", w, out)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(string(out), n) {
					t.Errorf("unexpected %q in\n%s", n, out)
				}
			}
		})
	}
	if _, err := Render(tr, "pdf", ExportOptions{}); err == nil {
		t.Error("an unknown format rendered")
	}
}

// A Word file is a zip whose document part holds each line, escaped.
func TestRenderDocx(t *testing.T) {
	out, err := Render(exportFixture(), "docx", ExportOptions{Speakers: true, Timestamps: true, Title: "Standup"})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(data)
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		if parts[name] == "" {
			t.Fatalf("missing part %s", name)
		}
	}
	doc := parts["word/document.xml"]
	for _, w := range []string{"Standup", "0:00 · Speaker 1", "Hello there.", "1:05 · Ana", "A &amp; B &lt;ok&gt;"} {
		if !strings.Contains(doc, w) {
			t.Errorf("document lacks %q: %s", w, doc)
		}
	}
}

func TestRecognizedShare(t *testing.T) {
	for _, tc := range []struct{ p, want float64 }{
		{0, 0}, {PhaseExtracting, 0}, {PhaseRecognizing, 0},
		{(PhaseRecognizing + PhaseSpeakers) / 2, 0.5},
		{PhaseSpeakers, 1}, {PhaseSaving, 1}, {1, 1},
	} {
		if got := RecognizedShare(tc.p); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("RecognizedShare(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

// ExportTo writes the stored transcript to the path given, as asked.
func TestExportTo(t *testing.T) {
	svc, _ := setupTestASR(t)
	db, _ := svc.store()
	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio)
		VALUES ('test', 'import', '/tmp/test.mp4', ?, 1)`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	path := filepath.Join(t.TempDir(), "out.txt")
	if err := svc.ExportTo(id, "txt", ExportOptions{}, path); err == nil {
		t.Fatal("exported a transcript that does not exist")
	}
	if err := svc.saveTranscript(id, []Segment{{StartMs: 0, EndMs: 1000, Text: "only line"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExportTo(id, "txt", ExportOptions{Timestamps: true}, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[0:00] only line\n" {
		t.Errorf("file says %q", data)
	}
}
