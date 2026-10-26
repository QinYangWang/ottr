package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The app's own folder is searched before the PATH, so a user who drops FFmpeg
// beside sherpa-onnx needs no environment change.
func TestFindsTheProgramInTheBinDir(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(exe, []byte("not really ffmpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	SetBinDir(dir)
	t.Cleanup(func() { SetBinDir("") })

	got, err := FFmpeg()
	if err != nil {
		t.Fatalf("ffmpeg in the bin dir was not found: %v", err)
	}
	if got != exe {
		t.Errorf("found %q, want %q", got, exe)
	}
}

// What the user picked in the settings wins, whether they picked the program or
// the folder holding it — which finds ffprobe too.
func TestChosenPathComesFirst(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ffmpeg.exe")
	probe := filepath.Join(dir, "ffprobe.exe")
	for _, p := range []string{exe, probe} {
		if err := os.WriteFile(p, []byte("not really ffmpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { SetChosen("") })

	SetChosen(dir)
	if got, err := FFmpeg(); err != nil || got != exe {
		t.Errorf("folder chosen: %q, %v", got, err)
	}
	if got, err := FFprobe(); err != nil || got != probe {
		t.Errorf("ffprobe beside it: %q, %v", got, err)
	}

	SetChosen(exe)
	if got, err := FFmpeg(); err != nil || got != exe {
		t.Errorf("program chosen: %q, %v", got, err)
	}
	if got, err := FFprobe(); err != nil || got != probe {
		t.Errorf("ffprobe beside the chosen program: %q, %v", got, err)
	}
}

// A program that is nowhere says where to put it, and that a PATH change needs
// a restart.
func TestMissingProgramSaysWhere(t *testing.T) {
	dir := t.TempDir()
	SetBinDir(dir)
	t.Cleanup(func() { SetBinDir("") })

	_, err := find("ottr-no-such-program")
	if err == nil {
		t.Fatal("a program that cannot exist was found")
	}
	if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), dir) {
		t.Errorf("the error does not say where: %v", err)
	}
	if !strings.Contains(err.Error(), "restart") {
		t.Errorf("the error does not say a PATH change needs a restart: %v", err)
	}
}
