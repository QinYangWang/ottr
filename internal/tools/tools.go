// Package tools finds the external programs Ottr runs: FFmpeg and its
// ffprobe. They are searched for in the folders that a user or an installer is
// likely to put them in, before the PATH — which a running app cannot see
// change until it restarts.
package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	mu     sync.Mutex
	binDir string
	chosen string
)

// SetBinDir sets the folder searched first, the one sherpa-onnx lives in.
func SetBinDir(dir string) {
	mu.Lock()
	defer mu.Unlock()
	binDir = dir
}

// SetChosen points the finder at a folder, or an executable inside one, that
// the user picked in the settings. An empty path clears it.
func SetChosen(path string) {
	mu.Lock()
	defer mu.Unlock()
	chosen = strings.TrimSpace(path)
}

// FFmpeg returns the ffmpeg to run, or an error saying where to put it.
func FFmpeg() (string, error) { return find("ffmpeg") }

// FFprobe returns the ffprobe to run, or an error saying where to put it.
func FFprobe() (string, error) { return find("ffprobe") }

// find looks for name in the folders Ottr knows about, in order, and then on
// the PATH: an installed-but-not-yet-on-the-PATH install still works.
func find(name string) (string, error) {
	mu.Lock()
	dir, picked := binDir, chosen
	mu.Unlock()

	var folders []string
	add := func(folder string) {
		if folder != "" {
			folders = append(folders, folder)
		}
	}

	if picked != "" {
		if isProgram(picked, name) {
			return picked, nil
		}
		add(picked)               // a folder
		add(filepath.Dir(picked)) // or the file it holds, beside it
	}
	add(dir)
	if appData := os.Getenv("APPDATA"); appData != "" {
		add(filepath.Join(appData, "Ottr", "bin"))
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		// Where winget puts what it installs: ffmpeg arrives in
		// Packages/<id>/<version>/bin, with shortcuts in Links.
		add(filepath.Join(localAppData, "Microsoft", "WinGet", "Links"))
		for _, match := range wingetPackages(localAppData) {
			add(match)
		}
	}
	if exe, err := os.Executable(); err == nil {
		here := filepath.Dir(exe)
		add(filepath.Join(here, "bin"))
		add(here)
	}

	for _, folder := range folders {
		for _, candidate := range []string{
			filepath.Join(folder, name+".exe"),
			filepath.Join(folder, name),
		} {
			if isProgram(candidate, name) {
				return candidate, nil
			}
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("%s not found: put %s beside sherpa-onnx in %s, choose it in Settings, or add it to the PATH and restart Ottr", name, name, dir)
}

// wingetPackages lists the bin folders of the packages winget installed, which
// is where a portable FFmpeg lands.
func wingetPackages(localAppData string) []string {
	matches, err := filepath.Glob(filepath.Join(localAppData, "Microsoft", "WinGet", "Packages", "*", "*", "bin"))
	if err != nil {
		return nil
	}
	return matches
}

// isProgram reports whether path is the program named name: the same file,
// whatever the case of the extension on Windows.
func isProgram(path, name string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return strings.EqualFold(base, name)
}
