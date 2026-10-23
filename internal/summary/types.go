package summary

// Chapter represents a timestamped section or topic of the media.
type Chapter struct {
	StartMs int64  `json:"startMs"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// Highlight represents a specific timestamped key moment or quote.
type Highlight struct {
	StartMs int64  `json:"startMs"`
	Note    string `json:"note"`
}

// Summary stores the complete AI-generated structured summary of a media file.
type Summary struct {
	ID              int64       `json:"id"`
	MediaID         int64       `json:"mediaId"`
	Overview        string      `json:"overview"`
	KeyPoints       []string    `json:"keyPoints"`
	ActionItems     []string    `json:"actionItems"`
	Chapters        []Chapter   `json:"chapters"`
	Highlights      []Highlight `json:"highlights"`
	ContentMarkdown string      `json:"contentMarkdown"`
	Model           string      `json:"model"`
	CreatedAt       string      `json:"createdAt"`
}

// Settings stores user configuration for the remote AI model endpoint.
type Settings struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}
