package models

import (
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
	modelDir := filepath.Join(base, "sherpa-test-model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"tokens.txt", "encoder-test.onnx", "decoder-test.onnx", "joiner-test.onnx"} {
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
	mdl, err := m.FindASRModel()
	if err != nil {
		t.Fatal(err)
	}
	if mdl.Dir != modelDir {
		t.Errorf("wrong dir: %s", mdl.Dir)
	}
	if mdl.Size != int64(len("dummy-data")*4) {
		t.Errorf("wrong size: %d", mdl.Size)
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
		for _, f := range []string{"tokens.txt", "encoder.onnx", "decoder.onnx", "joiner.onnx"} {
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
	if err := os.Remove(filepath.Join(second, "joiner.onnx")); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.FindASRModel(); err == nil {
		t.Error("a broken chosen model was used anyway")
	}
}
