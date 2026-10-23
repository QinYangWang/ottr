package models

import (
	"os"
	"path/filepath"
	"testing"
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
