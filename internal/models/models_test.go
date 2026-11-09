package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ottr/internal/storage"
)

func TestManagerNoModels(t *testing.T) {
	emptyDir := t.TempDir()
	m := NewManager(emptyDir, emptyDir)
	list, err := m.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 models, got %d", len(list))
	}
	if _, err := m.FindASRModel(); err == nil {
		t.Fatal("expected error when no model installed")
	}
}

func TestManagerDetectsModel(t *testing.T) {
	base := t.TempDir()
	modelDir := filepath.Join(base, "sherpa-streaming-test-model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"tokens.txt", "model.int8.onnx"} {
		if err := os.WriteFile(filepath.Join(modelDir, f), []byte("dummy-data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := NewManager(base, base)
	list, err := m.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Installed {
		t.Fatalf("expected 1 installed model, got %+v", list)
	}
	if list[0].Name != "SenseVoice Small" {
		t.Errorf("display name = %q", list[0].Name)
	}
	mdl, err := m.FindASRModel()
	if err != nil {
		t.Fatal(err)
	}
	if mdl.Dir != modelDir {
		t.Errorf("wrong dir: %s", mdl.Dir)
	}
	if mdl.Size != int64(len("dummy-data")*2) {
		t.Errorf("wrong size: %d", mdl.Size)
	}
	if mdl.Kind != KindSenseVoice {
		t.Errorf("model kind = %q", mdl.Kind)
	}
}

// Choosing a model in the Models page is what FindASRModel uses, and it is
// remembered in the settings table.
func TestSelectedModelIsUsedAndRemembered(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	base := t.TempDir()
	for _, name := range []string{"first-model", "second-model"} {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"tokens.txt", "model.int8.onnx"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("dummy"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	m := NewManager(base, base)
	m.Open(db)
	// Without a choice, the first valid directory is used.
	if mdl, err := m.FindASRModel(); err != nil || mdl.Dir != filepath.Join(base, "first-model") {
		t.Fatalf("default model: %+v, %v", mdl, err)
	}

	second := filepath.Join(base, "second-model")
	if err := m.Select(second); err != nil {
		t.Fatal(err)
	}
	if mdl, err := m.FindASRModel(); err != nil || mdl.Dir != second {
		t.Fatalf("chosen model: %+v, %v", mdl, err)
	}

	// Another launch reads the same choice.
	reopened := NewManager(base, base)
	reopened.Open(db)
	if got := reopened.SelectedDir(); got != second {
		t.Errorf("the choice was not remembered: %q", got)
	}

	// A chosen model that is no longer valid says so instead of quietly
	// transcribing with something else.
	if err := os.Remove(filepath.Join(second, "model.int8.onnx")); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.FindASRModel(); err == nil {
		t.Error("a broken chosen model was used anyway")
	}
}

// Every supported model is offered, whether or not one is on disk.
func TestAvailableListsEverySupportedModel(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir())
	offers := m.Available()
	if len(offers) != len(SupportedModels()) {
		t.Fatalf("%d offers for %d models", len(offers), len(SupportedModels()))
	}
	for _, offer := range offers {
		if offer.Installed || offer.DownloadSize == 0 || offer.Name == "" {
			t.Errorf("offer: %+v", offer)
		}
	}
}

// A download is checked, unpacked and validated before it becomes the model.
func TestInstallDownloadsAndUnpacks(t *testing.T) {
	archive, err := os.ReadFile("testdata/tiny.tar.bz2")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()
	sum := sha256.Sum256(archive)
	dir := t.TempDir()
	m := NewManager(dir, dir)

	var last float64
	err = m.Install(context.Background(), Supported{
		ID: "tiny-model", Kind: KindSenseVoice, Name: "Tiny", Size: int64(len(archive)),
		URL: srv.URL, SHA256: hex.EncodeToString(sum[:]),
	}, func(p float64) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	if last != 1 {
		t.Errorf("progress ended at %v", last)
	}
	mdl, err := m.FindASRModel()
	if err != nil {
		t.Fatal(err)
	}
	if mdl.Kind != KindSenseVoice || filepath.Base(mdl.Dir) != "tiny-model" {
		t.Errorf("installed model: %+v", mdl)
	}
	offers := m.Available()
	if len(offers) == 0 || !offers[0].Installed || offers[0].Path != mdl.Dir {
		t.Errorf("the installed model is not offered: %+v", offers)
	}
}

// An archive that does not hash as pinned is dropped and installs nothing.
func TestInstallRefusesAnotherArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not the pinned bytes"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := NewManager(dir, dir)
	err := m.Install(context.Background(), Supported{
		ID: "tiny-model", Kind: KindSenseVoice, Name: "Tiny", Size: 20, URL: srv.URL,
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	}, nil)
	if err == nil {
		t.Fatal("an archive with another digest was installed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed install left files: %v", entries)
	}
}
