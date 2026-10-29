//go:build windows

package capture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/library"
	"ottr/internal/storage"
)

// A source that disappears mid-recording ends the session as a failure and
// leaves no file behind. The frames are synthetic — a real window closing is
// the case this stands for — and FFmpeg really runs, since the encoder is
// still part of the session.
func TestVanishEndsTheRecording(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lib := &library.Service{}
	lib.Open(db)
	r := &Recorder{Backend: &vanishingBackend{}, Dir: dir, Lib: lib}

	if err := r.Start(Config{SourceID: "screen"}); err != nil {
		t.Skipf("no encoder on this machine: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for r.State().Recording {
		if time.Now().After(deadline) {
			r.Stop()
			t.Fatal("the recording outlived its source")
		}
		time.Sleep(50 * time.Millisecond)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".mp4" {
			t.Errorf("a failed recording left %s behind", e.Name())
		}
	}
	if m, err := r.Stop(); err == nil {
		t.Errorf("Stop of a failed session returned %+v", m)
	}
}

// vanishingBackend delivers a few frames and then loses its source.
type vanishingBackend struct{}

func (vanishingBackend) Sources(context.Context) ([]Source, error) {
	return []Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}, nil
}

func (vanishingBackend) Open(ctx context.Context, cfg StreamConfig) (Stream, error) {
	q := newFrameQueue()
	go func() {
		for i := 0; i < 5; i++ {
			q.push(syntheticFrame(320, 240, byte(i)))
			time.Sleep(30 * time.Millisecond)
		}
		q.close(fmt.Errorf("%w: the window was closed", ErrSourceGone))
	}()
	return &fakeStream{queue: q, backend: &fakeBackend{}}, nil
}

func (vanishingBackend) Snapshot(context.Context, SnapshotConfig) (Frame, error) {
	return Frame{}, ErrSourceGone
}

func (vanishingBackend) Close() error { return nil }
