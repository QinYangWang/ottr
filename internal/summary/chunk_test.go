package summary

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ottr/internal/asr"
)

func TestChunkSegmentsFitsTheLimit(t *testing.T) {
	var segs []asr.Segment
	for i := 0; i < 10; i++ {
		segs = append(segs, asr.Segment{Idx: i + 1, StartMs: int64(i) * 1000, Text: strings.Repeat("a", 100)})
	}

	chunks := chunkSegments(segs, 250)
	if len(chunks) < 2 {
		t.Fatalf("a transcript over the limit was not split: %d chunks", len(chunks))
	}
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
		if len(formatSegments(chunk)) > 250+110 { // the line already over the limit may stand alone
			t.Errorf("chunk of %d characters is over the limit", len(formatSegments(chunk)))
		}
	}
	if total != len(segs) {
		t.Errorf("chunks hold %d of %d segments", total, len(segs))
	}

	// A line longer than the limit is still summarized, alone in its chunk.
	one := chunkSegments([]asr.Segment{{Text: strings.Repeat("a", 500)}}, 250)
	if len(one) != 1 || len(one[0]) != 1 {
		t.Errorf("an oversized segment was dropped or split: %v", one)
	}
}

// A transcript too long for one prompt is summarized part by part, and the
// parts are rolled up into one summary whose chapters keep their order.
func TestGenerateSummarySplitsLongTranscript(t *testing.T) {
	var calls int
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		bodies = append(bodies, req.Messages[1].Content)
		calls++

		var content []byte
		if strings.Contains(req.Messages[0].Content, "merge partial summaries") {
			content, _ = json.Marshal(llmSummaryResponse{
				Overview:    "The whole recording.",
				KeyPoints:   []string{"first", "second"},
				ActionItems: []string{"ship it"},
			})
		} else {
			// Parts answer in reverse time order, so that merging has work to do.
			content, _ = json.Marshal(llmSummaryResponse{
				Overview:   fmt.Sprintf("Overview of part %d", calls),
				Chapters:   []Chapter{{StartMs: int64(4-calls) * 60000, Title: fmt.Sprintf("Chapter %d", calls)}},
				Highlights: []Highlight{{StartMs: int64(4-calls) * 60000, Note: fmt.Sprintf("Moment %d", calls)}},
			})
		}
		resp := chatResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{})
		resp.Choices[0].Message.Content = string(content)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := asr.Transcript{}
	for i := 0; i < 100; i++ {
		tr.Segments = append(tr.Segments, asr.Segment{
			Idx: i + 1, StartMs: int64(i) * 10000, EndMs: int64(i+1) * 10000,
			Text: strings.Repeat("spoken words ", 20), // 260 characters a line
		})
	}
	if len(chunkSegments(tr.Segments, maxChunkChars)) < 2 {
		t.Fatalf("the test transcript does not need splitting")
	}

	var progress []float64
	sum, err := generateSummary(context.Background(), Settings{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Model:   "mock-model",
	}, "Long Meeting", tr, func(p float64) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}

	parts := len(chunkSegments(tr.Segments, maxChunkChars))
	if calls != parts+1 {
		t.Errorf("%d requests: want %d parts and one rollup", calls, parts)
	}
	if sum.Overview != "The whole recording." {
		t.Errorf("overview: %q", sum.Overview)
	}
	if len(sum.KeyPoints) != 2 || sum.ActionItems[0] != "ship it" {
		t.Errorf("key points %v, action items %v", sum.KeyPoints, sum.ActionItems)
	}
	if len(sum.Chapters) != parts {
		t.Fatalf("%d chapters from %d parts", len(sum.Chapters), parts)
	}
	for i := 1; i < len(sum.Chapters); i++ {
		if sum.Chapters[i].StartMs < sum.Chapters[i-1].StartMs {
			t.Errorf("chapters are not in time order: %+v", sum.Chapters)
		}
	}
	if len(sum.Highlights) != parts {
		t.Errorf("%d highlights from %d parts", len(sum.Highlights), parts)
	}
	if len(progress) == 0 || progress[len(progress)-1] != 1 {
		t.Errorf("progress ended at %v, want 1", progress)
	}
	if !strings.Contains(sum.ContentMarkdown, "The whole recording.") {
		t.Errorf("markdown has no overview: %s", sum.ContentMarkdown)
	}
	// The rollup is asked with what the parts said, not the transcript again.
	last := bodies[len(bodies)-1]
	if !strings.Contains(last, "Part 1") || strings.Contains(last, "spoken words") {
		t.Errorf("the rollup was given the transcript instead of the parts: %s", last)
	}
}
