package translate

// TranslationSegment represents one timestamped translated subtitle line.
type TranslationSegment struct {
	ID             int64  `json:"id,omitempty"`
	SegmentID      int64  `json:"segmentId"`
	Idx            int    `json:"idx"`
	StartMs        int64  `json:"startMs"`
	EndMs          int64  `json:"endMs"`
	OriginalText   string `json:"originalText"`
	TranslatedText string `json:"translatedText"`
}

// Translation contains all translated segments for a media file in a specific target language.
type Translation struct {
	ID         int64                `json:"id"`
	MediaID    int64                `json:"mediaId"`
	TargetLang string               `json:"targetLang"`
	SourceLang string               `json:"sourceLang"`
	CreatedAt  string               `json:"createdAt"`
	Segments   []TranslationSegment `json:"segments"`
}

// Settings stores user configuration for online translation.
type Settings struct {
	Provider string `json:"provider"` // "mymemory" | "custom"
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
}
