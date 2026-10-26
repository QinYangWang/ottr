package summary

import (
	"io"
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

// A summary is a job: cancelling it stops the request it is waiting on.
func TestStartGenerateIsCancellable(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	lib := &library.Service{}
	lib.Open(db)
	js := &jobs.Service{}
	js.Open(db)
	asrSvc := asr.NewService(lib, models.NewManager(t.TempDir(), t.TempDir()), js)
	asrSvc.Open(db)
	sumSvc := NewService(lib, asrSvc, js)
	sumSvc.Open(db)

	at := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO media (name, source, path, created_at, has_audio, transcript_status)
		VALUES ('demo', 'import', '/tmp/demo.mp4', ?, 1, 'done')`, at)
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := res.LastInsertId()
	trRes, err := db.Exec(`INSERT INTO transcript (media_id, language, created_at) VALUES (?, 'en', ?)`, mediaID, at)
	if err != nil {
		t.Fatal(err)
	}
	trID, _ := trRes.LastInsertId()
	if _, err := db.Exec(`INSERT INTO segment (transcript_id, idx, start_ms, end_ms, text)
		VALUES (?, 1, 0, 3000, 'hello')`, trID); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, 1)
	gone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the body so that the server notices the client hanging up.
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		started <- struct{}{}
		select {
		case <-r.Context().Done(): // answer only once the caller gives up
		case <-gone:
		}
	}))
	defer server.Close()
	defer close(gone)
	if err := sumSvc.SaveSettings(Settings{BaseURL: server.URL, APIKey: "test-key", Model: "mock"}); err != nil {
		t.Fatal(err)
	}

	jobID, err := sumSvc.StartGenerate(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the summary request never went out")
	}

	if err := js.Cancel(jobID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		job, ok := js.Latest(mediaID)
		if ok && !job.Running() {
			if job.State != jobs.StateCancelled {
				t.Errorf("the job ended as %q: %+v", job.State, job)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the job did not stop: %+v", job)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Nothing was produced, so the media is not left saying it failed.
	m, err := lib.Get(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if m.SummaryStatus == "pending" || m.SummaryStatus == "error" {
		t.Errorf("summary_status after a cancelled job: %q", m.SummaryStatus)
	}
}
