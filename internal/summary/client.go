package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ottr/internal/asr"
)

type llmSummaryResponse struct {
	Overview    string      `json:"overview"`
	KeyPoints   []string    `json:"key_points"`
	ActionItems []string    `json:"action_items"`
	Chapters    []Chapter   `json:"chapters"`
	Highlights  []Highlight `json:"highlights"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GenerateSummary sends the transcript to the remote AI API and parses the structured summary.
func GenerateSummary(ctx context.Context, cfg Settings, mediaName string, tr asr.Transcript) (*Summary, error) {
	if len(tr.Segments) == 0 {
		return nil, errors.New("cannot summarize an empty transcript")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	if !strings.HasSuffix(baseURL, "/v1") && !strings.Contains(baseURL, "/v1/") {
		baseURL += "/v1"
	}

	if cfg.APIKey == "" && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		return nil, errors.New("API Key not configured. Please click ⚙ Config to configure your AI API Key (DeepSeek, OpenAI, SiliconFlow, or Ollama)")
	}

	model := cfg.Model
	if model == "" {
		if strings.Contains(baseURL, "deepseek") {
			model = "deepseek-chat"
		} else {
			model = "gpt-4o-mini"
		}
	}

	var sb strings.Builder
	for _, seg := range tr.Segments {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", formatTime(seg.StartMs), seg.Text))
	}
	transcriptText := sb.String()

	systemPrompt := `You are an expert AI video assistant analyzing a transcript.
Generate a structured, professional summary in JSON format.
The JSON must contain:
{
  "overview": "A clear, comprehensive summary of the entire session.",
  "key_points": ["Main takeaway 1", "Main takeaway 2"],
  "action_items": ["Action item or to-do 1", "Action item or to-do 2"],
  "chapters": [
    {"start_ms": 1000, "title": "Chapter title", "summary": "What was discussed"}
  ],
  "highlights": [
    {"start_ms": 5000, "note": "Notable moment or quotation"}
  ]
}
Output ONLY valid JSON without markdown fences.`

	userPrompt := fmt.Sprintf("Title: %s\n\nTranscript with timestamps:\n%s\n\nProduce the structured JSON summary.", mediaName, transcriptText)

	reqBody, _ := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
	})

	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote AI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chatRes chatResponse
	if err := json.Unmarshal(body, &chatRes); err != nil {
		return nil, fmt.Errorf("decode AI response: %w (%s)", err, strings.TrimSpace(string(body)))
	}
	if chatRes.Error != nil && chatRes.Error.Message != "" {
		return nil, errors.New(chatRes.Error.Message)
	}
	if len(chatRes.Choices) == 0 {
		return nil, errors.New("empty response from AI model")
	}

	content := strings.TrimSpace(chatRes.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed llmSummaryResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		// If pure JSON parsing failed, use the text content directly as overview
		parsed.Overview = content
	}

	md := FormatMarkdown(mediaName, parsed.Overview, parsed.KeyPoints, parsed.ActionItems, parsed.Chapters, parsed.Highlights)

	return &Summary{
		Overview:        parsed.Overview,
		KeyPoints:       parsed.KeyPoints,
		ActionItems:     parsed.ActionItems,
		Chapters:        parsed.Chapters,
		Highlights:      parsed.Highlights,
		ContentMarkdown: md,
		Model:           model,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// FormatMarkdown formats structured summary fields into a clean Markdown document.
func FormatMarkdown(title, overview string, keyPoints, actionItems []string, chapters []Chapter, highlights []Highlight) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s — AI Summary\n\n", title))

	if overview != "" {
		sb.WriteString("## Overview\n\n")
		sb.WriteString(overview)
		sb.WriteString("\n\n")
	}

	if len(keyPoints) > 0 {
		sb.WriteString("## Key Points\n\n")
		for _, kp := range keyPoints {
			sb.WriteString(fmt.Sprintf("- %s\n", kp))
		}
		sb.WriteString("\n")
	}

	if len(actionItems) > 0 {
		sb.WriteString("## Action Items\n\n")
		for _, ai := range actionItems {
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", ai))
		}
		sb.WriteString("\n")
	}

	if len(chapters) > 0 {
		sb.WriteString("## Chapters\n\n")
		for _, ch := range chapters {
			sb.WriteString(fmt.Sprintf("- **[%s] %s**: %s\n", formatTime(ch.StartMs), ch.Title, ch.Summary))
		}
		sb.WriteString("\n")
	}

	if len(highlights) > 0 {
		sb.WriteString("## Highlights\n\n")
		for _, hl := range highlights {
			sb.WriteString(fmt.Sprintf("- **[%s]**: %s\n", formatTime(hl.StartMs), hl.Note))
		}
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func formatTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	m := s / 60
	s %= 60
	h := m / 60
	m %= 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
