package asr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"ottr/internal/models"
)

// A packaged install carries the library in its own resources, which are
// searched before the folder a user may have installed it in by hand.
func TestRuntimePrefersTheAppsOwnResources(t *testing.T) {
	res := t.TempDir()
	bin := filepath.Join(res, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	bundled := filepath.Join(bin, libraryNames[0])
	if err := os.WriteFile(bundled, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathResources, res)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathResources, "") })

	userBin := t.TempDir()
	svc := NewService(nil, models.NewManager(t.TempDir(), userBin), nil)
	paths, dir := svc.libraryCandidates()
	if len(paths) == 0 || paths[0] != bundled {
		t.Fatalf("candidates are %v, want the bundled library first", paths)
	}
	if !strings.Contains(dir, bin) || !strings.Contains(dir, userBin) {
		t.Errorf("the error names %q, want both folders", dir)
	}
	if len(paths) != 1 {
		t.Errorf("candidates are %v, want only what exists", paths)
	}
}

// Without a packaged library, the configured bin folder is still what a manual
// install uses.
func TestRuntimeFallsBackToTheBinDir(t *testing.T) {
	mygo.App.SetPath(mygo.PathResources, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathResources, "") })

	bin := t.TempDir()
	manual := filepath.Join(bin, libraryNames[0])
	if err := os.WriteFile(manual, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, models.NewManager(t.TempDir(), bin), nil)
	paths, _ := svc.libraryCandidates()
	if len(paths) != 1 || paths[0] != manual {
		t.Fatalf("candidates are %v, want %s", paths, manual)
	}
}

// A missing library says where it looked.
func TestRuntimeSaysWhereItLooked(t *testing.T) {
	mygo.App.SetPath(mygo.PathResources, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathResources, "") })

	svc := NewService(nil, models.NewManager(t.TempDir(), t.TempDir()), nil)
	rt := svc.RuntimeStatus()
	if rt.OK() {
		t.Fatalf("a machine with no library reports one: %+v", rt)
	}
	if !strings.Contains(rt.Err, libraryNames[0]) {
		t.Errorf("the error does not name the file: %q", rt.Err)
	}
}
