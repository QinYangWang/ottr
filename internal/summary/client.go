package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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

// GenerateSummary sends the transcript to the remote AI API and parses the
// structured summary.
func GenerateSummary(ctx context.Context, cfg Settings, mediaName string, tr asr.Transcript) (*Summary, error) {
	return generateSummary(ctx, cfg, mediaName, tr, nil)
}

// maxChunkChars bounds the transcript text one request carries, so that a long
// recording is summarized in parts instead of being cut by the provider.
const maxChunkChars = 6000

// generateSummary summarizes a transcript in as many requests as it takes to
// stay inside one prompt. A transcript that fits is summarized in one request;
// a long one is summarized part by part, and the parts are rolled up into one
// summary (map-reduce), keeping each part's chapters and highlights with the
// timestamps they were written at.
func generateSummary(ctx context.Context, cfg Settings, mediaName string, tr asr.Transcript, progress func(float64)) (*Summary, error) {
	if len(tr.Segments) == 0 {
		return nil, errors.New("cannot summarize an empty transcript")
	}
	ai, err := newCompleter(cfg)
	if err != nil {
		return nil, err
	}

	chunks := chunkSegments(tr.Segments, maxChunkChars)
	if len(chunks) == 1 {
		content, err := ai.chat(ctx, summarySystemPrompt, summaryUserPrompt(mediaName, formatSegments(chunks[0])))
		if err != nil {
			return nil, err
		}
		if progress != nil {
			progress(1)
		}
		return buildSummary(mediaName, parseSummary(content), ai.model), nil
	}

	parts := make([]*llmSummaryResponse, len(chunks))
	for i, chunk := range chunks {
		user := fmt.Sprintf("Title: %s\n\nThis is part %d of %d of the transcript, with timestamps:\n%s\n\nSummarize only this part. Use the same JSON structure; chapters and highlights must keep the timestamps shown above.",
			mediaName, i+1, len(chunks), formatSegments(chunk))
		content, err := ai.chat(ctx, summarySystemPrompt, user)
		if err != nil {
			return nil, fmt.Errorf("summary part %d of %d: %w", i+1, len(chunks), err)
		}
		parts[i] = parseSummary(content)
		if progress != nil {
			progress(0.8 * float64(i+1) / float64(len(chunks)))
		}
	}

	content, err := ai.chat(ctx, rollupSystemPrompt, rollupUserPrompt(mediaName, parts))
	if err != nil {
		return nil, fmt.Errorf("summary rollup: %w", err)
	}
	rolled := parseSummary(content)
	// Chapters and highlights come from the parts, where the timestamps are.
	rolled.Chapters = mergeChapters(parts)
	rolled.Highlights = mergeHighlights(parts)
	if rolled.Overview == "" {
		rolled.Overview = joinOverviews(parts)
	}
	if len(rolled.KeyPoints) == 0 {
		for _, part := range parts {
			rolled.KeyPoints = append(rolled.KeyPoints, part.KeyPoints...)
		}
	}
	if progress != nil {
		progress(1)
	}
	return buildSummary(mediaName, rolled, ai.model), nil
}

// completer is what one chat request needs: where to send it, and what to say.
type completer struct {
	url   string
	key   string
	model string
}

func newCompleter(cfg Settings) (*completer, error) {
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
	return &completer{url: baseURL + "/chat/completions", key: cfg.APIKey, model: model}, nil
}

// chat asks the model one question and returns its text.
func (c *completer) chat(ctx context.Context, system, user string) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote AI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatRes chatResponse
	if err := json.Unmarshal(body, &chatRes); err != nil {
		return "", fmt.Errorf("decode AI response: %w (%s)", err, strings.TrimSpace(string(body)))
	}
	if chatRes.Error != nil && chatRes.Error.Message != "" {
		return "", errors.New(chatRes.Error.Message)
	}
	if len(chatRes.Choices) == 0 {
		return "", errors.New("empty response from AI model")
	}
	return chatRes.Choices[0].Message.Content, nil
}

const summarySystemPrompt = `You are an expert AI video assistant analyzing a transcript.
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

const rollupSystemPrompt = `You merge partial summaries of one recording into a single summary.
Output ONLY valid JSON, without markdown fences, containing:
{
  "overview": "One overview of the whole recording, in the order the parts happened.",
  "key_points": ["The main takeaways of the whole recording, without repetition"],
  "action_items": ["Every action item or to-do the parts mention"]
}`

func summaryUserPrompt(mediaName, transcriptText string) string {
	return fmt.Sprintf("Title: %s\n\nTranscript with timestamps:\n%s\n\nProduce the structured JSON summary.", mediaName, transcriptText)
}

// rollupUserPrompt asks for the one summary of many parts, from what each part
// said, which is far shorter than the transcript it came from.
func rollupUserPrompt(mediaName string, parts []*llmSummaryResponse) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Title: %s\n\nThe recording summarized in %d parts, in order:\n", mediaName, len(parts))
	for i, part := range parts {
		if part == nil {
			continue
		}
		fmt.Fprintf(&sb, "\n--- Part %d ---\n", i+1)
		if part.Overview != "" {
			sb.WriteString("Overview: " + part.Overview + "\n")
		}
		if len(part.KeyPoints) > 0 {
			sb.WriteString("Key points: " + strings.Join(part.KeyPoints, "; ") + "\n")
		}
		if len(part.ActionItems) > 0 {
			sb.WriteString("Action items: " + strings.Join(part.ActionItems, "; ") + "\n")
		}
		titles := make([]string, 0, len(part.Chapters))
		for _, ch := range part.Chapters {
			titles = append(titles, fmt.Sprintf("[%s] %s", formatTime(ch.StartMs), ch.Title))
		}
		if len(titles) > 0 {
			sb.WriteString("Chapters: " + strings.Join(titles, "; ") + "\n")
		}
	}
	sb.WriteString("\nMerge them into the JSON summary of the whole recording.")
	return sb.String()
}

// chunkSegments splits a transcript into groups of lines whose text stays
// under limit characters, so that each group fits one prompt.
func chunkSegments(segs []asr.Segment, limit int) [][]asr.Segment {
	var chunks [][]asr.Segment
	var cur []asr.Segment
	size := 0
	for _, seg := range segs {
		n := len(seg.Text) + 8 // the timestamp and the line break
		if len(cur) > 0 && size+n > limit {
			chunks = append(chunks, cur)
			cur, size = nil, 0
		}
		cur = append(cur, seg)
		size += n
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

func formatSegments(segs []asr.Segment) string {
	var sb strings.Builder
	for _, seg := range segs {
		fmt.Fprintf(&sb, "[%s] %s\n", formatTime(seg.StartMs), seg.Text)
	}
	return sb.String()
}

// parseSummary reads the JSON a model answered with, falling back to the text
// as the overview when it is not JSON at all.
func parseSummary(content string) *llmSummaryResponse {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed llmSummaryResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		parsed = llmSummaryResponse{Overview: content}
	}
	return &parsed
}

// mergeChapters joins the chapters of every part in time order.
func mergeChapters(parts []*llmSummaryResponse) []Chapter {
	var all []Chapter
	for _, part := range parts {
		if part != nil {
			all = append(all, part.Chapters...)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].StartMs < all[j].StartMs })
	if len(all) > maxChapters {
		all = all[:maxChapters]
	}
	return all
}

func mergeHighlights(parts []*llmSummaryResponse) []Highlight {
	var all []Highlight
	for _, part := range parts {
		if part != nil {
			all = append(all, part.Highlights...)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].StartMs < all[j].StartMs })
	if len(all) > maxHighlights {
		all = all[:maxHighlights]
	}
	return all
}

func joinOverviews(parts []*llmSummaryResponse) string {
	var texts []string
	for _, part := range parts {
		if part != nil && part.Overview != "" {
			texts = append(texts, part.Overview)
		}
	}
	return strings.Join(texts, " ")
}

// A summary of a long recording keeps a readable number of chapters and moments.
const (
	maxChapters   = 40
	maxHighlights = 20
)

func buildSummary(mediaName string, parsed *llmSummaryResponse, model string) *Summary {
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
	}
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
