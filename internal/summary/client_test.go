package summary

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ottr/internal/asr"
)

func TestFormatMarkdown(t *testing.T) {
	title := "Team Standup"
	overview := "Daily sync on product progress."
	keyPoints := []string{"Phase 1 done", "Phase 2 in review"}
	actionItems := []string{"Fix issue #12"}
	chapters := []Chapter{
		{StartMs: 0, Title: "Intro", Summary: "Greetings"},
		{StartMs: 60000, Title: "Updates", Summary: "Dev updates"},
	}
	highlights := []Highlight{
		{StartMs: 120000, Note: "Great launch stats"},
	}

	md := FormatMarkdown(title, overview, keyPoints, actionItems, chapters, highlights)
	if !strings.Contains(md, "# Team Standup — AI Summary") {
		t.Errorf("title missing: %s", md)
	}
	if !strings.Contains(md, "Daily sync on product progress.") {
		t.Errorf("overview missing: %s", md)
	}
	if !strings.Contains(md, "- Phase 1 done") {
		t.Errorf("key point missing: %s", md)
	}
	if !strings.Contains(md, "- [ ] Fix issue #12") {
		t.Errorf("action item missing: %s", md)
	}
	if !strings.Contains(md, "- **[01:00] Updates**: Dev updates") {
		t.Errorf("chapter missing or wrong time format: %s", md)
	}
	if !strings.Contains(md, "- **[02:00]**: Great launch stats") {
		t.Errorf("highlight missing or wrong time format: %s", md)
	}
}

func TestGenerateSummaryWithServer(t *testing.T) {
	expectedRes := llmSummaryResponse{
		Overview:    "Discussion on release plan.",
		KeyPoints:   []string{"Scope fixed", "Release next week"},
		ActionItems: []string{"QA testing"},
		Chapters: []Chapter{
			{StartMs: 0, Title: "Kickoff", Summary: "Welcome"},
		},
		Highlights: []Highlight{
			{StartMs: 5000, Note: "Ready to ship"},
		},
	}
	resJSON, _ := json.Marshal(expectedRes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{Content: string(resJSON)},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Settings{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Model:   "mock-model",
	}

	tr := asr.Transcript{
		Segments: []asr.Segment{
			{Idx: 1, StartMs: 0, EndMs: 4000, Text: "Hello team, let's talk about the release."},
		},
	}

	sum, err := GenerateSummary(context.Background(), cfg, "Release Sync", tr)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Overview != expectedRes.Overview {
		t.Errorf("overview: got %q, want %q", sum.Overview, expectedRes.Overview)
	}
	if len(sum.KeyPoints) != 2 || sum.KeyPoints[0] != "Scope fixed" {
		t.Errorf("key points: %+v", sum.KeyPoints)
	}
	if len(sum.Chapters) != 1 || sum.Chapters[0].Title != "Kickoff" {
		t.Errorf("chapters: %+v", sum.Chapters)
	}
	if !strings.Contains(sum.ContentMarkdown, "Discussion on release plan.") {
		t.Errorf("markdown missing overview: %s", sum.ContentMarkdown)
	}
}

func TestGenerateSummaryRequiresAPIKey(t *testing.T) {
	cfg := Settings{
		BaseURL: "https://api.deepseek.com/v1",
		APIKey:  "",
	}
	tr := asr.Transcript{
		Segments: []asr.Segment{{Text: "Hi"}},
	}
	_, err := GenerateSummary(context.Background(), cfg, "Demo", tr)
	if err == nil || !strings.Contains(err.Error(), "API Key not configured") {
		t.Fatalf("expected API Key error, got: %v", err)
	}
}
