package native

import (
	"strings"

	"ottr/internal/library"
)

// rowFacts is what a library row says beyond the media's record: the first
// line of its transcript, with the speaker's name, and how many voices it has.
type rowFacts struct {
	line     string
	speakers int
}

// ensureRowFacts reads a transcribed media's first line once, in the
// background, as thumbnails are.
func (a *App) ensureRowFacts(m library.Media) {
	if a.ASR == nil || m.TranscriptStatus != "done" {
		return
	}
	if _, done := a.rowFacts[m.ID]; done || a.rowFactsBusy[m.ID] {
		return
	}
	if a.rowFactsBusy == nil {
		a.rowFactsBusy = map[int64]bool{}
	}
	a.rowFactsBusy[m.ID] = true
	id := m.ID
	go func() {
		tr, err := a.ASR.Get(id)
		var f rowFacts
		if err == nil {
			seen := map[int]bool{}
			for _, seg := range tr.Segments {
				seen[seg.Speaker] = true
				if f.line == "" && strings.TrimSpace(seg.Text) != "" {
					name := tr.SpeakerNames[seg.Speaker]
					if name != "" && !strings.HasPrefix(name, "Speaker ") {
						f.line = strings.Fields(name)[0] + ": " + seg.Text
					} else {
						f.line = seg.Text
					}
				}
			}
			f.speakers = len(seen)
		}
		a.update(func() {
			delete(a.rowFactsBusy, id)
			if a.rowFacts == nil {
				a.rowFacts = map[int64]rowFacts{}
			}
			a.rowFacts[id] = f
		})
	}()
}
