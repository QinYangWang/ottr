package summary

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/asr"
	"ottr/internal/jobs"
	"ottr/internal/library"
	"ottr/internal/models"
	"ottr/internal/storage"
)

func setupTestSummary(t *testing.T) (*Service, *asr.Service, *library.Service) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	lib := &library.Service{}
	lib.Open(db)

	js := &jobs.Service{}
	js.Open(db)
	mm := models.NewManager(t.TempDir(), t.TempDir())
	asrSvc := asr.NewService(lib, mm, js)
	asrSvc.Open(db)

	sumSvc := NewService(lib, asrSvc, js)
	sumSvc.Open(db)

	return sumSvc, asrSvc, lib
}

func TestSettingsPersist(t *testing.T) {
	sumSvc, _, _ := setupTestSummary(t)
	cfg := Settings{
		BaseURL: "https://api.deepseek.com/v1",
		APIKey:  "sk-deepseek-test",
		Model:   "deepseek-chat",
	}
	if err := sumSvc.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	got := sumSvc.GetSettings()
	if got.BaseURL != cfg.BaseURL || got.APIKey != cfg.APIKey || got.Model != cfg.Model {
		t.Fatalf("settings mismatch: %+v", got)
	}
}

func TestSummaryLifecycle(t *testing.T) {
	sumSvc, asrSvc, lib := setupTestSummary(t)

	// Mock LLM server
	resJSON, _ := json.Marshal(llmSummaryResponse{
		Overview:    "Overview text",
		KeyPoints:   []string{"Point 1", "Point 2"},
		ActionItems: []string{"Action 1"},
		Chapters:    []Chapter{{StartMs: 0, Title: "Ch 1", Summary: "Intro"}},
		Highlights:  []Highlight{{StartMs: 2000, Note: "Key quote"}},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	sumSvc.SaveSettings(Settings{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Model:   "test-model",
	})

	// Create media & transcript
	db, _ := sumSvc.store()
	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio, transcript_status)
		VALUES ('demo', 'import', '/tmp/demo.mp4', ?, 1, 'done')`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := res.LastInsertId()

	resTr, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'zh', ?)`,
		mediaID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	trID, _ := resTr.LastInsertId()
	_, err = db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text) VALUES (?, 1, 0, 3000, '测试文本')`, trID)
	if err != nil {
		t.Fatal(err)
	}

	// Generate summary
	sum, err := sumSvc.Generate(context.Background(), mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Overview != "Overview text" {
		t.Errorf("overview: %s", sum.Overview)
	}
	if len(sum.KeyPoints) != 2 || len(sum.ActionItems) != 1 || len(sum.Chapters) != 1 || len(sum.Highlights) != 1 {
		t.Fatalf("unexpected summary items: %+v", sum)
	}

	// Verify media status updated to done
	m, err := lib.Get(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if m.SummaryStatus != "done" {
		t.Errorf("summary_status: %s", m.SummaryStatus)
	}

	// Fetch via Get
	fetched, err := sumSvc.Get(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Overview != sum.Overview || len(fetched.Chapters) != 1 || fetched.Chapters[0].Title != "Ch 1" {
		t.Fatalf("fetched summary mismatch: %+v", fetched)
	}
	_ = asrSvc
}
