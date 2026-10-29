package asr

// Segment represents one timestamped subtitle line.
type Segment struct {
	ID      int64  `json:"id,omitempty"`
	Idx     int    `json:"idx"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// Transcript contains all segments for a media file.
type Transcript struct {
	ID        int64     `json:"id"`
	MediaID   int64     `json:"mediaId"`
	Language  string    `json:"language"`
	CreatedAt string    `json:"createdAt"`
	Segments  []Segment `json:"segments"`
}

// LiveSubtitle is one revision of the line being spoken while a recording runs.
//
// The same utterance is published again and again with more text, each time
// with the same Utterance number: the window replaces that line instead of
// adding one. Final marks the revision that will not change again.
type LiveSubtitle struct {
	Utterance int64  `json:"utterance"`
	StartMs   int64  `json:"startMs"`
	EndMs     int64  `json:"endMs"`
	Text      string `json:"text"`
	Final     bool   `json:"final"`
	// Warn is set on a revision that only tells the window live text is
	// behind; it carries no text and replaces nothing.
	Warn string `json:"warn,omitempty"`
	// Error is set when live recognition stopped for good.
	Error string `json:"error,omitempty"`
}
