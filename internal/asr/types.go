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

// LiveSubtitle is broadcast during real-time transcription.
type LiveSubtitle struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
	Final   bool   `json:"final"`
}
